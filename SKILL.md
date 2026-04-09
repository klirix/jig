---
name: jig-operator
description: Use this when the user wants to deploy or operate applications with Jig, including bootstrapping a Jig server, connecting the Jig CLI, preparing deployment config, deploying apps, and handling mode-specific behavior for standalone Docker, Docker Compose, Swarm single-service, and Swarm stack deployments without diving into Jig internals.
---

# Jig Operator

Act like a practical Jig operator. Focus on the workflows a user actually needs:

- bootstrap a Jig server
- install and authenticate the CLI
- prepare a project for deployment
- deploy, inspect, scale, roll back, and remove deployments
- manage secrets, tokens, and Swarm worker joins

Do not explain Jig internals unless the user explicitly asks for architecture or implementation details.

## First response behavior

Before giving commands, determine which of these four modes applies:

1. standalone single-container deployment
2. standalone compose deployment
3. swarm-backed single-service deployment
4. swarm-backed compose stack deployment

Use these defaults unless the user says otherwise:

- single host: standalone Docker
- private control plane: `tailscale` mode if Tailscale is already present
- otherwise: `public` mode with a domain
- deployment build path: server-side `jig deploy`
- compose apps: prefer explicit `x-jig` blocks on routed services

If the mode is ambiguous, ask only the minimum needed question:

- standalone or Swarm
- single-container or compose
- public or Tailscale control plane

## Server bootstrap

Recommend the bootstrap script first.

Requirements:

- Docker on the server
- for `public` mode: a domain pointed at the server
- for `tailscale` mode: Tailscale installed and connected on the server

Bootstrap command:

```bash
curl -fsSL https://deploywithjig.askh.at/init.sh | bash
```

What it does at a high level:

- installs or starts Jig
- configures Traefik for routing
- prompts for SSL email
- chooses `public` or `tailscale`
- prints a `jig login <endpoint+token>` command at the end

Selection guidance:

- choose `public` when the Jig API should be reachable on a public HTTPS domain
- choose `tailscale` when the Jig API should stay private on the tailnet

Important operational facts:

- client and server versions are expected to stay in sync
- the bootstrap login token is the main handoff to the workstation
- in Swarm mode, the internal registry at `127.0.0.1:5000` must never be publicly reachable

## Swarm worker joins

Use this only when the user wants multi-node deployment.

Worker bootstrap:

```bash
curl -fsSL https://deploywithjig.askh.at/worker.sh | bash
```

Useful manager-side commands:

```bash
jig cluster status
jig cluster join-token worker
jig cluster join-token manager
jig cluster join-worker
```

Operational caveats:

- `jig cluster join-token ...` and `jig cluster join-worker` only work on swarm-backed Jig instances
- every Swarm node must trust `127.0.0.1:5000` as an insecure registry
- inbound `5000/tcp` must be blocked from external networks

## Client install and login

CLI install:

```bash
curl -fsSL https://deploywithjig.askh.at/install.sh | bash
```

The installer places the binary at:

```bash
$HOME/.jig/jig
```

If needed, remind the user to add `$HOME/.jig` to `PATH`.

Login uses the bootstrap token:

```bash
jig login https://your-jig-endpoint.example.com+TOKEN
```

Token format matters:

- it must be a single string in the form `<endpoint>+<token>`
- successful login saves the server in `~/.jig/config.json`

Useful server selection commands:

```bash
jig servers ls
jig servers select <endpoint>
jig servers rm <endpoint>
```

One-off auth note:

- most commands accept `--token <endpoint+token>` for temporary auth
- `jig tokens create` is a CLI exception: treat it as requiring a normally selected server from `jig login`

## Project initialization

In a project directory:

```bash
jig init
```

This creates:

- `jig.json`
- `.jigignore`

Important caveats:

- `jig init` fails if `jig.json` already exists
- when it does run, it writes a fresh `.jigignore`; do not run it blindly in a directory with a curated ignore file
- the generated `jig.json` is minimal; the agent usually needs to add fields before deployment

## Upload and ignore behavior

Jig uploads the project directory as a tarball. The built-in ignore defaults are:

- `.git/**`
- `.gitignore`
- `.jig/**`
- `node_modules/**`

Important caveat:

- `.env` files are not ignored by default

Agent guidance:

- prefer Jig secrets over uploading local secret files
- add entries such as `.env*` to `.jigignore` unless the user explicitly wants them uploaded
- `.jigignore` supports glob patterns and `!` negation

## Mode selection rules

Use this matrix when deciding what to tell the user.

### Standalone single-container

Use this when there is no compose file and the server is not Swarm-backed.

Relevant `jig.json` fields:

- `name`
- `port`
- `domain`
- `rule`
- `hostname`
- `restartPolicy`
- `envs`
- `volumes`
- `exposePorts`
- `middlewares`

### Standalone compose

Use this when the project has a compose file and the server is not Swarm-backed.

Recommended pattern:

- top-level `jig.json` with `name`, `composeFile`, and any truly shared defaults
- `x-jig` blocks inside routed services

Services without `x-jig` still run, but Jig does not fully manage them as first-class deployments.

### Swarm single-service

Use this when there is no compose file and Jig is backed by Swarm.

Relevant `jig.json` fields are similar to standalone single-container, with one extra rule:

- bind mounts require `placement.requiredNodeLabels`

### Swarm stack

Use this when the project has a compose file and Jig is backed by Swarm.

Recommended pattern:

- top-level `jig.json` with `name`, `composeFile`, and shared defaults
- `x-jig` blocks on routed services
- compose file owns published ports and volumes

## Config rules the agent must follow

### Routing fields

Use these semantics:

- `rule` overrides `domain`
- `domain` is the simple convenience form for `Host(...)`
- `hostname` is an internal container or network alias, not the public route
- `port` is the backend container port that Traefik should send HTTP traffic to

Important caveat:

- if a service should be internal-only, set `middlewares.noHTTP: true`
- do not assume that omitting `domain` makes a service internal-only

If the user wants plain HTTP without HTTPS, use:

```json
{
  "middlewares": {
    "noTLS": true
  }
}
```

If the service should not be exposed through Traefik at all, use:

```json
{
  "middlewares": {
    "noHTTP": true
  }
}
```

### `exposePorts`

`exposePorts` publishes host ports. It is supported only for:

- standalone single-container deployments
- singular swarm service deployments

Example:

```json
{
  "exposePorts": {
    "5432": "5432",
    "53/udp": "53"
  }
}
```

Important caveat:

- compose deployments must publish ports in `docker-compose.yml`, not in `jig.json` or `x-jig`

### Volumes

Use `volumes` in `jig.json` only for non-compose deployments.

Format:

```json
{
  "volumes": [
    "/srv/app-data:/app/data"
  ]
}
```

Important caveats:

- compose deployments must define volumes in the compose file
- swarm deployments with bind mounts require `placement.requiredNodeLabels`
- the labels in `placement.requiredNodeLabels` must have non-empty keys and values

### Compose config rules

For compose projects, prefer `x-jig` on each routed service.

Per-service `x-jig` is the right place for:

- `name`
- `domain` or `rule`
- `port`
- `hostname`
- `envs`
- `middlewares`
- `restartPolicy`

Per-service `x-jig` must not set:

- `composeFile`
- `composeService`
- `volumes`
- `exposePorts`

Top-level compose `jig.json` should usually hold:

- `name`
- `composeFile`
- shared `envs`
- shared `restartPolicy`

Important inheritance caveats for `x-jig` services:

- top-level `envs` and `restartPolicy` are inherited
- top-level `domain`, `rule`, `hostname`, and `middlewares` also behave like defaults
- top-level `port`, `exposePorts`, `volumes`, `composeFile`, and `composeService` are not per-service defaults

Agent guidance:

- for multi-service compose, prefer putting route-specific fields inside each `x-jig` block
- avoid shared top-level `domain` or `rule` unless every routed service should actually share it
- keep `x-jig.name` values unique within a stack

### Legacy compose fallback

Jig still supports compose projects with no `x-jig` blocks by choosing one primary service.

How it selects the primary service:

1. `composeService` if set
2. a compose service whose name matches the top-level deployment `name`
3. otherwise the first service returned by `docker compose config --services`

Agent guidance:

- do not rely on the fallback order
- if using legacy compose, set `composeService` explicitly
- if you need explicit per-service routing behavior, prefer `x-jig` instead

Important caveat:

- legacy compose rejects top-level `port`, `volumes`, and `exposePorts`

## Deployment commands

Default deploy:

```bash
jig deploy
```

Useful variants:

```bash
jig deploy -v
jig deploy -c ./jig.json
jig deploy -l
```

Command guidance:

- prefer plain `jig deploy` unless the user specifically wants a local image build
- `jig deploy -l` works for single-container deployments, including swarm-backed single-service deployments
- `jig deploy -l` is not supported for compose deployments
- compose deployments require `docker compose` to exist on the server

## Command support matrix

Use these rules when suggesting operational commands.

### `jig ls`

Works in all modes.

Interpretation caveats:

- compose deployments appear as a stack parent plus child services
- services without `x-jig` do not show up as first-class Jig deployments

### `jig logs`

Supported forms:

```bash
jig logs <name>
jig logs <stack>
jig logs <stack:service>
```

Behavior caveats:

- `jig logs <stack>` only shows Jig-managed services in that stack
- internal compose-only services without `x-jig` are not exposed as separate Jig log targets

### `jig deployments rm`

Supported forms depend on mode.

Valid and useful:

- `jig deployments rm <name>` for singular deployments
- `jig deployments rm <stack>` for swarm stacks
- `jig deployments rm <stack:service>` for standalone compose managed services

Important caveats:

- removing an individual service from a swarm stack is not supported
- on swarm, removing a stack uses `docker stack rm` and removes the whole stack
- on standalone compose, removing `<stack>` or `<stack:service>` only removes Jig-managed labeled containers, not every compose-only service in the project

### `jig deployments rollback`

Rollback support is mode-specific.

Supported:

- standalone single-container deployments, if a previous container exists
- singular swarm service deployments, if Swarm has a `PreviousSpec`
- individual swarm stack services via `stack:service`, if that service has a `PreviousSpec`

Not supported:

- standalone compose deployments
- whole swarm stacks

Agent guidance:

- do not say rollback is a universal deployment feature
- on swarm stacks, rollback is per service, not per stack

### `jig deployments scale`

Scaling is only supported for singular swarm service deployments.

Important caveats:

- not supported on standalone instances
- not supported for compose stack services
- not supported for whole stacks
- not supported when the deployment uses `placement.requiredNodeLabels`
- replicas must be at least `1`

### `jig deployments stats`

Behavior differs by backend:

- standalone: per-deployment CPU and memory stats
- swarm-backed: node-level cluster summary, not per-deployment container stats

## Secrets and tokens

Secrets:

```bash
jig secrets ls
jig secrets add <name> <value>
jig secrets inspect <name>
jig secrets rm <name>
```

Use them from config like this:

```json
{
  "envs": {
    "DATABASE_URL": "@database-url"
  }
}
```

Operational guidance:

- prefer secrets over uploading `.env` files
- `jig secrets add` returns a friendly warning rather than failing hard when the secret already exists

Tokens:

```bash
jig tokens ls
jig tokens create <name>
jig tokens delete <name>
```

Operational guidance:

- after `jig tokens create`, store the returned `<endpoint>+<token>` immediately
- create tokens only after normal login has selected a server

## Recommended troubleshooting order

When something fails, investigate in this order:

1. `jig servers ls`
2. `jig ls`
3. `jig logs <name>` or `jig logs <stack:service>`
4. verify `jig.json` exists and has a non-empty `name`
5. if compose is involved, verify the actual compose filename and whether the server has `docker compose`
6. verify secrets referenced as `@name` exist on the server
7. if Swarm bind mounts are used, verify node labels match `placement.requiredNodeLabels`
8. if published ports are involved, verify they were configured in the right layer

Common pitfalls to catch early:

- wrong login token format
- client not on `PATH`
- `jig deploy -l` attempted for compose
- `.env` accidentally uploaded because `.jigignore` did not exclude it
- top-level compose `port` used where per-service `x-jig.port` or compose `ports:` was required
- `exposePorts` placed in compose `jig.json` or `x-jig`
- trying to remove a single service from a swarm stack
- expecting rollback for standalone compose or whole swarm stacks
- expecting `jig logs <stack>` to include internal compose-only services
- exposed registry port `5000/tcp` on Swarm nodes

## Response style for this skill

Prefer short operator instructions over long explanations.

A good response usually contains:

- the mode you are assuming
- the exact command sequence
- one or two checks to confirm success
- only the caveats that materially affect this specific task

Do not dump the entire rulebook to the user unless they ask for a deep operational explanation.
