package main

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"

	jigtypes "askh.at/jig/v2/pkgs/types"
	"github.com/docker/docker/api/types"
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
	if stack.Status != "unhealthy" {
		t.Fatalf("expected stack health to reflect the worst child, got %#v", stack)
	}
	if len(stack.Children) != 2 {
		t.Fatalf("expected two child services, got %#v", stack.Children)
	}
	if stack.Children[0].Name != "api" && stack.Children[1].Name != "api" {
		t.Fatalf("expected api child in %#v", stack.Children)
	}
	if stack.Children[0].Status != "healthy" && stack.Children[1].Status != "healthy" {
		t.Fatalf("expected one healthy child in %#v", stack.Children)
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
