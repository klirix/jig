# Jig — Dead simple deployment tool

noun

&nbsp; _a device that holds a piece of work and guides the tool operating on it._

## Warning

Jig is still experimental and client/server versions are expected to stay in sync.

## Installation

Installation is done in two steps:

1. Server setup: run a startup script on the server
2. Client setup: install the CLI and log in

### Server setup

Docker is required on the server.

```bash
curl -fsSL https://deploywithjig.askh.at/init.sh | bash
```

This installs Traefik and Jig, asks for the initial settings, and prints a login command for the client.

### Swarm-backed server setup

Use this path when the Jig server itself should run on a Docker Swarm manager and deploy single-service apps as Swarm services.

The current `init.sh` script is for the standalone Docker mode. For Swarm, deploy the server manually:

1. Initialize Swarm on the manager node if it is not already active:

```bash
docker swarm init
```

2. Create the shared overlay network that Jig and Traefik will use:

```bash
docker network create --driver overlay --attachable jig
```

3. Create a persistent directory for Jig state on the manager:

```bash
mkdir -p /var/jig
```

4. Choose the public domain for the Jig API and export the required env vars:

```bash
export JIG_DOMAIN=jig.example.com
export JIG_SSL_EMAIL=ops@example.com
export JIG_VERCEL_APIKEY=your-vercel-token
```

If you do not use Vercel DNS challenge, leave `JIG_VERCEL_APIKEY` unset and Jig will configure Traefik with the HTTP challenge instead.

5. Pull the Jig image:

```bash
docker pull askhatsaiapov/jig:latest
```

6. Deploy the Jig server itself as a Swarm service on a manager node:

```bash
docker service create \
  --name jig \
  --constraint 'node.role == manager' \
  --network jig \
  --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock \
  --mount type=bind,src=/var/jig,dst=/var/jig \
  --env JIG_DOMAIN=$JIG_DOMAIN \
  --env JIG_SSL_EMAIL=$JIG_SSL_EMAIL \
  --env JIG_VERCEL_APIKEY=$JIG_VERCEL_APIKEY \
  --label traefik.enable=true \
  --label traefik.docker.network=jig \
  --label traefik.http.services.jig.loadbalancer.server.port=5000 \
  --label 'traefik.http.middlewares.https-only.redirectscheme.scheme=https' \
  --label 'traefik.http.middlewares.https-only.redirectscheme.permanent=true' \
  --label "traefik.http.routers.jig.rule=Host(\`$JIG_DOMAIN\`)" \
  --label 'traefik.http.routers.jig.entrypoints=web' \
  --label 'traefik.http.routers.jig.middlewares=https-only' \
  --label "traefik.http.routers.jig-secure.rule=Host(\`$JIG_DOMAIN\`)" \
  --label 'traefik.http.routers.jig-secure.entrypoints=websecure' \
  --label 'traefik.http.routers.jig-secure.tls=true' \
  --label 'traefik.http.routers.jig-secure.tls.certresolver=defaultresolver' \
  askhatsaiapov/jig:latest
```

7. Watch the service logs until startup completes:

```bash
docker service logs -f jig
```

On startup, Jig will detect that it is running on a Swarm manager, ensure the `jig` overlay network exists, and create a central Traefik Swarm service if one is not already present.

8. Fetch the initial login token from the service logs:

```bash
docker service logs jig --tail 50
```

Look for the printed `jig login https://...+TOKEN` line and use that on your workstation.

9. Point DNS for `JIG_DOMAIN` at the Swarm manager or the nodes handling the published Traefik ports `80` and `443`.

10. Deploy applications normally with `jig deploy`.

Notes for Swarm-backed deployments:

- Single-service non-Compose apps are deployed as Docker services.
- Compose deployments still use `docker compose` on the server and are listed alongside other deployments.
- Swarm apps with bind mounts must set `placement.requiredNodeLabels` in `jig.json`.
- Jig stats are not available for Swarm deployments.
- The Jig server service should stay constrained to a manager because it needs Docker API access and Swarm control-plane access.

### Client setup

Install the CLI:

```bash
curl -fsSL https://deploywithjig.askh.at/install.sh | bash
```

Then log in with the token printed during server setup:

```bash
jig login <token>
```

Create a starter config in a project directory:

```bash
jig init
```

## Deployments

Jig supports two deployment modes.

### Single-container deployments

This is the original mode. `jig deploy` uploads the project and builds the image on the server. `jig deploy -l` builds the image locally and uploads the image instead.

Supported `jig.json` fields in this mode:

- `name`
- `port`
- `restartPolicy`
- `domain`
- `hostname`
- `rule`
- `envs`
- `exposePorts`
- `volumes`
- `middlewares`
- `placement.requiredNodeLabels` in Swarm mode when bind mounts are used

Example:

Minimal regular deployment:

`jig.json`

```json
{
  "name": "frontend",
  "domain": "app.example.com",
  "port": 3000,
  "restartPolicy": "unless-stopped",
  "envs": {
    "API_URL": "https://api.example.com",
    "SESSION_SECRET": "@session-secret"
  },
  "volumes": ["/var/lib/my-app:/app/data"]
}
```

`Dockerfile`

```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY . .
RUN npm install
RUN npm run build
CMD ["npm", "start"]
```

Swarm-specific example with a bind mount pinned to labeled nodes:

```json
{
  "name": "frontend",
  "domain": "app.example.com",
  "port": 3000,
  "restartPolicy": "unless-stopped",
  "volumes": ["/var/lib/frontend:/app/data"],
  "placement": {
    "requiredNodeLabels": {
      "jig.disk": "frontend-data"
    }
  }
}
```

Apply the matching label on the target node before deploying:

```bash
docker node update --label-add jig.disk=frontend-data <node-name>
```

### Compose deployments

If the project contains `docker-compose.yaml`, `docker-compose.yml`, `compose.yaml`, or `compose.yml`, Jig deploys it through `docker compose`.

You can pin the compose file and, for legacy single-deployment compose projects, the primary routed service in `jig.json`:

```json
{
  "name": "my-stack",
  "composeFile": "docker-compose.yaml",
  "composeService": "web",
  "domain": "app.example.com",
  "envs": {
    "DATABASE_URL": "@database-url"
  }
}
```

Supported `jig.json` fields in compose mode:

- `name`
- `composeFile`
- `composeService`
- `restartPolicy`
- `domain`
- `hostname`
- `rule`
- `envs`
- `middlewares`

Behavior in compose mode:

- Legacy compose mode treats the whole compose project as one Jig deployment and applies `jig.json` routing to one primary service.
- Compose services can opt into separate Jig deployments with an `x-jig` block inside the compose file.
- Each service with `x-jig` becomes its own Jig deployment with its own route, middleware, and resolved secrets.
- Services without `x-jig` still run as part of the compose project but remain internal to Jig.
- Jig connects each managed `x-jig` service container to the `jig` Docker network.
- Volumes must be declared in the compose file itself. `jig.json` `volumes` are rejected for compose deployments.
- `jig deploy -l` is not supported for compose deployments.
- Rollback is not supported for compose deployments.

Example multi-deployment compose file:

```yaml
services:
  frontend:
    build: .
    x-jig:
      name: frontend
      domain: app.example.com
      envs:
        PUBLIC_API_URL: https://api.example.com

  api:
    build: ./api
    x-jig:
      name: api
      domain: api.example.com
      envs:
        DATABASE_URL: "@database-url"

  db:
    image: postgres:16
```

Matching top-level `jig.json` for the compose project:

```json
{
  "name": "my-stack",
  "composeFile": "docker-compose.yaml",
  "restartPolicy": "unless-stopped"
}
```

This gives you:

- one Jig deployment named `frontend` routed at `app.example.com`
- one Jig deployment named `api` routed at `api.example.com`
- one internal `db` service that still runs but does not appear as its own Jig deployment

## Secrets

Secrets are stored on the Jig server and can be referenced from `jig.json` env values with an `@` prefix.

Example:

```json
{
  "envs": {
    "API_TOKEN": "@prod-api-token"
  }
}
```

CLI support:

```bash
jig secrets ls
jig secrets add <name> <value>
jig secrets rm <name>
jig secrets inspect <name>
```

## Other CLI features

Jig currently supports:

- listing deployments with `jig ls`
- deleting deployments
- rollback for single-container deployments
- viewing deployment logs
- viewing deployment stats
- managing secrets
- managing auth tokens

## Notes

- Traefik is used for HTTP/TLS routing.
- Compose deployments require `docker compose` to be available on the server.
- `.jigignore` controls which project files are uploaded during deployment.

## Testing

- Unit tests: `env GOCACHE=/tmp/go-build-cache go test ./...`
- Compose end-to-end integration tests: `make test-integration`
- Integration tests require a working Docker daemon and are opt-in through the `integration` build tag.
- The integration target runs only `TestComposeDeploymentE2E` to avoid unrelated package tests that use a shared on-disk SQLite fixture.
