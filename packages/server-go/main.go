package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	secret_db "askh.at/jig/v2/pkgs/secrets"
	"askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	_ "github.com/mattn/go-sqlite3"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func ensureTraefikRunning(cli *client.Client) error {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		return err
	}
	var containerId string = ""
	var isRunning bool = false
	for _, container := range containers {
		if container.Names[0] == "/traefik" {
			containerId = container.ID
			if container.State == "running" {
				isRunning = true
			}

		}
		println(container.Names[0], container.State)
	}
	if containerId != "" {
		println("Container exists")
		if !isRunning {
			println("Container exists but is not running")
			if err := cli.ContainerStart(context.Background(), containerId, container.StartOptions{}); err != nil {
				println("Failed to restart contnainer, removing...", err.Error())
				cli.ContainerRemove(context.Background(), containerId, container.RemoveOptions{})
				return ensureTraefikRunning(cli)
			}
		}
	} else {
		containerCreated, err := cli.ContainerCreate(context.Background(), &container.Config{
			Image: "traefik:latest",
			Cmd: []string{
				"--api.insecure=true",
				"--entrypoints.web.address=:80",
				"--entrypoints.websecure.address=:443",
				"--providers.docker=true",
				"--providers.docker.exposedbydefault=false",
				"--certificatesresolvers.defaultresolver=true",
				"--certificatesresolvers.defaultresolver.acme.httpchallenge=true",
				"--certificatesresolvers.defaultresolver.acme.httpchallenge.entrypoint=web",
				"--certificatesresolvers.defaultresolver.acme.email=" + os.Getenv("JIG_SSL_EMAIL"),
				"--certificatesresolvers.defaultresolver.acme.storage=/var/jig/acme.json",
			},
		}, &container.HostConfig{
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
			Binds: []string{
				"/var/run/docker.sock:/var/run/docker.sock",
				"/var/jig:/var/jig",
			},
			PortBindings: map[nat.Port][]nat.PortBinding{
				"80/tcp":   {{HostPort: "80"}},
				"8080/tcp": {{HostPort: "8080"}},
				"443/tcp":  {{HostPort: "443"}},
			},
		}, &network.NetworkingConfig{}, &v1.Platform{}, "traefik")
		if err != nil {
			println("Failed to create container", err.Error())
			return err
		}
		println("Container created ", containerCreated.ID)
		if err := cli.ContainerStart(context.Background(), containerCreated.ID, container.StartOptions{}); err != nil {
			println("Failed to start container", err.Error())
			return err
		}
	}
	return nil
}

func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		print("Failed to connect to docker daemon")
		return
	}

	if err := ensureTraefikRunning(cli); err != nil {
		println("Failed to ensure traefik is running")
		panic(err)
	}

	if err := secret_db.Init(); err != nil {
		println("Failed to initialize secret_db")
		panic(err)
	}
	defer secret_db.Close()
	r := http.NewServeMux()

	r.HandleFunc("GET /deployments", func(w http.ResponseWriter, r *http.Request) {
		containers, err := cli.ContainerList(context.Background(), container.ListOptions{
			All: true,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var deployments []types.Deployment = []types.Deployment{}
		for _, container := range containers {
			if container.Labels["jig.name"] != "" {
				name := container.Labels["jig.name"]
				deploy := types.Deployment{
					ID:     container.ID,
					Name:   container.Labels["jig.name"],
					Rule:   container.Labels[name+`-secure.rule`],
					Status: container.State,
				}
				deployments = append(deployments, deploy)
			}
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

	r.HandleFunc("POST /deployments", func(w http.ResponseWriter, r *http.Request) {

		// Get config from header
		configString := r.Header.Get("x-jig-config")
		var config types.DeploymentConfig
		if err := json.Unmarshal([]byte(configString), &config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Load image from body
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
				cli.ContainerStop(context.Background(), containerInfo.ID, container.StopOptions{})
				return
			}
		}

		exposedPorts := map[nat.Port]struct{}{}
		exposedPorts[nat.Port(fmt.Sprint(config.Port)+"/tcp")] = struct{}{}

		rule := ""
		switch true {
		case config.Rule != "":
			rule = config.Rule
		case config.Domain != "":
			rule = "Host(`" + config.Domain + "`)"
		default:
			http.Error(w, "Rule or Domain is required", http.StatusBadRequest)
			return
		}

		labels := map[string]string{}
		labels["traefik.enable"] = "true"
		labels[config.Name+`.rule`] = rule
		labels[config.Name+`.middlewares`] = "https-only"
		labels[config.Name+`.entrypoints`] = "web"
		labels[config.Name+`-secure.rule`] = rule
		labels[config.Name+`-secure.tls.certresolver`] =
			"defaultresolver"
		labels[config.Name+`-secure.tls`] = "true"
		labels[config.Name+`-secure.entrypoints`] = "websecure"
		labels["jig.name"] = config.Name

		envs := []string{}
		for key, value := range config.Envs {
			envs = append(envs, key+"="+value)
		}

		restartPolicy := container.RestartPolicy{
			Name: container.RestartPolicyMode(config.RestartPolicy),
		}

		createdContainer, err := cli.ContainerCreate(context.Background(), &container.Config{
			ExposedPorts: exposedPorts,
			Env:          envs,
		}, &container.HostConfig{
			RestartPolicy: restartPolicy,
		}, &network.NetworkingConfig{}, &v1.Platform{}, config.Name)
		if err != nil {
			println("Failed to create container", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		err = cli.ContainerStart(context.Background(), createdContainer.ID, container.StartOptions{})
		if err != nil {
			println("Failed to start container", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	})

	r.HandleFunc("DELETE /deployments/{name}", func(w http.ResponseWriter, r *http.Request) {
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
				return
			}
		}

		http.Error(w, "Container not found", http.StatusNotFound)
	})

	r.HandleFunc("GET /deployments/{name}/logs", func(w http.ResponseWriter, r *http.Request) {
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
				io.Copy(w, logs)
				return
			}
		}

		http.Error(w, "Container not found", http.StatusNotFound)
	})

	r.HandleFunc("POST /secrets", func(w http.ResponseWriter, r *http.Request) {
		var body types.NewSecretBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := secret_db.Insert(body.Name, body.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	})

	r.HandleFunc("GET /secrets", func(w http.ResponseWriter, r *http.Request) {
		secrets, err := secret_db.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		secretList := types.SecretList{Secrets: secrets}

		secretsJson, err := json.Marshal(secretList)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(secretsJson)
	})

	r.HandleFunc("GET /secrets/{name}/inspect", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		secret, err := secret_db.Get(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var secretList types.SecretInspect = types.SecretInspect{Value: secret}

		secretJson, err := json.Marshal(secretList)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(secretJson)
	})

	print("Listening on 8080")
	http.Handle("/", r)
	http.ListenAndServe("0.0.0.0:8080", nil)
}
