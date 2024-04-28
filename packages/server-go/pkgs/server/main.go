package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"
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
	}
	if containerId != "" {
		if !isRunning {
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
	app := &cli.App{
		Name:  "Jig",
		Usage: "Deployment Docker wtapper",
		Action: func(c *cli.Context) error {
			serve()
			return nil
		},
		Commands: []*cli.Command{

			{
				Name:  "makeKey",
				Usage: "Generate a new JWT secret key",
				Args:  true,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "short",
						Aliases: []string{"s"},
						Value:   false,
						Usage:   "Short output",
					},
				},
				Action: func(ctx *cli.Context) error {
					// Create the Claims
					claims := &jwt.RegisteredClaims{
						Issuer:  "jig",
						Subject: "admin",
					}

					token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
					token.Header["alg"] = "HS256"
					ss, err := token.SignedString([]byte(jwtSecretKey))

					if err != nil {
						return err
					}

					if ctx.Bool("short") {
						print(ss)
					} else {
						println("Your new jwt secret key ✨🔑:")
						println(ss)
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func serve() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		print("Failed to connect to docker daemon")
		return
	}

	if err := ensureTraefikRunning(cli); err != nil {
		println("Failed to ensure traefik is running")
		panic(err)
	}

	secretDb, err := InitSecrets()
	if err != nil {
		println("Failed to initialize secret_db")
		panic(err)
	}
	defer secretDb.Close()

	print("Listening on 8080")
	http.Handle("/deployments", deploymentsRouter(*cli, secretDb))
	http.Handle("/secrets", secretRouter(secretDb))
	http.ListenAndServe("0.0.0.0:8080", nil)
}

func secretRouter(secret_db *Secrets) *http.ServeMux {
	r := http.NewServeMux()

	r.HandleFunc("POST /secrets", func(w http.ResponseWriter, r *http.Request) {

		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		var body types.NewSecretBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := secret_db.Insert(body.Name, body.Value); err != nil {
			println("Failed to insert secret", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	})

	r.HandleFunc("DELETE /secrets/{name}/", func(w http.ResponseWriter, r *http.Request) {
		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		name := r.PathValue("name")

		err := secret_db.Delete(name)
		if err != nil {
			println("Failed to delete secret", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	})

	r.HandleFunc("GET /secrets", func(w http.ResponseWriter, r *http.Request) {
		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		secrets, err := secret_db.List()

		if err != nil {
			println("Failed to list secrets", err.Error())
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

	r.HandleFunc("GET /secrets/{name}/", func(w http.ResponseWriter, r *http.Request) {

		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

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

	return r
}

func deploymentsRouter(cli client.Client, secretDb *Secrets) *http.ServeMux {
	r := http.NewServeMux()
	r.HandleFunc("GET /deployments", func(w http.ResponseWriter, r *http.Request) {

		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

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

		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

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
			if value[0] == '@' {
				secretValue, err := secretDb.Get(value[1:])
				if err != nil {
					println("Failed to get secret value", err.Error())
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				value = secretValue
			}
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

		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

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

		if err := ensureAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

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

	return r
}

func ensureAuth(req *http.Request) error {
	parts := strings.Split(req.Header.Get("Authorization"), " ")
	if len(parts) != 2 {
		return fmt.Errorf("invalid Authorization header")
	}

	if parts[0] != "Bearer" {
		return fmt.Errorf("invalid Authorization header")
	}

	tokenString := parts[1]

	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecretKey), nil
	})

	if err != nil {
		println("Failed to parse token", err.Error())
		return err
	}

	if !token.Valid {
		return fmt.Errorf("invalid token")
	}

	return nil
}

var jwtSecretKey = os.Getenv("JIG_SECRET")

func InitSecrets() (*Secrets, error) {
	println("db.go initialized")
	if _, err := os.Stat("./secrets.db"); errors.Is(err, os.ErrNotExist) {
		_, err := os.Create("./secrets.db")
		if err != nil {
			println("Failed to create db file", err.Error())
			return nil, err
		}
	}
	newDb, err := sql.Open("sqlite3", "./secrets.db")
	if err != nil {
		return nil, err
	}
	_, err = newDb.Exec("CREATE TABLE IF NOT EXISTS secrets (id INTEGER PRIMARY KEY, name TEXT, value TEXT)")
	if err != nil {
		return nil, err
	}
	return &Secrets{db: newDb}, nil
}

type Secrets struct {
	db *sql.DB
}

func (secrets *Secrets) Insert(name, value string) error {
	_, err := secrets.db.Exec("INSERT INTO secrets (name, value) VALUES (?, ?)", name, value)
	return err
}

func (secrets *Secrets) Get(name string) (string, error) {
	var value string
	err := secrets.db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	return value, err
}

func (secrets *Secrets) GetValue(name string) (string, error) {
	var value string
	err := secrets.db.QueryRow("SELECT value FROM secrets WHERE name = ?", name).Scan(&value)
	return value, err
}

func (secrets *Secrets) Update(name, value string) error {
	_, err := secrets.db.Exec("UPDATE secrets SET value = ? WHERE name = ?", value, name)
	return err
}

func (secrets *Secrets) Delete(name string) error {
	_, err := secrets.db.Exec("DELETE FROM secrets WHERE name = ?", name)
	return err
}

func (secrets *Secrets) List() ([]string, error) {
	rows, err := secrets.db.Query("SELECT name FROM secrets")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func (secrets *Secrets) Close() error {
	return secrets.db.Close()
}
