package main

import (
	"encoding/json"
	"maps"
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
			ServiceName: "web",
			Config: jigtypes.DeploymentConfig{
				Name:          "frontend",
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
			ServiceName: "api",
			Config: jigtypes.DeploymentConfig{
				Name:        "api",
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
		`"jig.name": "frontend"`,
		`"jig.name": "api"`,
		`"jig.deployment-kind": "compose"`,
		`"traefik.http.routers.frontend-secure.rule": "Host(` + "`app.example.com`" + `)"`,
		`"traefik.http.routers.api-secure.rule": "Host(` + "`api.example.com`" + `)"`,
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
	if managed[0].Config.RestartPolicy != "unless-stopped" || managed[1].Config.RestartPolicy != "unless-stopped" {
		t.Fatalf("expected top-level restart policy to be inherited: %#v", managed)
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
