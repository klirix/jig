package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"embed"

	"askh.at/jig/v2/pkgs/client/client_config"
	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/urfave/cli/v2"
)

var config, _ = client_config.InitConfig()
var ui = newCLIOutput()

var httpClient = &http.Client{}

func createRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, config.Endpoint+url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+config.Token)
	return req, nil
}

var defaultIgnorePatterns = []string{".git/**", ".gitignore", ".jig/**", "node_modules/**"}

func loadIgnorePatterns(ignoreFile string) ([]string, error) {
	patterns := append([]string{}, defaultIgnorePatterns...)

	ignoreFileContents, err := os.ReadFile(ignoreFile)
	if os.IsNotExist(err) {
		return patterns, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ignoreFile, err)
	}

	for _, line := range strings.Split(string(ignoreFileContents), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}

	return patterns, nil
}

func shouldIgnorePath(path string, isDir bool, ignorePatterns []string) (bool, error) {
	candidates := []string{path}
	if isDir {
		candidates = append(candidates, filepath.Join(path, "__jig_ignore_probe__"))
	}

	ignored := false
	for _, rawPattern := range ignorePatterns {
		pattern := rawPattern
		isNegated := strings.HasPrefix(pattern, "!")
		if isNegated {
			pattern = strings.TrimPrefix(pattern, "!")
		}

		for _, candidate := range candidates {
			matched, err := doublestar.Match(pattern, candidate)
			if err != nil {
				return false, fmt.Errorf("match pattern %q against %q: %w", rawPattern, candidate, err)
			}
			if matched {
				ignored = !isNegated
				break
			}
		}
	}

	return ignored, nil
}

func canSkipIgnoredDir(path string, ignorePatterns []string) bool {
	dirPrefix := path + string(os.PathSeparator)
	for _, pattern := range ignorePatterns {
		if !strings.HasPrefix(pattern, "!") {
			continue
		}
		if strings.HasPrefix(strings.TrimPrefix(pattern, "!"), dirPrefix) {
			return false
		}
	}
	return true
}

func collectFilesToPack(root string, ignorePatterns []string) ([]string, error) {
	filesToPack := []string{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		ignored, err := shouldIgnorePath(path, d.IsDir(), ignorePatterns)
		if err != nil {
			return err
		}
		if ignored {
			if d.IsDir() && canSkipIgnoredDir(path, ignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		filesToPack = append(filesToPack, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	return filesToPack, nil
}

func writeFileToTar(filename string, tw *tar.Writer) error {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filename, err)
	}

	hdr, err := tar.FileInfoHeader(fileInfo, "")
	if err != nil {
		return fmt.Errorf("create tar header for %s: %w", filename, err)
	}
	hdr.Name = filename

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header for %s: %w", filename, err)
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open %s: %w", filename, err)
	}
	defer file.Close()

	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("copy %s into tarball: %w", filename, err)
	}

	return nil
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

func loginCommand(c *cli.Context) error {
	token := c.Args().Get(0)
	if token != "" {
		err := config.UseTempToken(token)
		if err != nil {
			log.Fatal("Error using token: ", err)
		}
	}
	req, _ := createRequest("GET", "/deployments")
	loading := ui.startLoading("Connecting to server")
	resp, err := httpClient.Do(req)
	loading.stop()
	if err != nil {
		log.Fatal("Error making request: ", err)
	}
	if resp.StatusCode != 200 {
		log.Fatal("Error connecting to jig ", resp.Status)
	}
	config.AddServer(config.Endpoint, config.Token)
	config.Persist()
	ui.success("Logged in")
	return nil
}

const DEFAULT_CONFIG = "./jig.json"

//go:embed templates/*
var templates embed.FS

func main() {
	config.ReadFromFile()
	app := &cli.App{
		Name: "jig",
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initiate jig in the directory",
				Args:  false,
				Action: func(ctx *cli.Context) error {
					if _, err := os.Stat(DEFAULT_CONFIG); err == nil {
						log.Fatal("jig.json already exists")
					}
					currentDir, err := os.Getwd()
					dirName := filepath.Base(currentDir)
					if err != nil {
						log.Fatal("Error getting current directory: ", err)
					}
					configTemplate, _ := templates.ReadFile("templates/jig.json")
					err = os.WriteFile(DEFAULT_CONFIG, []byte(strings.Replace(string(configTemplate), "dir-name", dirName, -1)), 0644)

					if err != nil {
						log.Fatal("Error writing jig.json: ", err)
					}
					os.WriteFile("./.jigignore", []byte(`# This is a list of files to ignore when deploying`), 0644)
					ui.success("Created jig.json")
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
				Action: deployCommand,
			},
			{
				Name:      "logs",
				Usage:     "Get logs for a deployment or stack:service",
				Args:      true,
				ArgsUsage: " name|stack:service",
				Flags:     []cli.Flag{tokenFlag},
				Action:    logsCommand,
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
						Action: deployCommand,
					},

					{
						Name:  "rm",
						Usage: "Delete a deployment or stack:service",
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
						ArgsUsage: " name|stack:service",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							name := ctx.Args().First()
							if name == "" {
								log.Fatal("Name is required")
							}
							req, _ := createRequest("DELETE", "/deployments/"+name)
							loading := ui.startLoading("Removing deployment")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 204 {
								log.Fatal("Error deleting deployment: ", resp.Status)
							} else {
								ui.success("Removed deployment " + name)
							}
							return nil
						},
					},
					{
						Name:  "rollback",
						Usage: "Rollback a single-container deployment",
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
						ArgsUsage: " name|stack:service",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							name := ctx.Args().First()
							if name == "" {
								log.Fatal("Name is required")
							}
							req, _ := createRequest("POST", "/deployments/"+name+"/rollback")
							loading := ui.startLoading("Rolling back deployment")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 200 {
								log.Fatal("Error rolling back a deployment: ", resp.Status)
							} else {
								ui.success("Rolled back deployment " + name)
							}
							return nil
						},
					},
					{
						Name:  "logs",
						Usage: "Get logs for a deployment or stack:service",
						Args:  true,
						Flags: []cli.Flag{
							tokenFlag,
						},
						ArgsUsage: " name|stack:service",
						Action:    logsCommand,
					},
					{
						Name:  "scale",
						Usage: "Scale a singular swarm deployment",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Args:      true,
						ArgsUsage: " name replicas",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							name := ctx.Args().Get(0)
							replicasString := ctx.Args().Get(1)
							if name == "" || replicasString == "" {
								log.Fatal("Name and replicas are required")
							}
							replicas, err := strconv.Atoi(replicasString)
							if err != nil {
								log.Fatal("Replicas must be a number")
							}
							requestBody, err := json.Marshal(jigtypes.DeploymentScaleRequest{Replicas: replicas})
							if err != nil {
								log.Fatal("Error marshaling scale request: ", err)
							}
							req, _ := createRequest("POST", "/deployments/"+name+"/scale")
							req.Header.Set("Content-Type", "application/json")
							req.Body = io.NopCloser(bytes.NewReader(requestBody))
							loading := ui.startLoading("Scaling deployment")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusNoContent {
								body, _ := io.ReadAll(resp.Body)
								log.Fatalf("Error scaling deployment: %s: %s", resp.Status, strings.TrimSpace(string(body)))
							}
							ui.success(fmt.Sprintf("Scaled %s to %d replicas", name, replicas))
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
								config.UseTempToken(ctx.String("token"))
							}
							req, _ := createRequest("GET", "/deployments/stats")
							loading := ui.startLoading("Loading deployment stats")
							resp, err := httpClient.Do(req)
							loading.stop()
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
							var statsResponse jigtypes.DeploymentStatsResponse
							if err := json.Unmarshal(body, &statsResponse); err != nil {
								log.Fatal("Error unmarshalling response: ", err)
							}

							ui.section("Deployment Stats", "Live resource usage")
							if statsResponse.Mode == "swarm" {
								ui.table([]string{"node", "role", "availability", "state", "cpus", "memory", "running", "total"}, func(writer *tabwriter.Writer) {
									for _, node := range statsResponse.Nodes {
										fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%d\t%.2f GB\t%d\t%d\n",
											node.Name,
											node.Role,
											node.Availability,
											node.State,
											node.Cpus,
											float64(node.MemoryBytes)/(1024*1024*1024),
											node.RunningTasks,
											node.TotalTasks,
										)
									}
								})
							} else {
								ui.table([]string{"name", "memory", "cpu"}, func(writer *tabwriter.Writer) {
									for _, stat := range statsResponse.Stats {
										fmt.Fprintf(writer, "%s\t%.2f MB (%.2f%%)\t%.2f%%\n", stat.Name, stat.MemoryBytes, stat.MemoryPercentage, stat.CpuPercentage)
									}
								})
							}
							return nil
						},
					},
				},
			},
			{
				Name: "tokens",
				Subcommands: []*cli.Command{
					{
						Name:  "ls",
						Usage: "List tokens",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}

							req, _ := createRequest("GET", "/tokens")
							loading := ui.startLoading("Loading tokens")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							body, err := io.ReadAll(resp.Body)
							if err != nil {
								log.Fatal("Error reading request: ", err)
							}
							var tokens jigtypes.TokenListResponse
							err = json.Unmarshal(body, &tokens)

							if err != nil {
								println("Error unmarshalling response: ", string(body))
								log.Fatal("Error unmarshalling response: ", err)
							}

							ui.section("Tokens", fmt.Sprintf("%d configured", len(tokens.TokenNames)))
							if len(tokens.TokenNames) == 0 {
								ui.warning("No tokens configured")
								return nil
							}
							ui.table([]string{"name"}, func(writer *tabwriter.Writer) {
								for _, token := range tokens.TokenNames {
									fmt.Fprintf(writer, "%s\n", token)
								}
							})
							return nil
						},
					},
					{
						Name:  "create",
						Usage: "Create a token",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Args: true,
						Action: func(ctx *cli.Context) error {
							name := ctx.Args().First()
							req, _ := createRequest("POST", "/tokens")
							bodyToSend := jigtypes.TokenCreateRequest{
								Name: name,
							}
							bodyBytes, _ := json.Marshal(bodyToSend)

							req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
							loading := ui.startLoading("Creating token")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							if resp.StatusCode != 201 {
								log.Fatal("Error creating token: ", resp.Status)
							}
							var tokenCreateResponse jigtypes.TokenCreateResponse
							err = json.NewDecoder(resp.Body).Decode(&tokenCreateResponse)
							if err != nil {
								log.Fatal("Error reading response: ", err)
							}

							ui.section("Token Created", "Store this value now; it will not be shown again")
							ui.line("name", tokenCreateResponse.Name)
							ui.line("token", config.SelectedServer+"+"+tokenCreateResponse.Token)
							return nil
						},
					},
					{
						Name:  "delete",
						Usage: "Delete a token",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Args: true,
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							name := ctx.Args().First()
							if name == "" {
								log.Fatal("Name is required")
							}
							req, _ := createRequest("DELETE", "/tokens/"+name)
							loading := ui.startLoading("Removing token")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}

							if resp.StatusCode != 204 {
								log.Fatal("Error deleting token: ", resp.Status)
							} else {
								ui.success("Removed token " + name)
							}
							return nil
						},
					},
				},
			},
			{
				Name: "servers",
				Subcommands: []*cli.Command{
					{
						Name:  "ls",
						Usage: "List servers",
						Action: func(ctx *cli.Context) error {
							servers := sortedServers(config.Servers)
							ui.section("Servers", fmt.Sprintf("%d configured", len(servers)))
							if len(servers) == 0 {
								ui.warning("No servers configured")
								return nil
							}
							ui.table([]string{"server", "selected"}, func(writer *tabwriter.Writer) {
								for _, server := range servers {
									selected := ""
									if config.SelectedServer == server {
										selected = ui.cyan("current")
									}
									fmt.Fprintf(writer, "%s\t%s\n", server, selected)
								}
							})
							return nil
						},
					},
					{
						Name:  "select",
						Usage: "Select a server",
						Args:  true,
						Action: func(ctx *cli.Context) error {
							server := ctx.Args().First()
							err := config.SelectServer(server)
							if err != nil {
								log.Fatal("Error selecting server: ", err)
							}
							ui.success("Selected server " + server)
							return nil
						},
					},
					{
						Name:  "rm",
						Usage: "Remove a server",
						Args:  true,
						Action: func(ctx *cli.Context) error {
							server := ctx.Args().First()
							if server == "" {
								log.Fatal("Server name is required")
							}
							delete(config.Servers, server)
							if config.SelectedServer == server {
								config.SelectedServer = ""
								config.Token = ""
							}
							config.Persist()
							ui.success("Removed server " + server)
							return nil
						},
					},
				},
			},
			{
				Name: "cluster",
				Subcommands: []*cli.Command{
					{
						Name:  "status",
						Usage: "Show cluster status",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							req, _ := createRequest("GET", "/cluster/status")
							loading := ui.startLoading("Loading cluster status")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusOK {
								body, _ := io.ReadAll(resp.Body)
								log.Fatalf("Error getting cluster status: %s: %s", resp.Status, strings.TrimSpace(string(body)))
							}

							var status jigtypes.ClusterStatusResponse
							if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
								log.Fatal("Error decoding response: ", err)
							}

							ui.section("Cluster Status", "")
							ui.line("backend", status.Backend)
							ui.line("manager", status.ManagerAddress)
							if len(status.Nodes) == 0 {
								return nil
							}

							ui.table([]string{"node", "role", "availability", "state", "cpus", "memory", "running", "total"}, func(writer *tabwriter.Writer) {
								for _, node := range status.Nodes {
									fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%d\t%.2f GB\t%d\t%d\n",
										node.Name,
										node.Role,
										node.Availability,
										node.State,
										node.Cpus,
										float64(node.MemoryBytes)/(1024*1024*1024),
										node.RunningTasks,
										node.TotalTasks,
									)
								}
							})
							return nil
						},
					},
					{
						Name:  "join-token",
						Usage: "Get a swarm join token and command",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Args:      true,
						ArgsUsage: " worker|manager",
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							role := ctx.Args().First()
							if role == "" {
								log.Fatal("Role is required")
							}
							req, _ := createRequest("GET", "/cluster/join-token/"+role)
							loading := ui.startLoading("Loading join token")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusOK {
								body, _ := io.ReadAll(resp.Body)
								log.Fatalf("Error getting join token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
							}

							var joinToken jigtypes.ClusterJoinTokenResponse
							if err := json.NewDecoder(resp.Body).Decode(&joinToken); err != nil {
								log.Fatal("Error decoding response: ", err)
							}

							ui.section("Join Command", role)
							fmt.Println(joinToken.Command)
							return nil
						},
					},
					{
						Name:  "join-worker",
						Usage: "Print a worker bootstrap command",
						Flags: []cli.Flag{
							tokenFlag,
						},
						Action: func(ctx *cli.Context) error {
							if ctx.String("token") != "" {
								config.UseTempToken(ctx.String("token"))
							}
							req, _ := createRequest("GET", "/cluster/join-token/worker")
							loading := ui.startLoading("Loading worker bootstrap")
							resp, err := httpClient.Do(req)
							loading.stop()
							if err != nil {
								log.Fatal("Error making request: ", err)
							}
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusOK {
								body, _ := io.ReadAll(resp.Body)
								log.Fatalf("Error getting worker bootstrap command: %s: %s", resp.Status, strings.TrimSpace(string(body)))
							}

							var joinToken jigtypes.ClusterJoinTokenResponse
							if err := json.NewDecoder(resp.Body).Decode(&joinToken); err != nil {
								log.Fatal("Error decoding response: ", err)
							}

							ui.section("Worker Bootstrap", "Run this on the node you want to join")
							fmt.Printf("curl -fsSL https://deploywithjig.askh.at/worker.sh | JIG_SWARM_JOIN_TOKEN=%q JIG_SWARM_MANAGER_ADDR=%q bash\n", joinToken.Token, joinToken.ManagerAddress)
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
						Flags:   []cli.Flag{tokenFlag},
						Action:  ListSecrets,
					},
					{
						Name:   "inspect",
						Usage:  "Inspect a secret",
						Flags:  []cli.Flag{tokenFlag},
						Action: InspectSecret,
					},

					{
						Name:      "add",
						Args:      true,
						ArgsUsage: "name value",
						Usage:     "add a secret",
						Flags:     []cli.Flag{tokenFlag},
						Action:    AddSecret,
					},
					{
						Name:   "rm",
						Usage:  "delete a secret",
						Flags:  []cli.Flag{tokenFlag},
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
		config.UseTempToken(ctx.String("token"))
	}
	req, _ := createRequest("GET", "/deployments")
	loading := ui.startLoading("Loading deployments")
	resp, err := httpClient.Do(req)
	loading.stop()
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

	ui.section("Deployments", fmt.Sprintf("%d total", len(deployments)))
	if len(deployments) == 0 {
		ui.warning("No deployments found")
		return nil
	}
	ui.table([]string{"name", "kind", "replicas", "route", "state", "status", "rollback"}, func(writer *tabwriter.Writer) {
		for index, deployment := range deployments {
			printDeploymentRow(writer, deployment, "", true, index == len(deployments)-1)
		}
	})
	return nil
}

func logsCommand(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config.UseTempToken(ctx.String("token"))
	}
	name := ctx.Args().First()
	if name == "" {
		log.Fatal("Name is required")
	}
	req, _ := createRequest("GET", "/deployments/"+name+"/logs")
	loading := ui.startLoading("Loading logs")
	resp, err := httpClient.Do(req)
	loading.stop()
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
	ui.section("Logs", name)
	fmt.Print(string(body))

	return nil
}

func printDeploymentRow(writer *tabwriter.Writer, deployment jigtypes.Deployment, prefix string, isRoot bool, isLast bool) {
	replicas := ""
	if deployment.Replicas > 0 {
		replicas = fmt.Sprint(deployment.Replicas)
	}
	name := formatDeploymentName(prefix, deployment.Name, isRoot, isLast)
	if len(deployment.Children) > 0 {
		fmt.Fprintf(writer, "%s\t%s\t%s\t\t\t%s\t%s\n", name, deployment.Kind, replicas, deployment.Status, yesOrNo(deployment.HasRollback))
		childPrefix := childDeploymentPrefix(prefix, isRoot, isLast)
		for index, child := range deployment.Children {
			printDeploymentRow(writer, child, childPrefix, false, index == len(deployment.Children)-1)
		}
		return
	}
	fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", name, deployment.Kind, replicas, deployment.Rule, deployment.Lifetime, deployment.Status, yesOrNo(deployment.HasRollback))
}

func yesOrNo(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func ListSecrets(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config.UseTempToken(ctx.String("token"))
	}
	req, _ := createRequest("GET", "/secrets")
	loading := ui.startLoading("Loading secrets")
	resp, err := httpClient.Do(req)
	loading.stop()
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

	ui.section("Secrets", fmt.Sprintf("%d configured", len(secretList.Secrets)))
	if len(secretList.Secrets) == 0 {
		ui.warning("No secrets configured")
		return nil
	}
	ui.table([]string{"name"}, func(writer *tabwriter.Writer) {
		for _, secret := range secretList.Secrets {
			fmt.Fprintf(writer, "%s\n", secret)
		}
	})
	return nil
}

func DeleteSecret(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config.UseTempToken(ctx.String("token"))
	}

	name := ctx.Args().Get(0)

	if name == "" {
		log.Fatal("Secret name is required")
	}

	req, _ := createRequest("DELETE", "/secrets/"+name)

	loading := ui.startLoading("Removing secret")
	resp, err := httpClient.Do(req)
	loading.stop()

	if err != nil {
		log.Fatal("Error making request: ", err)
		log.Fatal("Error deleting secret: ", resp.Status)
		println(io.ReadAll(resp.Body))
	}
	if resp.StatusCode != 204 {
		log.Fatal("Error deleting secret: ", resp.Status)
		println(io.ReadAll(resp.Body))
	}

	ui.success("Removed secret " + name)
	return nil
}

func InspectSecret(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config.UseTempToken(ctx.String("token"))
	}

	name := ctx.Args().Get(0)

	if name == "" {
		log.Fatal("Secret name is required")
	}

	req, _ := createRequest("GET", "/secrets/"+name)

	loading := ui.startLoading("Loading secret")
	resp, err := httpClient.Do(req)
	loading.stop()
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

	ui.section("Secret", name)
	ui.line("name", name)
	ui.line("value", secret.Value)
	return nil
}

func AddSecret(ctx *cli.Context) error {
	if ctx.String("token") != "" {
		config.UseTempToken(ctx.String("token"))
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
	loading := ui.startLoading("Creating secret")
	resp, err := httpClient.Do(req)
	loading.stop()
	if err != nil {
		log.Fatal("Error making request: ", err)
	}
	if resp.StatusCode == 409 {
		ui.warning("Secret already exists: " + bodyToSend.Name)
		return nil
	}
	if resp.StatusCode != 201 {
		log.Fatal("Error creating secret: ", resp.Status)
	}

	ui.success("Created secret " + bodyToSend.Name)
	return nil
}
