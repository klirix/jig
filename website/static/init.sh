#!/usr/bin/env bash

set -euo pipefail

if [[ -z "${JIG_SSL_EMAIL:-}" ]]; then
  echo -n "Enter email for ssl: "
  read -r JIG_SSL_EMAIL
fi

if [[ -z "${JIG_DOMAIN:-}" ]]; then
  echo -n "Enter domain jig should be available on: "
  read -r JIG_DOMAIN
fi

if [[ -z "${JIG_VERCEL_APIKEY:-}" ]]; then
  echo -n "Enter vercel key (leave empty to use HTTP challenge): "
  read -r JIG_VERCEL_APIKEY
fi

SWARM_STATE="$(docker info --format '{{.Swarm.LocalNodeState}}' 2>/dev/null || true)"
SWARM_CONTROL="$(docker info --format '{{.Swarm.ControlAvailable}}' 2>/dev/null || true)"
SWARM_NODE_ID="$(docker info --format '{{.Swarm.NodeID}}' 2>/dev/null || true)"

ensure_swarm_network() {
  if ! docker network inspect jig >/dev/null 2>&1; then
    docker network create --driver overlay --attachable jig
  fi
}

ensure_swarm_ingress_label() {
  if [[ -z "$SWARM_NODE_ID" ]]; then
    echo "Failed to determine current Swarm node ID" >&2
    exit 1
  fi

  docker node update --label-add jig.ingress=true "$SWARM_NODE_ID" >/dev/null
}

install_swarm() {
  mkdir -p /var/jig

  ensure_swarm_network
  ensure_swarm_ingress_label

  docker service rm jig >/dev/null 2>&1 || true
  docker pull askhatsaiapov/jig:latest

  docker service create \
    --name jig \
    --constraint 'node.role == manager' \
    --network jig \
    --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock \
    --mount type=bind,src=/var/jig,dst=/var/jig \
    --env "JIG_SSL_EMAIL=$JIG_SSL_EMAIL" \
    --env "JIG_DOMAIN=$JIG_DOMAIN" \
    --env "JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY" \
    --label "traefik.enable=true" \
    --label "traefik.docker.network=jig" \
    --label "traefik.http.services.jig.loadbalancer.server.port=5000" \
    --label "traefik.http.middlewares.https-only.redirectscheme.scheme=https" \
    --label "traefik.http.middlewares.https-only.redirectscheme.permanent=true" \
    --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)" \
    --label "traefik.http.routers.jig.entrypoints=web" \
    --label "traefik.http.routers.jig.middlewares=https-only" \
    --label "traefik.http.routers.jig-secure.rule=Host(\`$JIG_DOMAIN\`)" \
    --label "traefik.http.routers.jig-secure.tls.certresolver=defaultresolver" \
    --label "traefik.http.routers.jig-secure.tls=true" \
    --label "traefik.http.routers.jig-secure.entrypoints=websecure" \
    askhatsaiapov/jig:latest

  echo
echo "Detected Docker Swarm manager, deployed Jig as a service."
echo "Marked the current manager node with jig.ingress=true for Traefik placement."
echo "Swarm stacks push images to jig-registry:5000, so every Swarm node must trust that registry as an insecure registry."
echo "Use /worker.sh on additional nodes to join the swarm and configure the daemon."
echo "Your jig instance should be available on: https://$JIG_DOMAIN"
echo "Tail logs with: docker service logs -f jig"
docker service logs jig --tail 20 || true
}

install_standalone() {
  docker stop traefik 2>/dev/null || true
  docker rm traefik 2>/dev/null || true
  docker stop jig 2>/dev/null || true
  docker rm jig 2>/dev/null || true

  docker pull traefik:2.11
  docker pull askhatsaiapov/jig:latest

  docker run -d --name jig \
    -e "JIG_SSL_EMAIL=$JIG_SSL_EMAIL" \
    -e "JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY" \
    -e "JIG_DOMAIN=$JIG_DOMAIN" \
    --label "traefik.http.middlewares.https-only.redirectscheme.scheme=https" \
    --label "traefik.http.middlewares.https-only.redirectscheme.permanent=true" \
    --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)" \
    --label "traefik.http.routers.jig.entrypoints=web" \
    --label "traefik.http.routers.jig.middlewares=https-only" \
    --label "traefik.http.routers.jig-secure.rule=Host(\`$JIG_DOMAIN\`)" \
    --label "traefik.http.routers.jig-secure.tls.certresolver=defaultresolver" \
    --label "traefik.http.routers.jig-secure.tls=true" \
    --label "traefik.http.routers.jig-secure.entrypoints=websecure" \
    --label "traefik.enable=true" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /var/jig:/var/jig \
    askhatsaiapov/jig:latest

  echo
  echo "Your jig instance should be available on: https://$JIG_DOMAIN"
  docker logs --tail 3 jig
}

if [[ "$SWARM_STATE" == "active" && "$SWARM_CONTROL" == "true" ]]; then
  install_swarm
else
  install_standalone
fi
