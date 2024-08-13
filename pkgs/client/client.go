package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"embed"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/urfave/cli/v2"
)

type Config struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	Servers  ServersConfig
}

func initConfig() (Config, error) {

}

type ServerConfig struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type ConfigfileJson struct {
	Servers        []ServerConfig `json:"servers"`
	LastUsedServer string
}

type ServersConfig = ConfigfileJson

var config, _ = initConfig()

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
	file, err := os.Open(ignoreFile)
	if err != nil {
		log.Fatal(err)
	}
	ignoreFileContents, err := io.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	ignorePatterns := strings.Split(string(ignoreFileContents), "\n")
	filteredIgnorePatterns := []string{}
	for _, v := range ignorePatterns {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" || strings.HasPrefix(trimmed, "#") {
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
			matched, err := doublestar.Match(ignorePattern, filename)
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

type TrackableReader struct {
	BytesRead int64
	io.ReadCloser
}

func (t *TrackableReader) Read(p []byte) (n int, err error) {
	n, err = t.ReadCloser.Read(p)
	t.BytesRead += int64(n)
	return
}

func updateConfigUsingToken(token string) Config {
	strings.Split(token, "+")
	newConfig := Config{
		Endpoint: strings.Split(token, "+")[0],
		Token:    strings.Split(token, "+")[1],
	}
	return newConfig
}

func loginCommand(c *cli.Context) error {
	token := c.Args().Get(0)
	config = updateConfigUsingToken(token)
	configJson, err := json.Marshal(config)
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

const DEFAULT_CONFIG = "./jig.json"

//go:embed templates/*
var templates embed.FS

func main() {
	loadFileConfig()
	app := &cli.App{
		Name: "jig",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initiate jig in the directory",
				Args:  false,
				Action: func(ctx *cli.Context) error {
					if _, err := os.Stat("./jig.json"); err == nil {
						log.Fatal("jig.json already exists")
					}
					currentDir, err := os.Getwd()
					dirName := filepath.Base(currentDir)
					if err != nil {
						log.Fatal("Error getting current directory: ", err)
					}
					configTemplate, _ := templates.ReadFile("jig.json")
					err = os.WriteFile("./jig.json", []byte(strings.Replace(string(configTemplate), "dir-name", dirName, -1)), 0644)
					if err != nil {
						log.Fatal("Error writing jig.json: ", err)
					}
					os.WriteFile("./.jigignore", []byte(`# This is a list of files to ignore when deploying`), 0644)
					println("Successfully created a jig.jfileson  ✨")
					return nil
				},
			},
			{
				Name:   "login",
				Usage:  "Login to the Jig server",
				Args:   true,
				Action: loginCommand,
			},
			{
				Name:  "ls",
				Usage: "List deployments",
				Flags: []cli.Flag{
					tokenFlag,
				},
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
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Config file",
					},
					tokenFlag,
				},
				Action: deployComment,
			},
			{
				Name: "deployments",
				Subcommands: []*cli.Command{
					{
						Name:  "ls",
						Usage: "List deployments",
						Flags: []cli.Flag{
							tokenFlag,
						},
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
							&cli.StringFlag{
								Name:    "config",
								Aliases: []string{"c"},
								Usage:   "Config file",
							},
							tokenFlag,
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
							tokenFlag,
						},
						Args:      true,
						ArgsUsage: " name",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config = updateConfigUsingToken(ctx.String("token"))
							}
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
						Name:  "rollback",
						Usage: "Rollback a deployment",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Verbose output",
								Value:   false,
							},
							tokenFlag,
						},
						Args:      true,
						ArgsUsage: " name",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config = updateConfigUsingToken(ctx.String("token"))
							}
							name := ctx.Args().First()
							if name == "" {
								log.Fatal("Name is required")
							}
							req, _ := createRequest("POST", "/deployments/"+name+"/rollback")
							resp, err := httpClient.Do(req)
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 200 {
								log.Fatal("Error rolling back a deployment: ", resp.Status)
							} else {
								println("Successfully rollbacked a deployment 🔃")
							}
							return nil
						},
					},
					{
						Name:  "logs",
						Usage: "Get logs for a container",
						Args:  true,
						Flags: []cli.Flag{
							tokenFlag,
						},
						ArgsUsage: "deployment name",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config = updateConfigUsingToken(ctx.String("token"))
							}
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
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config = updateConfigUsingToken(ctx.String("token"))
							}
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
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: ListSecrets,
					},
					{
						Name:  "inspect",
						Usage: "Inspect a secret",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: InspectSecret,
					},

					{
						Name:      "add",
						Args:      true,
						ArgsUsage: "name value",
						Usage:     "add a secret",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: AddSecret,
					},
					{
						Name:  "rm",
						Usage: "delete a secret",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: DeleteSecret,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

var (
	tokenFlag = &cli.StringFlag{
		Name:    "token",
		Aliases: []string{"t"},
		Usage:   "Token to use for authentication",
	}
)

func listDeployments(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config = updateConfigUsingToken(ctx.String("token"))
	}
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
	println("> Current deployments:\n")
	fmt.Fprintln(writer, "  name\trule\tstate\tstatus")
	for _, deployment := range deployments {
		fmt.Fprintf(writer, "  %s\t%s\t%s\t%s\n", deployment.Name, deployment.Rule, deployment.Status, deployment.Lifetime)
	}
	writer.Flush()
	return nil
}

func ListSecrets(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config = updateConfigUsingToken(ctx.String("token"))
	}
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
	fmt.Fprintln(os.Stdout, []any{"> Secrets:\n"}...)
	if len(secretList.Secrets) > 0 {
		fmt.Fprintln(writer, "  name")
	} else {
		fmt.Fprintln(writer, "  No secrets set yet 🤫")
	}
	for _, secret := range secretList.Secrets {
		fmt.Fprintf(writer, "  %s\n", secret)
	}
	writer.Flush()
	return nil
}

func DeleteSecret(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config = updateConfigUsingToken(ctx.String("token"))
	}

	name := ctx.Args().Get(0)

	if name == "" {
		log.Fatal("Secret name is required")
	}

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
	if ctx.String("token") != "" {
		config = updateConfigUsingToken(ctx.String("token"))
	}

	name := ctx.Args().Get(0)

	if name == "" {
		log.Fatal("Secret name is required")
	}

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
	if ctx.String("token") != "" {
		config = updateConfigUsingToken(ctx.String("token"))
	}

	bodyToSend := jigtypes.NewSecretBody{
		Name:  ctx.Args().Get(0),
		Value: ctx.Args().Get(1),
	}

	if bodyToSend.Name == "" {
		log.Fatal("Secret name is required")
	}

	if bodyToSend.Value == "" {
		log.Fatal("Secret value is required")
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
