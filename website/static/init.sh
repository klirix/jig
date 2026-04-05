#!/usr/bin/env bash

set -euo pipefail

IMAGE="askhatsaiapov/jig:latest"
JIG_TAILSCALE_LOCAL_PORT="${JIG_TAILSCALE_LOCAL_PORT:-5050}"

normalize_url() {
  local value="$1"
  if [[ "$value" != http://* && "$value" != https://* ]]; then
    value="https://$value"
  fi
  printf '%s' "${value%/}"
}

detect_tailscale_dns_name() {
  local dns_name
  dns_name="$(tailscale status --json 2>/dev/null | grep -m1 -o '"DNSName":"[^"]*"' | cut -d'"' -f4 || true)"
  printf '%s' "${dns_name%.}"
}

configure_tailscale_control_plane() {
  if ! command -v tailscale >/dev/null 2>&1; then
    echo "Tailscale mode requires the tailscale CLI on this host." >&2
    exit 1
  fi

  if ! tailscale status >/dev/null 2>&1; then
    echo "Tailscale is not connected on this host." >&2
    exit 1
  fi

  if [[ -z "${JIG_ADVERTISE_URL:-}" ]]; then
    local dns_name
    dns_name="$(detect_tailscale_dns_name)"
    if [[ -z "$dns_name" ]]; then
      echo -n "Enter the Tailscale HTTPS hostname for this node: "
      read -r dns_name
    fi
    JIG_ADVERTISE_URL="$(normalize_url "$dns_name")"
  else
    JIG_ADVERTISE_URL="$(normalize_url "$JIG_ADVERTISE_URL")"
  fi

  tailscale serve --bg "$JIG_TAILSCALE_LOCAL_PORT"
}

ensure_swarm_network() {
  if ! docker network inspect jig >/dev/null 2>&1; then
    docker network create --driver overlay --attachable --opt encrypted jig
  fi
}

ensure_swarm_ingress_label() {
  if [[ -z "${SWARM_NODE_ID:-}" ]]; then
    echo "Failed to determine current Swarm node ID" >&2
    exit 1
  fi

  docker node update --label-add jig.ingress=true "$SWARM_NODE_ID" >/dev/null
}

prompt_control_mode() {
  if [[ -n "${JIG_CONTROL_MODE:-}" ]]; then
    return
  fi

  local default_mode="public"
  if command -v tailscale >/dev/null 2>&1; then
    default_mode="tailscale"
  fi

  echo -n "Expose the Jig control plane publicly or through Tailscale? [public/tailscale] (default ${default_mode}): "
  read -r JIG_CONTROL_MODE
  JIG_CONTROL_MODE="${JIG_CONTROL_MODE:-$default_mode}"
}

prompt_settings() {
  prompt_control_mode
  case "$JIG_CONTROL_MODE" in
    public|tailscale)
      ;;
    *)
      echo "JIG_CONTROL_MODE must be either public or tailscale." >&2
      exit 1
      ;;
  esac

  if [[ -z "${JIG_SSL_EMAIL:-}" ]]; then
    echo -n "Enter email for ssl: "
    read -r JIG_SSL_EMAIL
  fi

  if [[ "$JIG_CONTROL_MODE" == "public" && -z "${JIG_DOMAIN:-}" ]]; then
    echo -n "Enter domain jig should be available on: "
    read -r JIG_DOMAIN
  fi

  if [[ -z "${JIG_VERCEL_APIKEY:-}" ]]; then
    echo -n "Enter vercel key (leave empty to use HTTP challenge): "
    read -r JIG_VERCEL_APIKEY
  fi

  if [[ "$JIG_CONTROL_MODE" == "public" ]]; then
    JIG_ADVERTISE_URL="$(normalize_url "${JIG_ADVERTISE_URL:-$JIG_DOMAIN}")"
  else
    configure_tailscale_control_plane
  fi
}

print_public_summary() {
  echo
  echo "Your jig instance should be available on: $JIG_ADVERTISE_URL"
}

print_tailscale_summary() {
  echo
  echo "Configured Tailscale Serve for the Jig control plane at: $JIG_ADVERTISE_URL"
  echo "Keep access restricted with Tailscale ACLs."
}

remove_existing_jig() {
  docker service rm jig >/dev/null 2>&1 || true
  docker stop jig >/dev/null 2>&1 || true
  docker rm jig >/dev/null 2>&1 || true
}

install_swarm() {
  mkdir -p /var/jig

  ensure_swarm_network
  ensure_swarm_ingress_label

  remove_existing_jig
  docker pull "$IMAGE"

  if [[ "$JIG_CONTROL_MODE" == "public" ]]; then
    local service_args=(
      service create
      --name jig
      --constraint node.role==manager
      --network jig
      --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock
      --mount type=bind,src=/var/jig,dst=/var/jig
      --env "JIG_SSL_EMAIL=$JIG_SSL_EMAIL"
      --env "JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY"
      --env "JIG_ADVERTISE_URL=$JIG_ADVERTISE_URL"
    )

    if [[ -n "${JIG_DOMAIN:-}" ]]; then
      service_args+=(--env "JIG_DOMAIN=$JIG_DOMAIN")
    fi

    service_args+=(
      --label "traefik.enable=true"
      --label "traefik.docker.network=jig"
      --label "traefik.http.services.jig.loadbalancer.server.port=5000"
      --label "traefik.http.middlewares.https-only.redirectscheme.scheme=https"
      --label "traefik.http.middlewares.https-only.redirectscheme.permanent=true"
      --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)"
      --label "traefik.http.routers.jig.entrypoints=web"
      --label "traefik.http.routers.jig.middlewares=https-only"
      --label "traefik.http.routers.jig-secure.rule=Host(\`$JIG_DOMAIN\`)"
      --label "traefik.http.routers.jig-secure.tls.certresolver=defaultresolver"
      --label "traefik.http.routers.jig-secure.tls=true"
      --label "traefik.http.routers.jig-secure.entrypoints=websecure"
    )
    service_args+=("$IMAGE")
    docker "${service_args[@]}"
  else
    local run_args=(
      run -d
      --name jig
      --network jig
      -p "127.0.0.1:$JIG_TAILSCALE_LOCAL_PORT:5000"
      -e "JIG_SSL_EMAIL=$JIG_SSL_EMAIL"
      -e "JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY"
      -e "JIG_ADVERTISE_URL=$JIG_ADVERTISE_URL"
      -v /var/run/docker.sock:/var/run/docker.sock
      -v /var/jig:/var/jig
    )

    if [[ -n "${JIG_DOMAIN:-}" ]]; then
      run_args+=(-e "JIG_DOMAIN=$JIG_DOMAIN")
    fi

    run_args+=("$IMAGE")
    docker "${run_args[@]}"
  fi

  echo
  echo "Detected Docker Swarm manager."
  echo "Marked the current manager node with jig.ingress=true for Traefik placement."
  echo "Swarm stacks push images to 127.0.0.1:5000 via the routing mesh, so every Swarm node must trust that registry as an insecure registry."
  echo "Block inbound TCP/5000 from external networks. Leaving that port exposed is a serious security risk."
  if [[ "$JIG_CONTROL_MODE" == "public" ]]; then
    echo "Deployed Jig as a Swarm service."
    print_public_summary
    echo "Tail logs with: docker service logs -f jig"
    docker service logs jig --tail 20 || true
  else
    echo "Started Jig as a local manager container so the control plane stays on 127.0.0.1:$JIG_TAILSCALE_LOCAL_PORT."
    print_tailscale_summary
    echo "The Jig API listens on 127.0.0.1:$JIG_TAILSCALE_LOCAL_PORT and is reachable through Tailscale Serve."
    echo "Tail logs with: docker logs -f jig"
    docker logs --tail 20 jig || true
  fi
  echo "Use /worker.sh on additional nodes to join the swarm and configure the daemon."
}

install_standalone() {
  mkdir -p /var/jig

  docker stop traefik 2>/dev/null || true
  docker rm traefik 2>/dev/null || true
  remove_existing_jig

  docker pull traefik:2.11
  docker pull "$IMAGE"

  local run_args=(
    run -d
    --name jig
    -e "JIG_SSL_EMAIL=$JIG_SSL_EMAIL"
    -e "JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY"
    -e "JIG_ADVERTISE_URL=$JIG_ADVERTISE_URL"
    -v /var/run/docker.sock:/var/run/docker.sock
    -v /var/jig:/var/jig
  )

  if [[ -n "${JIG_DOMAIN:-}" ]]; then
    run_args+=(-e "JIG_DOMAIN=$JIG_DOMAIN")
  fi

  if [[ "$JIG_CONTROL_MODE" == "public" ]]; then
    run_args+=(
      --label "traefik.http.middlewares.https-only.redirectscheme.scheme=https"
      --label "traefik.http.middlewares.https-only.redirectscheme.permanent=true"
      --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)"
      --label "traefik.http.routers.jig.entrypoints=web"
      --label "traefik.http.routers.jig.middlewares=https-only"
      --label "traefik.http.routers.jig-secure.rule=Host(\`$JIG_DOMAIN\`)"
      --label "traefik.http.routers.jig-secure.tls.certresolver=defaultresolver"
      --label "traefik.http.routers.jig-secure.tls=true"
      --label "traefik.http.routers.jig-secure.entrypoints=websecure"
      --label "traefik.enable=true"
    )
  else
    run_args+=(-p "127.0.0.1:$JIG_TAILSCALE_LOCAL_PORT:5000")
  fi

  run_args+=("$IMAGE")
  docker "${run_args[@]}"

  if [[ "$JIG_CONTROL_MODE" == "public" ]]; then
    print_public_summary
  else
    print_tailscale_summary
    echo "The Jig API listens on 127.0.0.1:$JIG_TAILSCALE_LOCAL_PORT and is reachable through Tailscale Serve."
  fi
  docker logs --tail 3 jig
}

prompt_settings

SWARM_STATE="$(docker info --format '{{.Swarm.LocalNodeState}}' 2>/dev/null || true)"
SWARM_CONTROL="$(docker info --format '{{.Swarm.ControlAvailable}}' 2>/dev/null || true)"
SWARM_NODE_ID="$(docker info --format '{{.Swarm.NodeID}}' 2>/dev/null || true)"

if [[ "$SWARM_STATE" == "active" && "$SWARM_CONTROL" == "true" ]]; then
  install_swarm
else
  install_standalone
fi
