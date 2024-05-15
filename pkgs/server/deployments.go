package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/go-chi/chi/v5"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func makeEnvs(newenvs map[string]string, secretDb *Secrets) ([]string, error) {
	resolvedEnvs := []string{}
	for key, value := range newenvs {
		if value[0] == '@' {
			secretValue, found, err := secretDb.Get(value[1:])
			if !found {
				return nil, errors.New("Secret not found: " + value)
			}
			if err != nil {
				return nil, errors.New("Failed to get secret value: " + value)
			}
			value = secretValue
		}
		resolvedEnvs = append(resolvedEnvs, key+"="+value)
	}
	return resolvedEnvs, nil
}

func makeRule(config jigtypes.DeploymentConfig) string {
	switch {
	case config.Rule != "":
		return config.Rule
	case config.Domain != "":
		return "Host(`" + config.Domain + "`)"
	default:
		return "No-HTTP"
	}
}

func makeLabels(config jigtypes.DeploymentConfig) map[string]string {
	name := config.Name
	rule := makeRule(config)
	labels := map[string]string{
		"traefik.docker.network": "jig",
		"jig.name":               name,
	}
	middlewares := []string{}
	keepTLS := config.Middlewares.NoTLS == nil || !*config.Middlewares.NoTLS
	keepHTTP := config.Middlewares.NoHTTP == nil || !*config.Middlewares.NoHTTP

	// No need to have HTTPS if HTTP is disabled as well
	if keepTLS && keepHTTP {
		maps.Copy(labels, map[string]string{
			"traefik.http.middlewares.https-only.redirectscheme.permanent": "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":    "https",
			"traefik.http.routers." + name + `-secure.rule`:                rule,
			"traefik.http.routers." + name + `-secure.tls.certresolver`:    "defaultresolver",
			"traefik.http.routers." + name + `-secure.tls`:                 "true",
			"traefik.http.routers." + name + `-secure.entrypoints`:         "websecure",
		})
		middlewares = append(middlewares, "https-only")
	}
	if keepHTTP {
		maps.Copy(labels, map[string]string{
			"traefik.enable":                                "true",
			"traefik.http.routers." + name + `.rule`:        rule,
			"traefik.http.routers." + name + `.entrypoints`: "web",
		})
	} else {
		labels["traefik.enable"] = "false"
	}
	if config.Middlewares.Compression != nil && *config.Middlewares.Compression {
		// No need to rename compress middleware since it's same everywhere
		labels["traefik.http.middlewares.compress.compress"] = "true"
		middlewares = append(middlewares, "compress")
	}
	if config.Middlewares.AddPrefix != nil {
		middlewareName := "addPrefix-" + name
		labels["traefik.http.middlewares."+middlewareName+".addprefix"] = *config.Middlewares.AddPrefix
		middlewares = append(middlewares, middlewareName)
	}
	if config.Middlewares.StripPrefix != nil {
		middlewareName := "stripPrefix-" + name
		labels["traefik.http.middlewares."+middlewareName+".stripprefix.prefixes"] = strings.Join(*config.Middlewares.StripPrefix, ",")
		middlewares = append(middlewares, middlewareName)
	}
	if config.Middlewares.BasicAuth != nil {
		middlewareName := "basicAuth-" + name
		labels["traefik.http.middlewares."+middlewareName+".basicauth.users"] = strings.Join(*config.Middlewares.BasicAuth, ",")
		middlewares = append(middlewares, middlewareName)
	}
	if config.Middlewares.RateLimiting != nil {
		middlewareName := "ratelimit-" + name
		maps.Copy(labels, map[string]string{
			"traefik.http.middlewares." + middlewareName + ".ratelimit.average": fmt.Sprint(config.Middlewares.RateLimiting.Average),
			"traefik.http.middlewares." + middlewareName + ".ratelimit.burst":   fmt.Sprint(config.Middlewares.RateLimiting.Burst),
		})
		middlewares = append(middlewares, middlewareName)
	}
	if len(middlewares) > 0 {
		labels["traefik.http.routers."+name+`.middlewares`] = strings.Join(middlewares, ", ")
	}
	return labels
}

type DeploymentsRouter struct {
	cli       *client.Client
	secret_db *Secrets
}

func (dr DeploymentsRouter) Router() func(r chi.Router) {
	cli := dr.cli
	secretDb := dr.secret_db
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {

			containers, err := cli.ContainerList(context.Background(), container.ListOptions{
				All: true,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var deployments []jigtypes.Deployment = []jigtypes.Deployment{}
			for _, container := range containers {
				name, isJigDeployment := container.Labels["jig.name"]
				if !isJigDeployment {
					continue
				}
				deployments = append(deployments, jigtypes.Deployment{
					ID:       container.ID,
					Name:     container.Labels["jig.name"],
					Rule:     container.Labels["traefik.http.routers."+name+`-secure.rule`],
					Status:   container.State,
					Lifetime: container.Status,
				})
			}

			deploymentsJson, err := json.Marshal(deployments)
			if err != nil {
				println("Failed to marshal deployments", deploymentsJson, err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(deploymentsJson)
		})

		r.Post("/", func(w http.ResponseWriter, r *http.Request) {

			// Get config from header
			configString := r.Header.Get("x-jig-config")
			jigImageHeader := r.Header.Get("x-jig-image")
			isJigImage := jigImageHeader == "true"
			if configString == "" {
				http.Error(w, "Config not found", http.StatusBadRequest)
				return
			}
			var config jigtypes.DeploymentConfig
			if err := json.Unmarshal([]byte(configString), &config); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if config.Name == "" {
				http.Error(w, "Name is required", http.StatusBadRequest)
				return
			}

			// Load image from body
			if isJigImage {
				res, err := cli.ImageLoad(context.Background(), r.Body, true)
				if err != nil {
					fmt.Println("Failed to load image for deployment", config)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				defer res.Body.Close()
				data, err := io.ReadAll(res.Body)
				if !strings.Contains(string(data), "Loaded image") || err != nil {
					fmt.Println("Failed to load image for deployment", config)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				// build image from buildcontext over the request body
				buildResponse, err := cli.ImageBuild(context.Background(), r.Body, types.ImageBuildOptions{
					Tags:        []string{config.Name + ":latest"},
					Remove:      true,
					ForceRemove: true,
				})
				if err != nil {
					fmt.Println("Failed to load image for deployment", config)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				defer buildResponse.Body.Close()

				buf := bufio.NewScanner(buildResponse.Body)
				for buf.Scan() {
					jsonMessage := jsonmessage.JSONMessage{}
					json.Unmarshal(buf.Bytes(), &jsonMessage)
					jsonMessage.Display(os.Stdout, true)
					w.Write(buf.Bytes())
					w.Write([]byte{'\n'})
					w.(http.Flusher).Flush()
					if jsonMessage.Error != nil {
						buildResponse.Body.Close()
						return
					}
				}
			}

			// Check if container already exists
			containers, err := cli.ContainerList(context.Background(), container.ListOptions{
				All: true,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, containerInfo := range containers {
				if containerInfo.Labels["jig.name"] == config.Name {
					if !isJigImage {
						w.Write([]byte("\n{\"stream\": \"Container exists, stopping...\"}\n"))
						w.(http.Flusher).Flush()
					}
					fmt.Printf("Container %s exists, stopping...\n", config.Name)
					cli.ContainerStop(context.Background(), containerInfo.ID, container.StopOptions{})
					fmt.Printf("Container %s exists, removing\n", config.Name)
					cli.ContainerRemove(context.Background(), containerInfo.ID, container.RemoveOptions{})
				}
			}

			exposedPorts := map[nat.Port]struct{}{}
			if config.Port != 0 {
				exposedPorts[nat.Port(fmt.Sprint(config.Port)+"/tcp")] = struct{}{}
			}

			envs, err := makeEnvs(config.Envs, secretDb)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var restartPolicy container.RestartPolicy
			if strings.Contains(config.RestartPolicy, ":") {
				parts := strings.Split(config.RestartPolicy, ":")
				retryCount, err := strconv.Atoi(parts[1])
				if err != nil {
					println("Failed to parse retry count", err.Error())
					http.Error(w, "Failed to parse retry count", http.StatusInternalServerError)
					return
				}
				restartPolicy = container.RestartPolicy{
					Name:              container.RestartPolicyMode(parts[0]),
					MaximumRetryCount: retryCount,
				}
			} else {
				restartPolicy = container.RestartPolicy{
					Name: container.RestartPolicyMode(config.RestartPolicy),
				}
			}

			internalHostname := config.Name
			if config.Hostname != "" {
				internalHostname = config.Hostname
			}

			_, err = cli.ContainerCreate(context.Background(), &container.Config{
				ExposedPorts: exposedPorts,
				Env:          envs,
				Image:        config.Name + ":latest",
				Labels:       makeLabels(config),
				Volumes:      map[string]struct{}{},
			}, &container.HostConfig{
				RestartPolicy: restartPolicy,
			}, &network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					"jig": {
						Aliases: []string{internalHostname},
					},
				},
			}, &v1.Platform{}, config.Name)
			if err != nil {
				println("Failed to create container", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Printf("Container %s created\n", config.Name)

			err = cli.ContainerStart(context.Background(), config.Name, container.StartOptions{})
			if err != nil {
				println("Failed to start container", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Printf("Container %s started\n", config.Name)
			if !isJigImage {
				w.Write([]byte("{\"stream\": \"\\nImage built and container started\"}\n"))
				w.(http.Flusher).Flush()
			}
		})

		r.Delete("/{name}", func(w http.ResponseWriter, r *http.Request) {

			name := r.PathValue("name")
			containers, err := cli.ContainerList(context.Background(), container.ListOptions{
				All: true,
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			for _, containerInfo := range containers {
				if containerInfo.Labels["jig.name"] == name {
					cli.ContainerStop(context.Background(), containerInfo.ID, container.StopOptions{})
					cli.ContainerRemove(context.Background(), containerInfo.ID, container.RemoveOptions{})
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			http.Error(w, "Container not found", http.StatusNotFound)
		})

		r.Get("/{name}/logs", func(w http.ResponseWriter, r *http.Request) {

			name := r.PathValue("name")
			containers, err := cli.ContainerList(context.Background(), container.ListOptions{
				All: true,
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			for _, containerInfo := range containers {
				if containerInfo.Labels["jig.name"] == name {
					logs, err := cli.ContainerLogs(context.Background(), containerInfo.ID, container.LogsOptions{
						ShowStdout: true,
						ShowStderr: true,
					})
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusOK)
					w.Header().Set("Content-Type", "text/plain")
					io.Copy(w, logs)
					return
				}
			}

			http.Error(w, "Container not found", http.StatusNotFound)
		})

		r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
			containers, err := cli.ContainerList(context.Background(), container.ListOptions{Filters: filters.NewArgs(filters.KeyValuePair{Key: "label", Value: "jig.name"})})
			if err != nil {
				log.Print("Failed to list the containers")
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			var allStats []jigtypes.Stats = make([]jigtypes.Stats, 0, len(containers))
			for _, container := range containers {
				stats, err := cli.ContainerStatsOneShot(context.Background(), container.ID)
				if err != nil {
					log.Print("Failed to get container stats info")
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				body, err := io.ReadAll(stats.Body)
				if err != nil {
					log.Println("Somehow failed to read the body", err.Error())
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				defer stats.Body.Close()
				var containerStats types.StatsJSON
				if err := json.Unmarshal(body, &containerStats); err != nil {
					log.Println("Failed to unmarshal stats", err.Error())
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}

				usedMemory :=
					containerStats.MemoryStats.Usage -
						containerStats.MemoryStats.Stats["cache"]

				cpuD :=
					containerStats.CPUStats.CPUUsage.TotalUsage -
						containerStats.PreCPUStats.CPUUsage.TotalUsage

				sysCpuD :=
					containerStats.CPUStats.SystemUsage -
						containerStats.PreCPUStats.CPUUsage.TotalUsage

				cpuNum := containerStats.CPUStats.OnlineCPUs

				allStats = append(allStats, jigtypes.Stats{
					Name:             container.Names[0],
					MemoryBytes:      math.Round((float64(usedMemory)/(1024*1024))*100) / 100,
					MemoryPercentage: math.Round((float64(usedMemory)/float64(containerStats.MemoryStats.Limit))*10000) / 100,
					CpuPercentage:    math.Round((float64(cpuD)/float64(sysCpuD))*float64(cpuNum)*10000) / 100,
				})
			}

			statsJson, err := json.Marshal(allStats)
			if err != nil {
				log.Print("Failed to marshal stats")
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(statsJson)

		})
	}
}