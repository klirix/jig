package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/urfave/cli/v2"
)

func loadDeploymentConfig(configFilename string) (jigtypes.DeploymentConfig, error) {
	configContents, err := os.ReadFile(configFilename)
	if os.IsNotExist(err) {
		return jigtypes.DeploymentConfig{}, fmt.Errorf("no %s file found in the current directory", configFilename)
	}
	if err != nil {
		return jigtypes.DeploymentConfig{}, fmt.Errorf("read %s: %w", configFilename, err)
	}

	var deploymentConfig jigtypes.DeploymentConfig
	if err := json.Unmarshal(configContents, &deploymentConfig); err != nil {
		return jigtypes.DeploymentConfig{}, fmt.Errorf("parse %s: %w", configFilename, err)
	}
	if deploymentConfig.Name == "" {
		return jigtypes.DeploymentConfig{}, fmt.Errorf("name is required in %s", configFilename)
	}

	return deploymentConfig, nil
}

func newTarStream(filesToPack []string) io.ReadCloser {
	reader, writer := io.Pipe()

	go func() {
		tw := tar.NewWriter(writer)
		var err error

		for _, filename := range filesToPack {
			if err = writeFileToTar(filename, tw); err != nil {
				break
			}
		}
		if closeErr := tw.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		_ = writer.Close()
	}()

	return reader
}

func displayDockerOutput(stream io.Reader) error {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if !json.Valid(line) {
			fmt.Fprintln(os.Stdout, string(line))
			continue
		}

		var jsonMessage jsonmessage.JSONMessage
		if err := json.Unmarshal(line, &jsonMessage); err != nil {
			return fmt.Errorf("decode docker output: %w", err)
		}
		if err := jsonMessage.Display(os.Stdout, true); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read docker output: %w", err)
	}

	return nil
}

func buildAndSaveLocalImage(ctx context.Context, dockerClient *client.Client, imageName string, buildContext io.ReadCloser) (io.ReadCloser, error) {
	defer buildContext.Close()

	buildResponse, err := dockerClient.ImageBuild(ctx, buildContext, types.ImageBuildOptions{
		Tags:   []string{imageName + ":latest"},
		Remove: true,
	})
	if err != nil {
		return nil, fmt.Errorf("request image build: %w", err)
	}
	defer buildResponse.Body.Close()

	if err := displayDockerOutput(buildResponse.Body); err != nil {
		return nil, fmt.Errorf("build image: %w", err)
	}

	newImage, err := dockerClient.ImageSave(ctx, []string{imageName + ":latest"})
	if err != nil {
		return nil, fmt.Errorf("save image: %w", err)
	}

	return newImage, nil
}

func resolveComposeFile(root, configured string) (string, bool, error) {
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	} else {
		candidates = append(candidates, "docker-compose.yaml", "docker-compose.yml", "compose.yaml", "compose.yml")
	}

	for _, candidate := range candidates {
		composePath := filepath.Join(root, candidate)
		info, err := os.Stat(composePath)
		if os.IsNotExist(err) {
			if configured != "" {
				return "", false, fmt.Errorf("compose file %s not found", candidate)
			}
			continue
		}
		if err != nil {
			return "", false, fmt.Errorf("stat compose file %s: %w", candidate, err)
		}
		if info.IsDir() {
			return "", false, fmt.Errorf("compose file %s is a directory", candidate)
		}
		return candidate, true, nil
	}

	return "", false, nil
}

func deployCommand(c *cli.Context) error {
	if token := c.String("token"); token != "" {
		if err := config.UseTempToken(token); err != nil {
			return fmt.Errorf("use token: %w", err)
		}
	}

	configFilename := c.String("config")
	if configFilename == "" {
		configFilename = DEFAULT_CONFIG
	}

	deploymentConfig, err := loadDeploymentConfig(configFilename)
	if err != nil {
		return err
	}

	composeFile, hasComposeFile, err := resolveComposeFile(".", deploymentConfig.ComposeFile)
	if err != nil {
		return err
	}
	if hasComposeFile {
		deploymentConfig.ComposeFile = composeFile
	}

	ignorePatterns, err := loadIgnorePatterns(".jigignore")
	if err != nil {
		return err
	}

	filesToPack, err := collectFilesToPack(".", ignorePatterns)
	if err != nil {
		return err
	}
	localBuild := c.Bool("local")

	if c.Bool("verbose") {
		ui.section("Package Preview", fmt.Sprintf("%d files", len(filesToPack)))
		for _, file := range filesToPack {
			fmt.Fprintf(os.Stdout, "  %s\n", file)
		}
	}
	ui.section("Deploy", deploymentConfig.Name)
	ui.line("files", fmt.Sprintf("%d", len(filesToPack)))
	ui.line("ignored", strings.Join(ignorePatterns, ", "))
	if hasComposeFile {
		ui.line("compose", deploymentConfig.ComposeFile)
	}
	if localBuild {
		ui.line("build", "local image")
	} else {
		ui.line("build", "server-side")
	}

	uploadStream := newTarStream(filesToPack)
	defer uploadStream.Close()

	if hasComposeFile && localBuild {
		return fmt.Errorf("local image deployments are not supported with compose files")
	}
	if localBuild {
		ui.line("phase", "building local image")
		dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return fmt.Errorf("create docker client: %w", err)
		}
		defer dockerClient.Close()

		imageStream, err := buildAndSaveLocalImage(c.Context, dockerClient, deploymentConfig.Name, uploadStream)
		if err != nil {
			return err
		}

		uploadStream = imageStream
		defer uploadStream.Close()
	}

	compactConfigBytes, err := json.Marshal(deploymentConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	req, err := createRequest("POST", "/deployments")
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("x-jig-config", string(compactConfigBytes))
	req.Header.Set("x-jig-image", fmt.Sprint(localBuild))
	req.Header.Set("x-jig-verbose", fmt.Sprint(c.Bool("verbose")))
	req.Body = &TrackableReader{ReadCloser: uploadStream}

	loading := ui.startLoading("Uploading deployment")
	resp, err := httpClient.Do(req)
	loading.stop()
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("create deployment: %s", resp.Status)
		}

		bodyText := bytes.TrimSpace(body)
		if len(bodyText) == 0 {
			return fmt.Errorf("create deployment: %s", resp.Status)
		}

		return fmt.Errorf("create deployment: %s: %s", resp.Status, bodyText)
	}

	if localBuild {
		ui.success("Created deployment " + deploymentConfig.Name)
		return nil
	}

	ui.line("phase", "streaming remote build output")
	return displayDockerOutput(resp.Body)
}
