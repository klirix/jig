package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
)

func compareLabels(t *testing.T, expected, actual map[string]string) {
	t.Helper()

	if !maps.Equal(expected, actual) {
		t.Fatalf("expected labels %#v, got %#v", expected, actual)
	}
}

func ptr[T any](b T) *T {
	return &b
}

func labelConfigString(t *testing.T, config jigtypes.DeploymentConfig) string {
	t.Helper()

	configString, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	return string(configString)
}

func TestMakeLabels(t *testing.T) {
	t.Run("basic deployment", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{
			Name:        "jig",
			Domain:      "jig.app",
			Rule:        "",
			Middlewares: jigtypes.DeploymentMiddleares{},
		}
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"jig.config":             labelConfigString(t, config),
			"traefik.http.middlewares.https-only.redirectscheme.permanent":     "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":        "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:             "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`: "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:              "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:      "websecure",
			"traefik.http.routers." + config.Name + `-secure.middlewares`:      "https-only",
			"traefik.enable": "true",
			"traefik.http.routers." + config.Name + `.rule`:        "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `.entrypoints`: "web",
			"traefik.http.routers." + config.Name + `.middlewares`: "https-only",
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})

	t.Run("custom rule", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{
			Name:        "jig",
			Rule:        "Host(`jig.app`) && PathPrefix(`/api`)",
			Middlewares: jigtypes.DeploymentMiddleares{},
		}
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"jig.config":             labelConfigString(t, config),
			"traefik.http.middlewares.https-only.redirectscheme.permanent":     "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":        "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:             "Host(`jig.app`) && PathPrefix(`/api`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`: "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:              "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:      "websecure",
			"traefik.http.routers." + config.Name + `-secure.middlewares`:      "https-only",
			"traefik.enable": "true",
			"traefik.http.routers." + config.Name + `.rule`:        "Host(`jig.app`) && PathPrefix(`/api`)",
			"traefik.http.routers." + config.Name + `.entrypoints`: "web",
			"traefik.http.routers." + config.Name + `.middlewares`: "https-only",
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})

	t.Run("internal service deployment", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{
			Name: "jig",
			Middlewares: jigtypes.DeploymentMiddleares{
				NoHTTP: ptr(true),
			},
		}
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"jig.config":             labelConfigString(t, config),
			"traefik.enable":         "false",
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})

	t.Run("multiple middlewares", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{
			Name:   "jig",
			Domain: "jig.app",
			Rule:   "",
			Middlewares: jigtypes.DeploymentMiddleares{
				Compression: ptr(true),
			},
		}
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"jig.config":             labelConfigString(t, config),
			"traefik.http.middlewares.https-only.redirectscheme.permanent":     "true",
			"traefik.http.middlewares.compress.compress":                       "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":        "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:             "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`: "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:              "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:      "websecure",
			"traefik.http.routers." + config.Name + `-secure.middlewares`:      "https-only, compress",
			"traefik.enable": "true",
			"traefik.http.routers." + config.Name + `.rule`:        "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `.entrypoints`: "web",
			"traefik.http.routers." + config.Name + `.middlewares`: "https-only, compress",
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})

	t.Run("addPrefix stripPrefix middlewares", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{
			Name:   "jig",
			Domain: "jig.app",
			Rule:   "",
			Middlewares: jigtypes.DeploymentMiddleares{
				Compression: ptr(true),
				AddPrefix:   ptr("/api"),
				StripPrefix: &[]string{"/papi", "/mami"},
			},
		}
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"jig.config":             labelConfigString(t, config),
			"traefik.http.middlewares.https-only.redirectscheme.permanent":                  "true",
			"traefik.http.middlewares.compress.compress":                                    "true",
			"traefik.http.middlewares.addPrefix-" + config.Name + ".addprefix":              "/api",
			"traefik.http.middlewares.stripPrefix-" + config.Name + ".stripprefix.prefixes": "/papi,/mami",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":                     "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:                          "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`:              "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:                           "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:                   "websecure",
			"traefik.http.routers." + config.Name + `-secure.middlewares`:                   "https-only, compress, addPrefix-" + config.Name + ", stripPrefix-" + config.Name,
			"traefik.enable": "true",
			"traefik.http.routers." + config.Name + `.rule`:        "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `.entrypoints`: "web",
			"traefik.http.routers." + config.Name + `.middlewares`: "https-only, compress, addPrefix-" + config.Name + ", stripPrefix-" + config.Name,
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})
	t.Run("ratelimiting middleware", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{
			Name:   "jig",
			Domain: "jig.app",
			Rule:   "",
			Middlewares: jigtypes.DeploymentMiddleares{
				Compression: ptr(true),
				RateLimiting: &jigtypes.RateLimitMiddleware{
					Average: 100,
					Burst:   200,
				},
			},
		}
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"jig.config":             labelConfigString(t, config),
			"traefik.http.middlewares.https-only.redirectscheme.permanent":             "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":                "https",
			"traefik.http.middlewares.compress.compress":                               "true",
			"traefik.http.middlewares.ratelimit-" + config.Name + ".ratelimit.average": "100",
			"traefik.http.middlewares.ratelimit-" + config.Name + ".ratelimit.burst":   "200",
			"traefik.http.routers." + config.Name + `-secure.rule`:                     "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`:         "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:                      "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:              "websecure",
			"traefik.http.routers." + config.Name + `-secure.middlewares`:              "https-only, compress, ratelimit-" + config.Name,
			"traefik.enable": "true",
			"traefik.http.routers." + config.Name + `.rule`:        "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `.entrypoints`: "web",
			"traefik.http.routers." + config.Name + `.middlewares`: "https-only, compress, ratelimit-" + config.Name,
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})
}

func TestPickComposePrimaryService(t *testing.T) {
	t.Run("explicit compose service", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{Name: "app", ComposeService: "web"}
		service, err := pickComposePrimaryService(config, []string{"web", "worker"})
		if err != nil {
			t.Fatalf("pickComposePrimaryService: %v", err)
		}
		if service != "web" {
			t.Fatalf("expected web, got %q", service)
		}
	})

	t.Run("fallback to deployment name", func(t *testing.T) {
		config := jigtypes.DeploymentConfig{Name: "app"}
		service, err := pickComposePrimaryService(config, []string{"worker", "app"})
		if err != nil {
			t.Fatalf("pickComposePrimaryService: %v", err)
		}
		if service != "app" {
			t.Fatalf("expected app, got %q", service)
		}
	})
}

func TestMakeComposeOverride(t *testing.T) {
	override := makeComposeOverride([]composeManagedService{
		{
			StackName:   "stack",
			ServiceName: "web",
			DisplayName: "frontend",
			Config: jigtypes.DeploymentConfig{
				Name:          "stack-frontend",
				ComposeFile:   "docker-compose.yaml",
				RestartPolicy: "unless-stopped",
				Domain:        "app.example.com",
				Middlewares:   jigtypes.DeploymentMiddleares{},
			},
			Envs: map[string]string{
				"API_TOKEN": "secret",
			},
		},
		{
			StackName:   "stack",
			ServiceName: "api",
			DisplayName: "api",
			Config: jigtypes.DeploymentConfig{
				Name:        "stack-api",
				ComposeFile: "docker-compose.yaml",
				Domain:      "api.example.com",
				Middlewares: jigtypes.DeploymentMiddleares{},
			},
		},
	})

	expectedSnippets := []string{
		`"web":`,
		`"api":`,
		`restart: "unless-stopped"`,
		`"API_TOKEN": "secret"`,
		`"jig.stack": "stack"`,
		`"jig.service": "frontend"`,
		`"jig.service": "api"`,
		`"jig.display-name": "stack:frontend"`,
		`"jig.display-name": "stack:api"`,
		`"jig.name": "stack-frontend"`,
		`"jig.name": "stack-api"`,
		`"jig.deployment-kind": "compose"`,
		`"traefik.http.routers.stack-frontend-secure.rule": "Host(` + "`app.example.com`" + `)"`,
		`"traefik.http.routers.stack-api-secure.rule": "Host(` + "`api.example.com`" + `)"`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(override, snippet) {
			t.Fatalf("expected override to contain %q, got:\n%s", snippet, override)
		}
	}
}

func TestCollectManagedComposeServices(t *testing.T) {
	project := composeProject{
		Services: map[string]composeProjectService{
			"frontend": {
				Jig: &jigtypes.DeploymentConfig{
					Name:   "frontend",
					Domain: "app.example.com",
					Envs: map[string]string{
						"PUBLIC_API_URL": "https://api.example.com",
					},
				},
			},
			"api": {
				Jig: &jigtypes.DeploymentConfig{
					Name:   "api",
					Domain: "api.example.com",
					Envs: map[string]string{
						"DATABASE_URL": "postgres://db",
					},
				},
			},
			"db": {},
		},
	}

	managed, err := collectManagedComposeServices(project, jigtypes.DeploymentConfig{
		Name:          "stack",
		ComposeFile:   "docker-compose.yaml",
		RestartPolicy: "unless-stopped",
	}, nil)
	if err != nil {
		t.Fatalf("collectManagedComposeServices: %v", err)
	}
	if len(managed) != 2 {
		t.Fatalf("expected 2 managed services, got %d", len(managed))
	}
	if managed[0].StackName != "stack" || managed[1].StackName != "stack" {
		t.Fatalf("expected stack name to be inherited: %#v", managed)
	}
	if managed[0].DisplayName != "api" && managed[1].DisplayName != "api" {
		t.Fatalf("expected api display name in %#v", managed)
	}
	if managed[0].Config.RestartPolicy != "unless-stopped" || managed[1].Config.RestartPolicy != "unless-stopped" {
		t.Fatalf("expected top-level restart policy to be inherited: %#v", managed)
	}
}

func TestCollectManagedComposeServicesAllowsPerServicePort(t *testing.T) {
	project := composeProject{
		Services: map[string]composeProjectService{
			"api": {
				Jig: &jigtypes.DeploymentConfig{
					Name:   "api",
					Domain: "api.example.com",
					Port:   5174,
				},
			},
		},
	}

	managed, err := collectManagedComposeServices(project, jigtypes.DeploymentConfig{
		Name:        "stack",
		ComposeFile: "docker-compose.yaml",
	}, nil)
	if err != nil {
		t.Fatalf("collectManagedComposeServices: %v", err)
	}
	if len(managed) != 1 {
		t.Fatalf("expected 1 managed service, got %d", len(managed))
	}
	if managed[0].Config.Port != 5174 {
		t.Fatalf("expected managed service port to be preserved, got %#v", managed[0])
	}
}

func TestMakeSwarmStackOverride(t *testing.T) {
	override, err := makeSwarmStackOverride([]composeManagedService{
		{
			StackName:   "stack",
			ServiceName: "web",
			DisplayName: "frontend",
			Config: jigtypes.DeploymentConfig{
				Name:          "stack-frontend",
				Hostname:      "frontend",
				ComposeFile:   "docker-compose.yaml",
				RestartPolicy: "on-failure:3",
				Domain:        "app.example.com",
				Placement: jigtypes.DeploymentPlacement{
					RequiredNodeLabels: map[string]string{"disk": "ssd"},
				},
			},
			Envs: map[string]string{
				"API_TOKEN": "secret",
			},
		},
	})
	if err != nil {
		t.Fatalf("makeSwarmStackOverride: %v", err)
	}

	expectedSnippets := []string{
		`services:`,
		`web:`,
		`hostname: frontend`,
		`API_TOKEN: secret`,
		`networks:`,
		`aliases:`,
		`- frontend`,
		`restart_policy:`,
		`condition: on-failure`,
		`max_attempts: 3`,
		`constraints:`,
		`- node.labels.disk == ssd`,
		`jig.deployment-kind: swarm-stack-service`,
		`jig.stack: stack`,
		`jig.service: frontend`,
		"traefik.http.routers.stack-frontend-secure.rule: Host(`app.example.com`)",
		`networks:`,
		`jig:`,
		`external: true`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(override, snippet) {
			t.Fatalf("expected override to contain %q, got:\n%s", snippet, override)
		}
	}
}

func TestMakeSwarmBuildOverride(t *testing.T) {
	override, images, err := makeSwarmBuildOverride(composeProject{
		Services: map[string]composeProjectService{
			"frontend": {Build: map[string]any{"context": "."}},
			"api":      {Image: "ghcr.io/example/api:latest"},
			"worker":   {Build: "./worker"},
		},
	}, "ringge", "127.0.0.1:5000")
	if err != nil {
		t.Fatalf("makeSwarmBuildOverride: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 built images, got %#v", images)
	}
	for _, expected := range []string{
		"127.0.0.1:5000/jig/ringge/frontend:",
		"127.0.0.1:5000/jig/ringge/worker:",
		"frontend:",
		"worker:",
	} {
		if !strings.Contains(override, expected) {
			t.Fatalf("expected override to contain %q, got:\n%s", expected, override)
		}
	}
	if strings.Contains(override, "ghcr.io/example/api:latest") {
		t.Fatalf("did not expect non-build image to be overridden, got:\n%s", override)
	}
}

func TestWriteSanitizedSwarmComposeFile(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	input := []byte(`
services:
  api:
    build: .
    image: example/api:latest
    x-jig:
      name: api
  worker:
    image: example/worker:latest
`)
	if err := os.WriteFile(composePath, input, 0644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	sanitizedFile, err := writeSanitizedSwarmComposeFile(tempDir, "docker-compose.yaml")
	if err != nil {
		t.Fatalf("writeSanitizedSwarmComposeFile: %v", err)
	}

	output, err := os.ReadFile(filepath.Join(tempDir, sanitizedFile))
	if err != nil {
		t.Fatalf("read sanitized file: %v", err)
	}
	if strings.Contains(string(output), "build:") {
		t.Fatalf("expected build stanza to be removed, got:\n%s", output)
	}
	if strings.Contains(string(output), "x-jig:") {
		t.Fatalf("expected x-jig stanza to be removed, got:\n%s", output)
	}
	if !strings.Contains(string(output), "image: example/api:latest") {
		t.Fatalf("expected image to be preserved, got:\n%s", output)
	}
}

func TestMakeDeployOutputFilter(t *testing.T) {
	recorder := httptest.NewRecorder()
	filter := makeDeployOutputFilter(recorder, "ringge-kit", false)

	filter("Ignoring unsupported options: build")
	filter("Since --detach=false was not specified, tasks will be created in the background.")
	filter("In a future release, --detach=false will become the default.")
	filter("Updating service ringge-kit_api (id: 123)")
	filter("Creating service ringge-kit_frontend (id: 456)")
	filter("unrelated noise")

	output := recorder.Body.String()
	if strings.Contains(output, "Ignoring unsupported options") {
		t.Fatalf("expected unsupported options warning to be filtered, got:\n%s", output)
	}
	if strings.Contains(output, "detach=false") {
		t.Fatalf("expected detach warning to be filtered, got:\n%s", output)
	}
	for _, expected := range []string{"Updating api", "Creating frontend"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestCopyDockerLogStreamDemuxesHeaders(t *testing.T) {
	stream := []byte{
		1, 0, 0, 0, 0, 0, 0, 5,
		'h', 'e', 'l', 'l', 'o',
		2, 0, 0, 0, 0, 0, 0, 5,
		'w', 'o', 'r', 'l', 'd',
	}

	var output bytes.Buffer
	if err := copyDockerLogStream(&output, bytes.NewReader(stream)); err != nil {
		t.Fatalf("copyDockerLogStream: %v", err)
	}
	if output.String() != "helloworld" {
		t.Fatalf("expected demuxed output, got %q", output.String())
	}
}

func TestMakeSwarmConstraints(t *testing.T) {
	config := jigtypes.DeploymentConfig{
		Placement: jigtypes.DeploymentPlacement{
			RequiredNodeLabels: map[string]string{
				"disk": "ssd",
				"zone": "eu-1",
			},
		},
	}

	constraints, err := makeSwarmConstraints(config)
	if err != nil {
		t.Fatalf("makeSwarmConstraints: %v", err)
	}
	expected := []string{
		"node.labels.disk == ssd",
		"node.labels.zone == eu-1",
	}
	if strings.Join(constraints, "|") != strings.Join(expected, "|") {
		t.Fatalf("expected %v, got %v", expected, constraints)
	}
}

func TestValidateSwarmConfig(t *testing.T) {
	t.Run("bind mounts require placement", func(t *testing.T) {
		err := validateSwarmConfig(jigtypes.DeploymentConfig{
			Name:    "app",
			Volumes: []string{"/srv/data:/data"},
		})
		if err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("bind mounts with placement pass", func(t *testing.T) {
		err := validateSwarmConfig(jigtypes.DeploymentConfig{
			Name:    "app",
			Volumes: []string{"/srv/data:/data"},
			Placement: jigtypes.DeploymentPlacement{
				RequiredNodeLabels: map[string]string{"disk": "ssd"},
			},
		})
		if err != nil {
			t.Fatalf("validateSwarmConfig: %v", err)
		}
	})
}

func TestMakeSwarmRestartPolicy(t *testing.T) {
	policy, err := makeSwarmRestartPolicy(jigtypes.DeploymentConfig{RestartPolicy: "on-failure:3"})
	if err != nil {
		t.Fatalf("makeSwarmRestartPolicy: %v", err)
	}
	if policy == nil {
		t.Fatal("expected restart policy")
	}
	if policy.Condition != swarm.RestartPolicyConditionOnFailure {
		t.Fatalf("expected on-failure condition, got %q", policy.Condition)
	}
	if policy.MaxAttempts == nil || *policy.MaxAttempts != 3 {
		t.Fatalf("expected max attempts 3, got %#v", policy.MaxAttempts)
	}
}

func TestMakeSwarmServiceSpec(t *testing.T) {
	spec, err := makeSwarmServiceSpec(jigtypes.DeploymentConfig{
		Name:          "app",
		Port:          8080,
		Domain:        "app.example.com",
		RestartPolicy: "on-failure:4",
		Volumes:       []string{"/srv/data:/data"},
		Placement: jigtypes.DeploymentPlacement{
			RequiredNodeLabels: map[string]string{"disk": "ssd"},
		},
	}, "app:swarm-1", []string{"APP_ENV=prod"})
	if err != nil {
		t.Fatalf("makeSwarmServiceSpec: %v", err)
	}

	if spec.TaskTemplate.ContainerSpec == nil {
		t.Fatal("expected container spec")
	}
	if spec.TaskTemplate.ContainerSpec.Image != "app:swarm-1" {
		t.Fatalf("unexpected image %q", spec.TaskTemplate.ContainerSpec.Image)
	}
	if got := spec.Annotations.Labels["traefik.http.routers.app.rule"]; got != "Host(`app.example.com`)" {
		t.Fatalf("unexpected router rule %q", got)
	}
	if got := spec.Annotations.Labels["traefik.http.services.app.loadbalancer.server.port"]; got != "8080" {
		t.Fatalf("unexpected load balancer port %q", got)
	}
	if len(spec.TaskTemplate.Placement.Constraints) != 1 || spec.TaskTemplate.Placement.Constraints[0] != "node.labels.disk == ssd" {
		t.Fatalf("unexpected constraints %#v", spec.TaskTemplate.Placement.Constraints)
	}
	if spec.TaskTemplate.RestartPolicy == nil || spec.TaskTemplate.RestartPolicy.MaxAttempts == nil || *spec.TaskTemplate.RestartPolicy.MaxAttempts != 4 {
		t.Fatalf("unexpected restart policy %#v", spec.TaskTemplate.RestartPolicy)
	}
}

func TestBuildDeploymentsGroupsComposeStack(t *testing.T) {
	containers := []types.Container{
		{
			ID:     "child1",
			State:  "running",
			Status: "Up",
			Names:  []string{"/stack-frontend"},
			Labels: map[string]string{
				"jig.deployment-kind": "compose",
				"jig.stack":           "stack",
				"jig.service":         "frontend",
				"jig.display-name":    "stack:frontend",
				"jig.name":            "stack-frontend",
				"traefik.http.routers.stack-frontend.rule": "Host(`app.example.com`)",
			},
		},
		{
			ID:     "child2",
			State:  "exited",
			Status: "Exited (0)",
			Names:  []string{"/stack-api"},
			Labels: map[string]string{
				"jig.deployment-kind":                 "compose",
				"jig.stack":                           "stack",
				"jig.service":                         "api",
				"jig.display-name":                    "stack:api",
				"jig.name":                            "stack-api",
				"traefik.http.routers.stack-api.rule": "Host(`api.example.com`)",
			},
		},
		{
			ID:     "solo",
			State:  "running",
			Status: "Up",
			Names:  []string{"/solo"},
			Labels: map[string]string{
				"jig.name":                       "solo",
				"traefik.http.routers.solo.rule": "Host(`solo.example.com`)",
			},
		},
	}

	deployments := buildDeployments(containers)
	if len(deployments) != 2 {
		t.Fatalf("expected 2 top-level deployments, got %#v", deployments)
	}

	if deployments[0].Name != "solo" && deployments[1].Name != "solo" {
		t.Fatalf("expected solo deployment in %#v", deployments)
	}

	var stack jigtypes.Deployment
	for _, deployment := range deployments {
		if deployment.Name == "stack" {
			stack = deployment
		}
	}
	if stack.Name != "stack" {
		t.Fatalf("expected stack deployment in %#v", deployments)
	}
	if stack.Kind != "stack" {
		t.Fatalf("expected generic stack kind, got %#v", stack)
	}
	if stack.Status != "unhealthy" {
		t.Fatalf("expected stack health to reflect the worst child, got %#v", stack)
	}
	if len(stack.Children) != 2 {
		t.Fatalf("expected two child services, got %#v", stack.Children)
	}
	if stack.Children[0].Kind != "stack-service" || stack.Children[1].Kind != "stack-service" {
		t.Fatalf("expected generic stack-service kinds in %#v", stack.Children)
	}
	if stack.Children[0].Name != "api" && stack.Children[1].Name != "api" {
		t.Fatalf("expected api child in %#v", stack.Children)
	}
	if stack.Children[0].Status != "healthy" && stack.Children[1].Status != "healthy" {
		t.Fatalf("expected one healthy child in %#v", stack.Children)
	}

	var solo jigtypes.Deployment
	for _, deployment := range deployments {
		if deployment.Name == "solo" {
			solo = deployment
		}
	}
	if solo.Name != "solo" {
		t.Fatalf("expected solo deployment in %#v", deployments)
	}
	if solo.Kind != "service" {
		t.Fatalf("expected standalone single to render as service, got %#v", solo)
	}
	if solo.Rule != "Host(`solo.example.com`)" {
		t.Fatalf("expected solo rule to be preserved, got %#v", solo)
	}
	if solo.Status != "healthy" {
		t.Fatalf("expected solo to be healthy, got %#v", solo)
	}
	if solo.HasRollback {
		t.Fatalf("did not expect solo to have rollback, got %#v", solo)
	}
}

func TestBuildSwarmDeploymentsGroupsStacks(t *testing.T) {
	replicas := uint64(2)
	services := []swarm.Service{
		{
			ID: "svc-single",
			Spec: swarm.ServiceSpec{
				Annotations: swarm.Annotations{
					Labels: map[string]string{
						"jig.name":                      "api",
						"jig.deployment-kind":           "swarm",
						"traefik.http.routers.api.rule": "Host(`api.example.com`)",
					},
				},
				Mode: swarm.ServiceMode{
					Replicated: &swarm.ReplicatedService{Replicas: &replicas},
				},
			},
			ServiceStatus: &swarm.ServiceStatus{RunningTasks: 2, DesiredTasks: 2},
		},
		{
			ID: "svc-stack-web",
			Spec: swarm.ServiceSpec{
				Annotations: swarm.Annotations{
					Labels: map[string]string{
						"jig.name":                            "stack-web",
						"jig.deployment-kind":                 "swarm-stack-service",
						"jig.stack":                           "stack",
						"jig.service":                         "web",
						"traefik.http.routers.stack-web.rule": "Host(`app.example.com`)",
					},
				},
				Mode: swarm.ServiceMode{
					Replicated: &swarm.ReplicatedService{Replicas: ptr(uint64(1))},
				},
			},
			ServiceStatus: &swarm.ServiceStatus{RunningTasks: 1, DesiredTasks: 1},
		},
		{
			ID: "svc-stack-worker",
			Spec: swarm.ServiceSpec{
				Annotations: swarm.Annotations{
					Labels: map[string]string{
						"jig.name":            "stack-worker",
						"jig.deployment-kind": "swarm-stack-service",
						"jig.stack":           "stack",
						"jig.service":         "worker",
					},
				},
				Mode: swarm.ServiceMode{
					Replicated: &swarm.ReplicatedService{Replicas: ptr(uint64(1))},
				},
			},
			ServiceStatus: &swarm.ServiceStatus{RunningTasks: 0, DesiredTasks: 1},
		},
	}

	deployments := buildSwarmDeployments(services)
	if len(deployments) != 2 {
		t.Fatalf("expected 2 top-level deployments, got %#v", deployments)
	}
	if deployments[0].Name != "api" || deployments[0].Kind != "service" {
		t.Fatalf("expected singular service first, got %#v", deployments[0])
	}
	if deployments[0].Rule != "Host(`api.example.com`)" {
		t.Fatalf("expected singular swarm rule to be preserved, got %#v", deployments[0])
	}
	if deployments[0].Replicas != 2 {
		t.Fatalf("expected singular swarm replicas to be shown, got %#v", deployments[0])
	}
	if deployments[0].Status != "healthy" {
		t.Fatalf("expected singular swarm service to be healthy, got %#v", deployments[0])
	}
	if deployments[1].Name != "stack" || deployments[1].Kind != "stack" {
		t.Fatalf("expected stack parent, got %#v", deployments[1])
	}
	if deployments[1].Status != "unhealthy" {
		t.Fatalf("expected unhealthy stack when one child is down, got %#v", deployments[1])
	}
	if len(deployments[1].Children) != 2 {
		t.Fatalf("expected 2 stack children, got %#v", deployments[1].Children)
	}
	for _, child := range deployments[1].Children {
		if child.Kind != "stack-service" {
			t.Fatalf("expected stack-service kind, got %#v", child)
		}
		if child.ParentName != "stack" {
			t.Fatalf("expected parent name stack, got %#v", child)
		}
	}
}

func TestDeploymentRepresentativeScorePrefersLiveContainerOverRollback(t *testing.T) {
	name := "jig-website"
	runningCurrent := types.Container{
		State:  "running",
		Names:  []string{"/" + name},
		Labels: map[string]string{"jig.name": name},
	}
	rollback := types.Container{
		State:  "exited",
		Names:  []string{"/" + name + "-prev"},
		Labels: map[string]string{"jig.name": name},
	}

	currentScore := deploymentRepresentativeScore(name, runningCurrent)
	rollbackScore := deploymentRepresentativeScore(name, rollback)

	if currentScore <= rollbackScore {
		t.Fatalf("expected current container score %d to be greater than rollback score %d", currentScore, rollbackScore)
	}
	if !isRollbackContainer(name, rollback) {
		t.Fatal("expected rollback container to be detected")
	}
	if isRollbackContainer(name, runningCurrent) {
		t.Fatal("did not expect current container to be treated as rollback")
	}
}
