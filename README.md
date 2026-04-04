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

Example:

```json
{
  "name": "my-app",
  "domain": "app.example.com",
  "port": 3000,
  "restartPolicy": "unless-stopped",
  "envs": {
    "DATABASE_URL": "@database-url"
  },
  "volumes": [
    "/var/lib/my-app:/app/data"
  ]
}
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
