package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"askh.at/jig/v2/pkgs/types"
	"github.com/urfave/cli/v2"
)

type Config struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

var config = Config{
	Endpoint: os.Getenv("JIG_ENDPOINT"),
	Token:    os.Getenv("JIG_TOKEN"),
}

func loadFileConfig() error {
	// Load config from file
	homedir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configFile, err := os.ReadFile(homedir + "/.jig/config.json")
	if err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			return nil
		}
	}

	err = json.Unmarshal(configFile, &config)
	if err != nil {
		log.Fatal(err)
	}
	println(config.Endpoint, config.Token)
	return nil
}

func enrichConfigFromFlags(cli *cli.Context) {
	if cli.String("endpoint") != "" {
		config.Endpoint = cli.String("endpoint")
	}
	if cli.String("token") != "" {
		config.Token = cli.String("token")
	}
}

var httpClient = &http.Client{}

func createRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, config.Endpoint+url, nil)
	req.Header.Set("Authorization", "Bearer "+config.Token)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func main() {
	loadFileConfig()
	app := &cli.App{
		Name: "jig",
		Commands: []*cli.Command{
			{
				Name:  "ls",
				Usage: "List deployments",
				Action: func(ctx *cli.Context) error {
					enrichConfigFromFlags(ctx)
					req, _ := createRequest("GET", "/deployments")
					resp, err := httpClient.Do(req)
					if err != nil {
						log.Fatal("Error making request: ", err)
					}
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						log.Fatal("Error reading request: ", err)
					}
					var deployments []types.Deployment
					err = json.Unmarshal(body, &deployments)

					if err != nil {
						println("Error unmarshalling response: ", string(body))
						log.Fatal("Error unmarshalling response: ", err)
					}

					writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
					fmt.Fprintln(writer, "ID\tNAME\tRULE\tSTATUS")
					for _, deployment := range deployments {
						fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", deployment.ID, deployment.Name, deployment.Rule, deployment.Status)
					}
					writer.Flush()
					return nil
				},
			},
			{
				Name:  "secrets",
				Usage: "List secrets",
				Subcommands: []*cli.Command{
					{
						Name:  "ls",
						Usage: "List secrets",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)
							req, _ := createRequest("GET", "/secrets")
							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							body, err := io.ReadAll(resp.Body)
							if err != nil {
								log.Fatal("Error reading request: ", err)
							}
							var secretList types.SecretList
							err = json.Unmarshal(body, &secretList)

							if err != nil {
								println("Error unmarshalling response: ", string(body))
								log.Fatal("Error unmarshalling response: ", err)
							}

							writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
							fmt.Fprintln(writer, "NAME")
							for _, secret := range secretList.Secrets {
								fmt.Fprintf(writer, "%s\n", secret)
							}
							writer.Flush()
							return nil
						},
					},
					{
						Name:  "inspect",
						Usage: "Inspect a secret",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)

							name := ctx.Args().Get(0)

							req, _ := createRequest("GET", "/secrets/"+name+"/")

							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error reading secret: ", err.Error())
							}
							if resp.StatusCode != 204 {
								log.Fatal("Error reading secret: ", resp.Status)
							}

							println(io.ReadAll(resp.Body))
							return nil
						},
					},

					{
						Name:  "add",
						Args:  true,
						Usage: "add a secret",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)

							bodyToSend := types.NewSecretBody{
								Name:  ctx.Args().Get(0),
								Value: ctx.Args().Get(1),
							}

							bodyBytes, err := json.Marshal(bodyToSend)
							if err != nil {
								log.Fatal("Error marshalling body: ", err)
							}
							req, _ := createRequest("POST", "/secrets")
							req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							if resp.StatusCode != 201 {
								log.Fatal("Error creating secret: ", resp.Status)
							}

							println("Successfully created a secret ✨")
							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "delete a secret",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)

							name := ctx.Args().Get(0)

							req, _ := createRequest("DELETE", "/secrets/"+name+"/")

							resp, err := httpClient.Do(req)
							println(req.Method, req.URL.String())
							if err != nil {
								log.Fatal("Error making request: ", err)
								log.Fatal("Error deleting secret: ", resp.Status)
								println(io.ReadAll(resp.Body))
							}
							if resp.StatusCode != 204 {
								log.Fatal("Error deleting secret: ", resp.Status)
								println(io.ReadAll(resp.Body))
							}

							println("Successfully removed a secret ❌")
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
