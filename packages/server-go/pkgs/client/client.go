package main

import (
	"archive/tar"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
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

var defaultIgnorePatterns = []string{".git", ".gitignore", ".jig", "docker-compose.yml", "node_modules/**"}

func loadIgnorePatterns(ignoreFile string) []string {
	_, err := os.Stat(ignoreFile)
	if err != nil {
		return defaultIgnorePatterns
	}
	ignorePatterns := []string{}
	file, err := os.Open(ignoreFile)
	if err != nil {
		log.Fatal(err)
	}
	ignoreFileContents, err := io.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	ignorePatterns = strings.Split(string(ignoreFileContents), "\n")
	filteredIgnorePatterns := []string{}
	for _, v := range ignorePatterns {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			filteredIgnorePatterns = append(filteredIgnorePatterns, trimmed)
		}
	}
	return filteredIgnorePatterns
}

func filterFiles(matches []string, ignorePatterns []string) []string {
	filesToPack := []string{}
	for _, filename := range matches {
		matchedIgnore := false
		for _, ignorePattern := range ignorePatterns {
			matched, err := filepath.Match(ignorePattern, filename)
			if err != nil {
				log.Fatal("Failed to match the pattern", err.Error())
			}
			if matched {
				matchedIgnore = true
			}
		}
		if !matchedIgnore {
			filesToPack = append(filesToPack, filename)
		}
	}
	return filesToPack
}

func writeFileToTar(filename string, tw *tar.Writer) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		log.Fatal(err)
	}
	hdr := &tar.Header{
		Name: filename,
		Mode: int64(fileInfo.Mode()),
		Size: fileInfo.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		log.Fatal(err)
	}

	file, err := os.Open(filename)
	if err != nil {
		log.Fatal("File is not readable", err)
	}

	defer file.Close()
	io.Copy(tw, file)

	if err != nil {
		log.Fatalf("Could not copy the file '%s' data to the tarball, got error '%s'", filename, err.Error())
	}
}

func loginCommand(c *cli.Context) error {
	token := c.Args().Get(0)
	strings.Split(token, "+")
	newConfig := Config{
		Endpoint: strings.Split(token, "+")[0],
		Token:    strings.Split(token, "+")[1],
	}
	config = newConfig
	configJson, err := json.Marshal(newConfig)
	if err != nil {
		log.Fatal("Error marshalling new config", err)
	}
	req, _ := createRequest("GET", "/deployments")
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal("Error making request: ", err)
	}
	if resp.StatusCode != 200 {
		log.Fatal("Error connecting to jig ", resp.Status)
	}
	homedir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to get home dir", err)
	}
	os.Mkdir(homedir+"/.jig", 0755)
	os.WriteFile(homedir+"/.jig/config.json", configJson, 0644)
	println("Successfully logged in ✨")
	return nil
}

func deployComment(c *cli.Context) error {

	_, err := os.Stat("./jig.json")
	if err != nil {
		log.Fatal("No jig.json file found in the current directory")
	}
	configContents, err := os.ReadFile("./jig.json")
	if err != nil {
		log.Fatal("Failed to read jig.json", err)
	}
	var deploymentConfig jigtypes.DeploymentConfig
	err = json.Unmarshal(configContents, &deploymentConfig)
	if err != nil {
		log.Fatal("Failed to parse jig.json", err)
	}
	if deploymentConfig.Name == "" {
		log.Fatal("Name is required in jig.json")
	}
	cleanedconfig := strings.ReplaceAll(string(configContents), "\n", "")

	println("Deploying container")

	ctx := c.Context
	// docker.ImageBuild(ctx)
	verbose := c.Bool("verbose")

	ignorePatterns := loadIgnorePatterns(".jigignore")

	matches, err := filepath.Glob("**")
	if err != nil {
		log.Fatal("Failed to glob the directory", err.Error())
	}

	filesToPack := filterFiles(matches, ignorePatterns)

	if verbose {
		println("Files to pack:")
		for _, file := range filesToPack {
			println("-", file)
		}
	}

	reader, writer := io.Pipe()

	go func() {
		tw := tar.NewWriter(writer)

		for _, filename := range filesToPack {
			writeFileToTar(filename, tw)
		}
		if tw.Close() != nil {
			log.Fatal(err)
		}
		writer.Close()
	}()

	var uploadStream io.ReadCloser = reader

	localBuild := c.Bool("local")

	if localBuild {
		docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Fatal("Failed to create docker client", err)
		}
		defer docker.Close()

		buildResponse, err := docker.ImageBuild(ctx, reader, types.ImageBuildOptions{
			Tags:   []string{deploymentConfig.Name + ":latest"},
			Remove: true,
		})
		if err != nil {
			log.Fatal("Failed to request image build", err.Error())
		}
		defer buildResponse.Body.Close()

		buf := bufio.NewScanner(buildResponse.Body)
		for buf.Scan() {
			jsonMessage := jsonmessage.JSONMessage{}
			json.Unmarshal(buf.Bytes(), &jsonMessage)
			if jsonMessage.Error != nil {
				jsonMessage.Display(os.Stdout, false)
				return jsonMessage.Error
			}
			jsonMessage.Display(os.Stdout, true)
		}

		newImage, err := docker.ImageSave(ctx, []string{deploymentConfig.Name + ":latest"})
		if err != nil {
			log.Fatal("Failed to save image", err.Error())
		}
		uploadStream = newImage
		defer newImage.Close()
	}

	req, _ := createRequest("POST", "/deployments")
	req.Header.Add("Content-Type", "application/x-tar")
	println("Uploading image", string(cleanedconfig))
	req.Header.Add("x-jig-config", string(cleanedconfig))
	if localBuild {
		req.Header.Add("x-jig-image", "true")
	} else {
		req.Header.Add("x-jig-image", "false")
	}
	req.Body = uploadStream

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal("Error making request: ", err)
	}
	defer resp.Body.Close()
	print("status code", resp.StatusCode)
	if localBuild {
		if resp.StatusCode != 200 {
			log.Fatal("Error creating deployment: ", resp.Status)
		}
		println("Successfully created a deployment ✨")
	} else {
		buf := bufio.NewScanner(resp.Body)
		for buf.Scan() {
			jsonMessage := jsonmessage.JSONMessage{}
			json.Unmarshal(buf.Bytes(), &jsonMessage)
			if jsonMessage.Error != nil {
				jsonMessage.Display(os.Stdout, true)
				return jsonMessage.Error
			}
			jsonMessage.Display(os.Stdout, true)
		}
	}

	return nil
}

func main() {
	loadFileConfig()
	app := &cli.App{
		Name: "jig",
		Commands: []*cli.Command{
			{
				Name:   "login",
				Usage:  "Login to the Jig server",
				Args:   true,
				Action: loginCommand,
			},
			{
				Name: "deployments",
				Subcommands: []*cli.Command{
					{
						Name:   "ls",
						Usage:  "List deployments",
						Action: listDeployments,
					},
					{
						Name:  "deploy",
						Usage: "Deploy a container",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Verbose output",
								Value:   false,
							},
							&cli.BoolFlag{
								Name:    "local",
								Aliases: []string{"l"},
								Usage:   "Build the image locally",
								Value:   false,
							},
						},
						Action: deployComment,
					},

					{
						Name:  "rm",
						Usage: "Stop a container",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Verbose output",
								Value:   false,
							},
						},
						Args:      true,
						ArgsUsage: " name",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)
							name := ctx.Args().First()
							if name == "" {
								log.Fatal("Name is required")
							}
							req, _ := createRequest("DELETE", "/deployments/"+name)
							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 204 {
								log.Fatal("Error deleting deployment: ", resp.Status)
							} else {
								println("Successfully removed a deployment ❌")
							}
							return nil
						},
					},
					{
						Name:      "logs",
						Usage:     "Get logs for a container",
						Args:      true,
						ArgsUsage: "deployment name",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)
							name := ctx.Args().First()
							if name == "" {
								log.Fatal("Name is required")
							}
							req, _ := createRequest("GET", "/deployments/"+name+"/logs")
							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 200 {
								log.Fatal("Error getting logs: ", resp.Status)
							}

							body, err := io.ReadAll(resp.Body)
							if err != nil {
								log.Fatal("Error reading request: ", err)
							}
							println(string(body))

							return nil
						},
					},
					{
						Name:  "stats",
						Usage: "Get resource usage stats",
						Action: func(ctx *cli.Context) error {
							enrichConfigFromFlags(ctx)
							req, _ := createRequest("GET", "/deployments/stats")
							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 200 {
								log.Fatal("Error getting logs: ", resp.Status)
							}

							body, err := io.ReadAll(resp.Body)
							if err != nil {
								log.Fatal("Error reading request: ", err)
							}
							var stats []jigtypes.Stats
							if err := json.Unmarshal(body, &stats); err != nil {
								log.Fatal("Error unmarshalling response: ", err)
							}

							writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight)
							fmt.Fprintln(writer, "name\tmemory\tcpu")
							for _, stat := range stats {
								fmt.Fprintf(writer, "%s\t%.2f MB (%.2f%%)\t%.2f%% \n", stat.Name, stat.MemoryBytes, stat.MemoryPercentage, stat.CpuPercentage)
							}
							writer.Flush()
							return nil
						},
					},
				},
			},
			{
				Name: "secrets",
				Subcommands: []*cli.Command{
					{
						Name:    "ls",
						Aliases: []string{"list"},
						Usage:   "List secrets",
						Action:  ListSecrets,
					},
					{
						Name:   "inspect",
						Usage:  "Inspect a secret",
						Action: InspectSecret,
					},

					{
						Name:   "add",
						Args:   true,
						Usage:  "add a secret",
						Action: AddSecret,
					},
					{
						Name:   "rm",
						Usage:  "delete a secret",
						Action: DeleteSecret,
					},
				},
			},
		},
		Action: deployComment,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Verbose output",
				Value:   false,
			},
			&cli.BoolFlag{
				Name:    "local",
				Aliases: []string{"l"},
				Usage:   "Build the image locally",
				Value:   false,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func listDeployments(ctx *cli.Context) error {
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
	var deployments []jigtypes.Deployment
	err = json.Unmarshal(body, &deployments)

	if err != nil {
		println("Error unmarshalling response: ", string(body))
		log.Fatal("Error unmarshalling response: ", err)
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	fmt.Fprintln(writer, "name\trule\tstate\tstatus")
	for _, deployment := range deployments {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", deployment.Name, deployment.Rule, deployment.Status, deployment.Lifetime)
	}
	writer.Flush()
	return nil
}

func ListSecrets(ctx *cli.Context) error {
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
	var secretList jigtypes.SecretList
	err = json.Unmarshal(body, &secretList)

	if err != nil {
		println("Error unmarshalling response: ", string(body))
		log.Fatal("Error unmarshalling response: ", err)
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	fmt.Fprintln(writer, "secret name")
	for _, secret := range secretList.Secrets {
		fmt.Fprintf(writer, "%s\n", secret)
	}
	writer.Flush()
	return nil
}

func DeleteSecret(ctx *cli.Context) error {
	enrichConfigFromFlags(ctx)

	name := ctx.Args().Get(0)

	req, _ := createRequest("DELETE", "/secrets/"+name)

	resp, err := httpClient.Do(req)

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
}

func InspectSecret(ctx *cli.Context) error {
	enrichConfigFromFlags(ctx)

	name := ctx.Args().Get(0)

	req, _ := createRequest("GET", "/secrets/"+name)

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal("Error reading secret: ", err.Error())
	}
	if resp.StatusCode != 200 {
		log.Fatal("Error reading secret: ", resp.Status)
	}

	var secret jigtypes.SecretInspect

	bodyRes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Error reading secret: ", err.Error())
	}

	json.Unmarshal(bodyRes, &secret)

	println("Secret name:", name)
	println("Secret value:", secret.Value)
	return nil
}

func AddSecret(ctx *cli.Context) error {
	enrichConfigFromFlags(ctx)

	bodyToSend := jigtypes.NewSecretBody{
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
	if resp.StatusCode == 409 {
		println("Secret already exists:", bodyToSend.Name)
		return nil
	}
	if resp.StatusCode != 201 {
		log.Fatal("Error creating secret: ", resp.Status)
	}

	println("Successfully created a secret ✨")
	return nil
}
