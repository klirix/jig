package main

import (
	"os"
	"testing"
)

func TestSwarmRegistryHost(t *testing.T) {
	if got := swarmRegistryHost(); got != "127.0.0.1:5000" {
		t.Fatalf("expected routing-mesh registry host, got %q", got)
	}
}

func TestAdvertisedServerURL(t *testing.T) {
	t.Setenv("JIG_ADVERTISE_URL", "")
	t.Setenv("JIG_DOMAIN", "")
	if got := advertisedServerURL(); got != "" {
		t.Fatalf("expected empty advertised url, got %q", got)
	}

	t.Setenv("JIG_DOMAIN", "jig.example.com")
	if got := advertisedServerURL(); got != "https://jig.example.com" {
		t.Fatalf("expected domain-based advertised url, got %q", got)
	}

	t.Setenv("JIG_ADVERTISE_URL", "https://jig.tailnet.ts.net")
	if got := advertisedServerURL(); got != "https://jig.tailnet.ts.net" {
		t.Fatalf("expected explicit advertised url, got %q", got)
	}

	_ = os.Unsetenv("JIG_ADVERTISE_URL")
	_ = os.Unsetenv("JIG_DOMAIN")
}
