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
	"github.com/docker/docker/api/types/mount"
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

	var configString string
	if configStringBytes, err := json.Marshal(config); err != nil {
		configString = ""
	} else {
		configString = string(configStringBytes)
	}

	labels := map[string]string{
		"traefik.docker.network": "jig",
		"jig.name":               name,
		"jig.config":             configString,
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
		labels["traefik.http.routers."+name+`-secure.middlewares`] = strings.Join(middlewares, ", ")
	}
	return labels
}

type DeploymentsRouter struct {
	cli       *client.Client
	secret_db *Secrets
}

func (d *DeploymentsRouter) getDeployments(w http.ResponseWriter, r *http.Request) {

	containers, err := d.cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var deployments []jigtypes.Deployment = []jigtypes.Deployment{}
	deploymentHasRollback := make(map[string]bool)
	for _, container := range containers {
		name, isJigDeployment := container.Labels["jig.name"]
		if !isJigDeployment {
			continue
		}
		if (container.Labels["jig.name"] + "-prev") == container.Names[0] {
			deploymentHasRollback[name] = true
		}
	}

	for _, container := range containers {
		name, isJigDeployment := container.Labels["jig.name"]
		if !isJigDeployment || (container.Labels["jig.name"]+"-prev") == container.Names[0] {
			continue
		}

		deployments = append(deployments, jigtypes.Deployment{
			ID:          container.ID,
			Name:        container.Labels["jig.name"],
			Rule:        container.Labels["traefik.http.routers."+name+`-secure.rule`],
			Status:      container.State,
			Lifetime:    container.Status,
			HasRollback: deploymentHasRollback[name],
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
}

func (d *DeploymentsRouter) runDeploy(w http.ResponseWriter, r *http.Request) {
	cli := d.cli
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

	// Check if image already exists
	images, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		log.Println("Failed to list images", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, image := range images {
		for _, tag := range image.RepoTags {
			if tag == config.Name+":latest" {
				cli.ImageTag(context.Background(), tag, config.Name+":prev")
				w.Write([]byte("Image exists, tagging as prev for rollback\n"))
			}
		}
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

	// Check if rollback container already exists
	rollbackContainer, err := containerExistsWithName(cli, config.Name+"-prev")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rollbackContainer != nil {
		if !isJigImage {
			w.Write([]byte("\n{\"stream\": \"Rollback container exists, removing...\"}\n"))
			w.(http.Flusher).Flush()
		}
		fmt.Printf("Rollback container %s exists, stopping...\n", config.Name)
		cli.ContainerStop(context.Background(), rollbackContainer.ID, container.StopOptions{})
		fmt.Printf("Rollback container %s exists, removing\n", config.Name)
		cli.ContainerRemove(context.Background(), rollbackContainer.ID, container.RemoveOptions{})

	}

	currentContainer, err := containerExistsWithName(cli, config.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if currentContainer != nil {
		if !isJigImage {
			w.Write([]byte("\n{\"stream\": \"Current container exists, renaming...\"}\n"))
			w.(http.Flusher).Flush()
		}
		fmt.Printf("Container %s exists, using it as a rollback...\n", config.Name)
		err = cli.ContainerStop(context.Background(), currentContainer.ID, container.StopOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			fmt.Printf("Failed to stop container %s: %s\n", config.Name, err.Error())
			return
		}

		fmt.Printf("Container %s exists, using it as a rollback...\n", config.Name)
		err = cli.ContainerRename(context.Background(), currentContainer.ID, config.Name+"-prev")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			fmt.Printf("Failed to rename container %s: %s\n", config.Name, err.Error())
			return
		}
	}

	exposedPorts := map[nat.Port]struct{}{}
	if config.Port != 0 {
		exposedPorts[nat.Port(fmt.Sprint(config.Port)+"/tcp")] = struct{}{}
	}

	envs, err := makeEnvs(config.Envs, d.secret_db)
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

	mounts := []mount.Mount{}

	if config.Volumes != nil {
		for _, volume := range config.Volumes {
			parts := strings.Split(volume, ":")
			if parts[0] == "" || parts[1] == "" {
				http.Error(w, "Invalid volume", http.StatusBadRequest)
				return
			}
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: parts[0],
				Target: parts[1],
			})
		}
	}

	_, err = cli.ContainerCreate(context.Background(), &container.Config{
		ExposedPorts: exposedPorts,
		Env:          envs,
		Image:        config.Name + ":latest",
		Labels:       makeLabels(config),
	}, &container.HostConfig{
		RestartPolicy: restartPolicy,
		Mounts:        mounts,
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
}

func (d *DeploymentsRouter) deleteDeploy(w http.ResponseWriter, r *http.Request) {

	name := r.PathValue("name")
	containers, err := d.cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, containerInfo := range containers {
		if containerInfo.Labels["jig.name"] == name {
			d.cli.ContainerStop(context.Background(), containerInfo.ID, container.StopOptions{})
			d.cli.ContainerRemove(context.Background(), containerInfo.ID, container.RemoveOptions{})
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "Container not found", http.StatusNotFound)
}

func (d *DeploymentsRouter) rollbackDeployment(w http.ResponseWriter, r *http.Request) {
	cli := d.cli
	name := r.PathValue("name")

	currentDeployment, err := containerExistsWithName(cli, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	if currentDeployment == nil {
		log.Printf("Original deployment not found: %s", name)
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}
	rollbackTarget, err := containerExistsWithName(cli, name+"-prev")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	if rollbackTarget == nil {
		log.Printf("Container not found for rollback: %s", name)
		http.Error(w, "Rollback targer doesn't exist", http.StatusNotFound)
		return
	}

	err = cli.ContainerStop(context.Background(), currentDeployment.ID, container.StopOptions{})
	if err != nil {
		http.Error(w, "Failed to stop container", http.StatusInternalServerError)
		return
	}
	err = cli.ContainerRename(context.Background(), currentDeployment.ID, name+"-old")
	if err != nil {
		http.Error(w, "Failed to rename current container", http.StatusInternalServerError)
		return
	}

	err = cli.ContainerRename(context.Background(), rollbackTarget.ID, name)
	if err != nil {
		http.Error(w, "Failed to rename rollback container", http.StatusInternalServerError)
		return
	}
	err = cli.ContainerStart(context.Background(), name, container.StartOptions{})
	if err != nil {
		http.Error(w, "Failed to start rollback container, trying to restart current deployment", http.StatusInternalServerError)
		cli.ContainerRename(context.Background(), currentDeployment.ID, name)
		cli.ContainerStart(context.Background(), name, container.StartOptions{})
		return
	}

	err = cli.ContainerRemove(context.Background(), currentDeployment.ID, container.RemoveOptions{})
	if err != nil {
		http.Error(w, "Failed to remove old container", http.StatusInternalServerError)
		return
	}

}

func (dr *DeploymentsRouter) getDeploymentLogs(w http.ResponseWriter, r *http.Request) {

	name := r.PathValue("name")
	containers, err := dr.cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, containerInfo := range containers {
		if containerInfo.Labels["jig.name"] == name {
			logs, err := dr.cli.ContainerLogs(context.Background(), containerInfo.ID, container.LogsOptions{
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
}

func (dr *DeploymentsRouter) getDeploymentStats(w http.ResponseWriter, r *http.Request) {
	containers, err := dr.cli.ContainerList(context.Background(), container.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "label", Value: "jig.name"}),
	})
	if err != nil {
		log.Print("Failed to list the containers")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	var allStats []jigtypes.Stats = make([]jigtypes.Stats, 0, len(containers))
	for _, container := range containers {
		stats, err := dr.cli.ContainerStatsOneShot(context.Background(), container.ID)

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

}

func (dr DeploymentsRouter) Router() (r chi.Router) {
	r = chi.NewRouter()
	r.Get("/", dr.getDeployments)

	r.Post("/", dr.runDeploy)

	r.Delete("/{name}", dr.deleteDeploy)

	r.Post("/{name}/rollback", dr.rollbackDeployment)

	r.Get("/{name}/logs", dr.getDeploymentLogs)

	r.Get("/stats", dr.getDeploymentStats)
	return r
}

func containerExistsWithName(cli *client.Client, name string) (*types.Container, error) {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}
	for _, containerInfo := range containers {

		namesInclude := false
		for _, containerName := range containerInfo.Names {
			if containerName == "/"+name {
				namesInclude = true
				break
			}
		}

		if containerInfo.Labels["jig.name"] == name || namesInclude {
			return &containerInfo, nil
		}
	}
	return nil, nil
}
