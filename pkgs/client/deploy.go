package main

import (
	"archive/tar"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/urfave/cli/v2"
)

func deployComment(c *cli.Context) error {
	if c.String("token") != "" {
		config.UseTempToken(c.String("token"))
	}
	configFilename := DEFAULT_CONFIG // Default config file
	if c.String("config") != "" {
		configFilename = c.String("config")
	}
	_, err := os.Stat(configFilename)
	if err != nil {
		log.Fatalf("No %s file found in the current directory", configFilename)
	}
	configContents, err := os.ReadFile(configFilename)
	if err != nil {
		log.Fatalf("Failed to read %s: %s", configFilename, err)
	}
	var deploymentConfig jigtypes.DeploymentConfig
	err = json.Unmarshal(configContents, &deploymentConfig)
	if err != nil {
		log.Fatalf("Failed to parse %s: %s", configFilename, err)
	}
	if deploymentConfig.Name == "" {
		log.Fatalf("Name is required in %s", configFilename)
	}
	cleanedconfig := strings.ReplaceAll(string(configContents), "\n", "")

	println("Deploying container")

	ctx := c.Context
	// docker.ImageBuild(ctx)
	verbose := c.Bool("verbose")

	ignorePatterns := loadIgnorePatterns(".jigignore")

	var matches = []string{}
	err = filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
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
	fmt.Printf("Packing files, ignoring: %v\n", ignorePatterns)

	reader, writer := io.Pipe()

	var uploadStream io.ReadCloser = reader

	go func() {
		tw := tar.NewWriter(writer)

		for _, filename := range filesToPack {
			writeFileToTar(filename, tw)
		}
		if err := tw.Close(); err != nil {
			log.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			log.Fatal(err)
		}

	}()

	localBuild := c.Bool("local")

	if localBuild {
		docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Fatal("Failed to create docker client", err)
		}
		defer docker.Close()
		println("Client started")

		buildResponse, err := docker.ImageBuild(ctx, reader, types.ImageBuildOptions{
			Tags:   []string{deploymentConfig.Name + ":latest"},
			Remove: true,
		})
		if err != nil {
			log.Fatal("Failed to request image build", err.Error())
		}
		defer buildResponse.Body.Close()
		println("Client built image")

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

		println("Image built")

		newImage, err := docker.ImageSave(ctx, []string{deploymentConfig.Name + ":latest"})
		if err != nil {
			log.Fatal("Failed to save image", err.Error())
		}
		println("Image saved")
		uploadStream = newImage
		defer newImage.Close()
	} else {

	}

	req, _ := createRequest("POST", "/deployments")
	req.Header.Add("Content-Type", "application/x-tar")

	req.Header.Add("x-jig-config", string(cleanedconfig))
	if localBuild {
		req.Header.Add("x-jig-image", "true")
	} else {
		req.Header.Add("x-jig-image", "false")
	}
	req.Body = &TrackableReader{ReadCloser: uploadStream}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatal("Error making request: ", err)
	}
	defer resp.Body.Close()

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
