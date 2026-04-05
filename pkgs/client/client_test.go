package main

import (
	"bytes"
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
		Rule:        "Host(`babyl.example.test`)",
		Status:      "healthy",
		Lifetime:    "Up 2 minutes",
		HasRollback: true,
	}
	stack := jigtypes.Deployment{
		Name:   "my-stack",
		Status: "healthy",
		Children: []jigtypes.Deployment{
			{
				Name:     "api",
				Rule:     "Host(`api.example.test`)",
				Status:   "healthy",
				Lifetime: "Up 1 minute",
			},
			{
				Name:     "db",
				Status:   "unhealthy",
				Lifetime: "Exited (1)",
			},
		},
	}

	printDeploymentRow(writer, flat, "", false)
	printDeploymentRow(writer, stack, "", true)
	writer.Flush()

	output := buffer.String()
	for _, expected := range []string{
		"babyl",
		"Host(`babyl.example.test`)",
		"Up 2 minutes",
		"yes",
		"my-stack",
		"\\_api",
		"\\_db",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}
