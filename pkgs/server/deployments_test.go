package main

import (
	"fmt"
	"testing"

	jigtypes "askh.at/jig/v2/pkgs/types"
)

func compareLabels(t *testing.T, expected, actual map[string]string) {
	if len(expected) != len(actual) {
		fmt.Printf("Actual : %v", actual)
		t.Errorf("Expected %d labels, but got %d", len(expected), len(actual))
	}

	for key, value := range expected {
		if actual[key] != value {
			t.Errorf("Expected label %s to have value %s, but got %s", key, value, actual[key])
		}
	}
}

func ptr[T any](b T) *T {
	return &b
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
			"traefik.http.middlewares.https-only.redirectscheme.permanent":     "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":        "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:             "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`: "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:              "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:      "websecure",
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
			"traefik.http.middlewares.https-only.redirectscheme.permanent":     "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":        "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:             "Host(`jig.app`) && PathPrefix(`/api`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`: "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:              "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:      "websecure",
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
		fmt.Printf("%#v\n", config)
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"traefik.http.middlewares.https-only.redirectscheme.permanent":     "true",
			"traefik.http.middlewares.compress.compress":                       "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":        "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:             "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`: "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:              "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:      "websecure",
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
		fmt.Printf("%#v\n", config)
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"traefik.http.middlewares.https-only.redirectscheme.permanent":                  "true",
			"traefik.http.middlewares.compress.compress":                                    "true",
			"traefik.http.middlewares.addPrefix-" + config.Name + ".addprefix":              "/api",
			"traefik.http.middlewares.stripPrefix-" + config.Name + ".stripprefix.prefixes": "/papi,/mami",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":                     "https",
			"traefik.http.routers." + config.Name + `-secure.rule`:                          "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`:              "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:                           "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:                   "websecure",
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
		fmt.Printf("%#v\n", config)
		expected := map[string]string{
			"traefik.docker.network": "jig",
			"jig.name":               config.Name,
			"traefik.http.middlewares.https-only.redirectscheme.permanent":             "true",
			"traefik.http.middlewares.https-only.redirectscheme.scheme":                "https",
			"traefik.http.middlewares.compress.compress":                               "true",
			"traefik.http.middlewares.ratelimit-" + config.Name + ".ratelimit.average": "100",
			"traefik.http.middlewares.ratelimit-" + config.Name + ".ratelimit.burst":   "200",
			"traefik.http.routers." + config.Name + `-secure.rule`:                     "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `-secure.tls.certresolver`:         "defaultresolver",
			"traefik.http.routers." + config.Name + `-secure.tls`:                      "true",
			"traefik.http.routers." + config.Name + `-secure.entrypoints`:              "websecure",
			"traefik.enable": "true",
			"traefik.http.routers." + config.Name + `.rule`:        "Host(`jig.app`)",
			"traefik.http.routers." + config.Name + `.entrypoints`: "web",
			"traefik.http.routers." + config.Name + `.middlewares`: "https-only, compress, ratelimit-" + config.Name,
		}

		actual := makeLabels(config)

		compareLabels(t, expected, actual)
	})
}
