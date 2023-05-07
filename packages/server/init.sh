#!/usr/bin/env bash

if [[ -z "$JIG_SECRET" ]]; then
  echo -n "Enter JIG SECRET: "
  read -r JIG_SECRET
fi

if [[ -z "$JIG_SSL_EMAIL" ]]; then
  echo -n "Enter email for ssl: "
  read -r JIG_SSL_EMAIL
fi

if [[ -z "$JIG_DOMAIN" ]]; then
  echo -n "Enter domain jig should be available on: "
  read -r JIG_DOMAIN
fi

docker stop jig
docker rm jig

docker pull traefik:latest
docker pull askhatsaiapov/jig:latest

docker run -d --name jig \
  -e JIG_SSL_EMAIL=$JIG_SSL_EMAIL -e JIG_SECRET=$JIG_SECRET  -e JIG_DOMAIN=$JIG_DOMAIN \
  --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)" \
  --label "traefik.http.routers.jig.tls=true" \
  --label "traefik.http.routers.jig.middlewares=https-only" \
  --label "traefik.http.routers.jig.tls.certresolver=defaultresolver" \
  --label "traefik.enable=true" \
  --label "traefik.http.middlewares.https-only.redirectscheme.scheme=https" \
  -v /var/run/docker.sock:/var/run/docker.sock askhatsaiapov/jig:latest
echo 
echo "Your jig instance should be available on: https://$JIG_DOMAIN"
docker exec jig node makeKey.js