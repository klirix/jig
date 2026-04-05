package main

import "testing"

func TestSwarmRegistryHost(t *testing.T) {
	if got := swarmRegistryHost(); got != "jig-registry:5000" {
		t.Fatalf("expected named swarm registry host, got %q", got)
	}
}
