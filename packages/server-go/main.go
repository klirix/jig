package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	secret_db "askh.at/jig/v2/pkgs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func ensureTraefikRunning(cli *client.Client) error {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{})
	if err != nil {
		return err
	}
	var traefikRunning bool = false
	for _, container := range containers {
		if container.Names[0] == "/traefik" {
			traefikRunning = true
		}
	}
	if !traefikRunning {
		_, err := cli.ContainerCreate(context.Background(), &container.Config{
			Image: "traefik:v2.11",
		}, &container.HostConfig{}, &network.NetworkingConfig{}, &v1.Platform{}, "traefik")
		if err != nil {
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

	// if err := ensureTraefikRunning(cli); err != nil {
	// 	println("Failed to ensure traefik is running")
	// 	panic(err)
	// }

	if err := secret_db.Init(); err != nil {
		println("Failed to initialize secret_db")
		panic(err)
	}
	defer secret_db.Close()
	r := mux.NewRouter()
	r.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		containers, err := cli.ContainerList(context.Background(), container.ListOptions{
			All: true,
		})
		if err != nil {
			panic(err)
		}
		for _, container := range containers {
			println(container.ID, container.Image, container.Names[0])
		}
		cli.ImageLoad(context.Background(), r.Body, false)
	})
	r.HandleFunc("/deployments/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			var deployment Deployment
			if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			deployments[deployment.Name] = deployment
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	r.HandleFunc("/deployments/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := mux.Vars(r)["name"]
		switch r.Method {
		case "POST":
			deployment, foundDeployment := deployments[name]
			if !foundDeployment {
				http.Error(w, "Deployment not found", http.StatusNotFound)
				fmt.Println(deployments)
				return
			}
			res, err := cli.ImageLoad(context.Background(), r.Body, true)
			defer res.Body.Close()
			if err != nil {
				fmt.Println("Failed to load image for deployment", deployment)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data, err := io.ReadAll(res.Body)
			if !strings.Contains(string(data), "Loaded image") || err != nil {
				fmt.Println("Failed to load image for deployment", deployment)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			cli.ContainerStop(context.Background(), name, container.StopOptions{
				Timeout: &[]int{2}[0],
			})

			creationRes, err := cli.ContainerCreate(context.Background(), &container.Config{}, &container.HostConfig{}, &network.NetworkingConfig{}, &v1.Platform{}, "lmao")

		}
	})
	print("Listening on 8080")
	http.Handle("/", r)
	http.ListenAndServe("0.0.0.0:8080", nil)
}

type Deployment struct {
	Name          string            `json:"name"`
	Port          int               `json:"port"`
	RestartPolicy string            `json:"restartPolicy"`
	Domain        string            `json:"domain"`
	Envs          map[string]string `json:"envs"`
}

var deployments = map[string]Deployment{}
