package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"
	_ "modernc.org/sqlite"
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
	imageList, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		println("Failed to list images", err.Error())
		return err
	}
	hasTraefikImage := false
	for _, image := range imageList {
		if len(image.RepoTags) == 0 {
			continue
		}
		if strings.Contains(image.RepoTags[0], "traefik") {
			hasTraefikImage = true
		}
	}
	if !hasTraefikImage {
		println("Pulling traefik image")
		cli.ImagePull(context.Background(), "traefik:2.11", types.ImagePullOptions{})
		println("Pulled traefik image, waiting for it to settle")
		time.Sleep(4 * time.Second)
		return ensureTraefikRunning(cli)
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
		envs := []string{}
		commands := []string{
			"--api.insecure=true",
			"--log.level=DEBUG",
			"--entrypoints.web.address=:80",
			"--entrypoints.websecure.address=:443",
			"--providers.docker=true",
			"--providers.docker.exposedbydefault=false",
			"--certificatesresolvers.defaultresolver=true",
			"--certificatesresolvers.defaultresolver.acme.email=" + os.Getenv("JIG_SSL_EMAIL"),
			"--certificatesresolvers.defaultresolver.acme.storage=/var/jig/acme.json",
		}
		if os.Getenv("JIG_VERCEL_APIKEY") != "" {
			commands = append(commands,
				"--certificatesresolvers.defaultresolver.acme.dnschallenge.provider=vercel",
				"--certificatesresolvers.defaultresolver.acme.dnschallenge.delaybeforecheck=2")
			envs = append(envs, "VERCEL_API_TOKEN="+os.Getenv("JIG_VERCEL_APIKEY"))
		} else {
			commands = append(commands,
				"--certificatesresolvers.defaultresolver.acme.httpchallenge=true",
				"--certificatesresolvers.defaultresolver.acme.httpchallenge.entrypoint=web")
		}
		containerCreated, err := cli.ContainerCreate(context.Background(), &container.Config{
			Image: "traefik:2.11",
			Cmd:   commands,
			ExposedPorts: map[nat.Port]struct{}{
				"80/tcp":   {},
				"443/tcp":  {},
				"8080/tcp": {},
			},
			Env: envs,
		}, &container.HostConfig{
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
			Binds: []string{
				"/var/run/docker.sock:/var/run/docker.sock",
				"/var/jig:/var/jig",
			},
			PortBindings: map[nat.Port][]nat.PortBinding{
				"80/tcp":   {{HostPort: "80/tcp"}},
				"443/tcp":  {{HostPort: "443/tcp"}},
				"8080/tcp": {{HostPort: "8080/tcp"}},
			},
		}, &network.NetworkingConfig{}, &v1.Platform{}, "traefik")

		if err != nil {
			println("Failed to create container", err.Error())
			return err
		}

		err = cli.NetworkConnect(context.Background(), "jig", containerCreated.ID, &network.EndpointSettings{})
		if err != nil {
			println("Failed to connect to network", err.Error())
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
					ss, err := MakeKey()
					if err != nil {
						return err
					}
					if !ctx.Bool("short") {
						print("Your new jwt secret key ✨🔑:\njig login ")
					}

					fmt.Printf("https://%s+%s\n", os.Getenv("JIG_DOMAIN"), ss)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func MakeKey() (string, error) {
	claims := &jwt.RegisteredClaims{
		Issuer:  "jig",
		Subject: "admin",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["alg"] = "HS256"
	token.Header["kid"] = uuid.New().String()
	ss, err := token.SignedString([]byte(jwtSecretKey))
	if err != nil {
		return "", err
	}
	return ss, nil
}

func ensureNetworkIsUp(cli *client.Client) error {
	networks, err := cli.NetworkList(context.Background(), types.NetworkListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "name", Value: "jig"}),
	})
	if err != nil {
		return err
	}
	for _, network := range networks {
		if network.Name == "jig" {
			return nil
		}
	}
	log.Print("Network jig doesn't exist, creating network")
	_, err = cli.NetworkCreate(context.Background(), "jig", types.NetworkCreate{Driver: "bridge"})
	if err != nil {
		return nil
	}
	return nil
}

func serve() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Println("Failed to connect to docker daemon")
		return
	}

	if err := ensureNetworkIsUp(cli); err != nil {
		log.Println("Failed to ensure bridge network is running")
		panic(err)
	}

	if err := ensureTraefikRunning(cli); err != nil {
		log.Println("Failed to ensure traefik is running")
		panic(err)
	}

	secretDb, err := InitSecretsWithName("/var/jig/secrets.db")
	if err != nil {
		log.Println("Failed to initialize secret_db")
		panic(err)
	}
	defer secretDb.Close()

	log.Println("Listening on 5000")
	http.ListenAndServe("0.0.0.0:5000", mainRouter(cli, secretDb))
}

func mainRouter(cli *client.Client, secret_db *Secrets) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.With(ensureAuth).Route("/secrets", SecretRouter{secret_db}.Router())

	r.With(ensureAuth).Route("/deployments", DeploymentsRouter{cli, secret_db}.Router())

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hewwo!"))
	})

	return r
}

func ensureAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.Header.Get("Authorization"), " ")
		if len(parts) != 2 {
			log.Println("Invalid Authorization header")
			http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
			return
		}

		if parts[0] != "Bearer" {
			log.Println("Invalid Authorization header")
			http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		claims := &jwt.RegisteredClaims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(jwtSecretKey), nil
		})

		if err != nil {
			log.Println("Failed to parse token")
			http.Error(w, "Failed to parse token", http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			log.Println("Invalid token")
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var jwtSecretKey = os.Getenv("JIG_SECRET")
