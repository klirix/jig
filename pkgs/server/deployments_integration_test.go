//go:build integration

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jigtypes "askh.at/jig/v2/pkgs/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
)

func dockerIntegrationClient(t *testing.T) *client.Client {
	t.Helper()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("docker client unavailable: %v", err)
	}
	t.Cleanup(func() { cli.Close() })

	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	}

	return cli
}

func writeIntegrationFile(t *testing.T, root, rel, contents string) {
	t.Helper()

	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func tarIntegrationDir(t *testing.T, root string) io.ReadCloser {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
	if err != nil {
		t.Fatalf("tar dir: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

func cleanupDeploymentByName(t *testing.T, cli *client.Client, name string) {
	t.Helper()

	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "jig.name="+name)),
	})
	if err != nil {
		t.Fatalf("list containers for cleanup: %v", err)
	}

	for _, containerInfo := range containers {
		if err := cli.ContainerRemove(context.Background(), containerInfo.ID, container.RemoveOptions{Force: true}); err != nil {
			t.Fatalf("remove container %s: %v", containerInfo.ID, err)
		}
	}
}

func waitForContainerByLabel(t *testing.T, cli *client.Client, label string) dockertypes.Container {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		containers, err := cli.ContainerList(context.Background(), container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("label", label)),
		})
		if err != nil {
			t.Fatalf("list containers: %v", err)
		}
		if len(containers) > 0 {
			return containers[0]
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for container with label %s", label)
	return dockertypes.Container{}
}

func TestComposeDeploymentE2E(t *testing.T) {
	cli := dockerIntegrationClient(t)

	if err := ensureNetworkIsUp(cli); err != nil {
		t.Fatalf("ensure jig network: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "integration.db")
	db, err := createOrOpenDb(dbPath)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer db.Close()

	secretStore, err := InitSecrets(db)
	if err != nil {
		t.Fatalf("init secrets: %v", err)
	}
	defer secretStore.Close()

	if err := secretStore.Insert("compose-api-secret", "supersecret"); err != nil {
		t.Fatalf("insert secret: %v", err)
	}

	deployments := DeploymentsRouter{cli: cli, secret_db: secretStore}
	router := chi.NewRouter()
	router.Mount("/deployments", deployments.Router())

	projectDir := t.TempDir()
	projectName := fmt.Sprintf("compose-e2e-%d", time.Now().UnixNano())
	frontendName := projectName + "-frontend"
	apiName := projectName + "-api"

	writeIntegrationFile(t, projectDir, "docker-compose.yaml", fmt.Sprintf(`
services:
  frontend:
    image: busybox:1.36
    command: ["sh", "-c", "while true; do sleep 5; done"]
    x-jig:
      name: %s
      domain: frontend.example.test
      envs:
        APP_ROLE: frontend

  api:
    image: busybox:1.36
    command: ["sh", "-c", "while true; do sleep 5; done"]
    x-jig:
      name: %s
      domain: api.example.test
      envs:
        APP_ROLE: api
        API_SECRET: "@compose-api-secret"

  db:
    image: busybox:1.36
    command: ["sh", "-c", "while true; do sleep 5; done"]
`, frontendName, apiName))

	t.Cleanup(func() {
		cleanupDeploymentByName(t, cli, frontendName)
		cleanupDeploymentByName(t, cli, apiName)
		_, _ = runComposeCommand(projectDir, "-p", projectName, "-f", "docker-compose.yaml", "down", "--remove-orphans")
	})

	req := httptest.NewRequest(http.MethodPost, "/deployments", tarIntegrationDir(t, projectDir))
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("x-jig-image", "false")
	req.Header.Set("x-jig-config", fmt.Sprintf(`{"name":"%s","composeFile":"docker-compose.yaml"}`, projectName))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("deploy status %d: %s", res.StatusCode, string(body))
	}

	frontendContainer := waitForContainerByLabel(t, cli, "jig.name="+frontendName)
	apiContainer := waitForContainerByLabel(t, cli, "jig.name="+apiName)

	if frontendContainer.Labels["traefik.http.routers."+frontendName+".rule"] != "Host(`frontend.example.test`)" {
		t.Fatalf("unexpected frontend rule: %#v", frontendContainer.Labels)
	}
	if apiContainer.Labels["traefik.http.routers."+apiName+".rule"] != "Host(`api.example.test`)" {
		t.Fatalf("unexpected api rule: %#v", apiContainer.Labels)
	}

	apiInspect, err := cli.ContainerInspect(context.Background(), apiContainer.ID)
	if err != nil {
		t.Fatalf("inspect api container: %v", err)
	}

	envs := strings.Join(apiInspect.Config.Env, "\n")
	if !strings.Contains(envs, "APP_ROLE=api") {
		t.Fatalf("expected APP_ROLE env in %q", envs)
	}
	if !strings.Contains(envs, "API_SECRET=supersecret") {
		t.Fatalf("expected resolved secret env in %q", envs)
	}
	if _, ok := apiInspect.NetworkSettings.Networks["jig"]; !ok {
		t.Fatalf("expected api service container to join jig network: %#v", apiInspect.NetworkSettings.Networks)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/deployments", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("list status %d", listW.Code)
	}

	var listed []jigtypes.Deployment
	if err := json.Unmarshal(listW.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode deployments: %v", err)
	}

	names := map[string]bool{}
	for _, deployment := range listed {
		names[deployment.Name] = true
	}
	if !names[frontendName] || !names[apiName] {
		t.Fatalf("expected frontend and api deployments in %#v", listed)
	}
}
