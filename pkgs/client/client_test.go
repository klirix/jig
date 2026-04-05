package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/tabwriter"

	jigtypes "askh.at/jig/v2/pkgs/types"
)

func TestLoadIgnorePatternsKeepsDefaultsAndSkipsComments(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldWD)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	if err := os.WriteFile(".jigignore", []byte("# comment\nbuild/**\n!.env.example\n\n"), 0644); err != nil {
		t.Fatalf("write .jigignore: %v", err)
	}

	patterns, err := loadIgnorePatterns(".jigignore")
	if err != nil {
		t.Fatalf("loadIgnorePatterns: %v", err)
	}

	for _, pattern := range defaultIgnorePatterns {
		if !slices.Contains(patterns, pattern) {
			t.Fatalf("missing default pattern %q in %v", pattern, patterns)
		}
	}
	if slices.Contains(patterns, "# comment") {
		t.Fatalf("comment line should not be kept: %v", patterns)
	}
	if !slices.Contains(patterns, "build/**") {
		t.Fatalf("expected custom ignore pattern in %v", patterns)
	}
	if !slices.Contains(patterns, "!.env.example") {
		t.Fatalf("expected negated pattern in %v", patterns)
	}
}

func TestCollectFilesToPackHonorsNegatedPatterns(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldWD)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	files := map[string]string{
		".env":         "secret",
		".env.example": "template",
		".git/config":  "gitdir",
		"keep.txt":     "keep",
	}
	for name, contents := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(name, []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	patterns := append(append([]string{}, defaultIgnorePatterns...), ".env*", "!.env.example")

	filesToPack, err := collectFilesToPack(".", patterns)
	if err != nil {
		t.Fatalf("collectFilesToPack: %v", err)
	}

	if slices.Contains(filesToPack, ".env") {
		t.Fatalf("did not expect .env in %v", filesToPack)
	}
	if slices.Contains(filesToPack, ".git/config") {
		t.Fatalf("did not expect .git/config in %v", filesToPack)
	}
	if !slices.Contains(filesToPack, ".env.example") {
		t.Fatalf("expected .env.example in %v", filesToPack)
	}
	if !slices.Contains(filesToPack, "keep.txt") {
		t.Fatalf("expected keep.txt in %v", filesToPack)
	}
}

func TestResolveComposeFile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "docker-compose.yaml"), []byte("services: {}\n"), 0644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	composeFile, found, err := resolveComposeFile(tempDir, "")
	if err != nil {
		t.Fatalf("resolveComposeFile: %v", err)
	}
	if !found {
		t.Fatal("expected compose file to be detected")
	}
	if composeFile != "docker-compose.yaml" {
		t.Fatalf("expected docker-compose.yaml, got %q", composeFile)
	}
}

func TestResolveComposeFileConfiguredMissing(t *testing.T) {
	tempDir := t.TempDir()

	_, _, err := resolveComposeFile(tempDir, "docker-compose.yaml")
	if err == nil {
		t.Fatal("expected an error for missing configured compose file")
	}
}

func TestPrintDeploymentRowRendersFlatAndStackDeployments(t *testing.T) {
	var buffer bytes.Buffer
	writer := tabwriter.NewWriter(&buffer, 0, 8, 1, '\t', tabwriter.AlignRight)

	flat := jigtypes.Deployment{
		Name:        "babyl",
		Kind:        "service",
		Rule:        "Host(`babyl.example.test`)",
		Status:      "healthy",
		Lifetime:    "Up 2 minutes",
		HasRollback: true,
	}
	stack := jigtypes.Deployment{
		Name:   "my-stack",
		Kind:   "stack",
		Status: "healthy",
		Children: []jigtypes.Deployment{
			{
				Kind:     "stack-service",
				Name:     "api",
				Rule:     "Host(`api.example.test`)",
				Status:   "healthy",
				Lifetime: "Up 1 minute",
			},
			{
				Kind:     "stack-service",
				Name:     "db",
				Status:   "unhealthy",
				Lifetime: "Exited (1)",
			},
		},
	}

	printDeploymentRow(writer, flat, "", true, false)
	printDeploymentRow(writer, stack, "", true, true)
	writer.Flush()

	output := buffer.String()
	for _, expected := range []string{
		"babyl",
		"service",
		"Host(`babyl.example.test`)",
		"Up 2 minutes",
		"yes",
		"my-stack",
		"stack",
		"|- api",
		"`- db",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestPrintDeploymentRowSingleSwarmService(t *testing.T) {
	buffer := &bytes.Buffer{}
	writer := tabwriter.NewWriter(buffer, 0, 8, 1, '\t', tabwriter.AlignRight)

	deployment := jigtypes.Deployment{
		Name:        "api",
		Kind:        "service",
		Replicas:    3,
		Rule:        "Host(`api.example.test`)",
		Status:      "healthy",
		Lifetime:    "rolling update complete",
		HasRollback: true,
	}

	printDeploymentRow(writer, deployment, "", true, true)
	writer.Flush()

	output := buffer.String()
	for _, expected := range []string{
		"api",
		"service",
		"3",
		"Host(`api.example.test`)",
		"rolling update complete",
		"healthy",
		"yes",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestPrintWorkerBootstrapCommand(t *testing.T) {
	joinToken := jigtypes.ClusterJoinTokenResponse{
		Token:          "abc123",
		ManagerAddress: "10.0.0.2:2377",
	}

	cmd := fmt.Sprintf("curl -fsSL https://deploywithjig.askh.at/worker.sh | JIG_SWARM_JOIN_TOKEN=%q JIG_SWARM_MANAGER_ADDR=%q bash\n", joinToken.Token, joinToken.ManagerAddress)
	if !strings.Contains(cmd, "JIG_SWARM_JOIN_TOKEN=\"abc123\"") || !strings.Contains(cmd, "JIG_SWARM_MANAGER_ADDR=\"10.0.0.2:2377\"") {
		t.Fatalf("unexpected bootstrap command: %s", cmd)
	}
}
