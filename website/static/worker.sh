#!/usr/bin/env bash

set -euo pipefail

if [[ -z "${JIG_SWARM_JOIN_TOKEN:-}" ]]; then
  echo -n "Enter swarm join token: "
  read -r JIG_SWARM_JOIN_TOKEN
fi

if [[ -z "${JIG_SWARM_MANAGER_ADDR:-}" ]]; then
  echo -n "Enter swarm manager address (host:2377): "
  read -r JIG_SWARM_MANAGER_ADDR
fi

if [[ -z "${JIG_SWARM_REGISTRY:-}" ]]; then
  JIG_SWARM_REGISTRY="127.0.0.1:5000"
fi

if [[ -z "${JIG_SWARM_INSECURE_REGISTRY:-}" ]]; then
  JIG_SWARM_INSECURE_REGISTRY="$JIG_SWARM_REGISTRY"
fi

join_cmd=(docker swarm join --token "$JIG_SWARM_JOIN_TOKEN" "$JIG_SWARM_MANAGER_ADDR")

docker info --format '{{.Swarm.LocalNodeState}}' 2>/dev/null | grep -q '^active$' && {
  echo "This node is already part of a swarm" >&2
  exit 1
}

if command -v jq >/dev/null 2>&1; then
  daemon_json="/etc/docker/daemon.json"
  tmp_json="$(mktemp)"
  if [[ -f "$daemon_json" ]]; then
    jq --arg registry "$JIG_SWARM_INSECURE_REGISTRY" '
      . + {"insecure-registries": ((."insecure-registries" // []) + [$registry] | unique)}
    ' "$daemon_json" > "$tmp_json"
  else
    jq --arg registry "$JIG_SWARM_INSECURE_REGISTRY" \
      '{ "insecure-registries": [$registry] }' > "$tmp_json"
  fi
  sudo install -m 0644 "$tmp_json" "$daemon_json"
  rm -f "$tmp_json"
  echo "Updated Docker daemon insecure registries: $JIG_SWARM_INSECURE_REGISTRY"
  echo "Restart Docker before joining the swarm."
else
  echo "jq is not installed; skipping daemon.json update for insecure registries." >&2
  echo "Add $JIG_SWARM_INSECURE_REGISTRY to /etc/docker/daemon.json on this node and restart Docker." >&2
fi

echo "Joining swarm..."
"${join_cmd[@]}"

echo
echo "Node joined the swarm."
echo "Docker on this node should trust $JIG_SWARM_INSECURE_REGISTRY as an insecure registry."
echo "Block inbound TCP/5000 from external networks. If port 5000 is left exposed, the registry is a serious security risk."
echo "If this is an ingress node, label it on the manager:"
echo "docker node update --label-add jig.ingress=true <node-name>"
