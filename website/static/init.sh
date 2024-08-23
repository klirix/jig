#!/usr/bin/env bash

if [[ -z "$JIG_SSL_EMAIL" ]]; then
  echo -n "Enter email for ssl: "
  read -r JIG_SSL_EMAIL
fi

if [[ -z "$JIG_DOMAIN" ]]; then
  echo -n "Enter domain jig should be available on: "
  read -r JIG_DOMAIN
fi

if [[ -z "$JIG_VERCEL_APIKEY" ]]; then
  echo -n "Enter vercel key: "
  read -r JIG_VERCEL_APIKEY
fi

docker stop traefik 2>/dev/null
docker rm traefik 2>/dev/null
docker stop jig 2>/dev/null
docker rm jig 2>/dev/null

docker pull traefik:2.11
docker pull askhatsaiapov/jig:latest


docker run -d --name jig \
  -e JIG_SSL_EMAIL=$JIG_SSL_EMAIL -e JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY -e JIG_DOMAIN=$JIG_DOMAIN \
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
docker logs -f --tail 3 jig 