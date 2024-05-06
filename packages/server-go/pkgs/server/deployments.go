package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/go-chi/chi/v5"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func makeLabels(name string, rule string) map[string]string {
	return map[string]string{
		"traefik.enable":                  "true",
		name + `.rule`:                    rule,
		name + `.middlewares`:             "https-only",
		name + `.entrypoints`:             "web",
		name + `-secure.rule`:             rule,
		name + `-secure.tls.certresolver`: "defaultresolver",
		name + `-secure.tls`:              "true",
		name + `-secure.entrypoints`:      "websecure",
		"jig.name":                        name,
	}
}

func makeEnvs(newenvs map[string]string, secretDb *Secrets) ([]string, error) {
	resolvedEnvs := []string{}
	for key, value := range newenvs {
		if value[0] == '@' {
			secretValue, err := secretDb.Get(value[1:])
			if err != nil {
				return nil, errors.New("Failed to get secret value: " + value)
			}
			value = secretValue
		}
		resolvedEnvs = append(resolvedEnvs, key+"="+value)
	}
	return resolvedEnvs, nil
}

func makeRule(config types.DeploymentConfig) string {
	switch true {
	case config.Rule != "":
		return config.Rule
	case config.Domain != "":
		return "Host(`" + config.Domain + "`)"
	default:
		return "No-HTTP"
	}
}

func DeploymentsRouter(cli *client.Client, secretDb *Secrets) func(chi.Router) {

	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {

			containers, err := cli.ContainerList(context.Background(), container.ListOptions{
				All: true,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var deployments []types.Deployment = []types.Deployment{}
			for _, container := range containers {
				name, isJigDeployment := container.Labels["jig.name"]
				if !isJigDeployment {
					continue
				}
				deployments = append(deployments, types.Deployment{
					ID:     container.ID,
					Name:   container.Labels["jig.name"],
					Rule:   container.Labels[name+`-secure.rule`],
					Status: container.State,
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

			envs, err := makeEnvs(config.Envs, secretDb)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			restartPolicy := container.RestartPolicy{
				Name: container.RestartPolicyMode(config.RestartPolicy),
			}

			createdContainer, err := cli.ContainerCreate(context.Background(), &container.Config{
				ExposedPorts: exposedPorts,
				Env:          envs,
				Labels:       makeLabels(config.Name, makeRule(config)),
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
					io.Copy(w, logs)
					return
				}
			}

			http.Error(w, "Container not found", http.StatusNotFound)
		})
	}
}
