#!/bin/bash

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

docker stop jig > /dev/null && docker rm jig > /dev/null

docker pull traefik:latest

docker run -d --name jig \
  -e JIG_SSL_EMAIL=$JIG_SSL_EMAIL -e JIG_SECRET=$JIG_SECRET \
  --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)" \
  --label "traefik.http.routers.jig.tls=true" \
  --label "traefik.http.routers.jig.tls.certresolver=defaultResolver" \
  --label "traefik.enable=true" \
  -v /var/run/docker.sock:/var/run/docker.sock askhatsaiapov/jig:latest
echo "Your jig instance should be available on: https://$JIG_DOMAIN"
echo "Check for a key in the logs: 'docker logs jig'"