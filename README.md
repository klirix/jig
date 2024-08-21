# Jig — Dead simple deployment tool

noun

&nbsp; _a device that holds a piece of work and guides the tool operating on it._

## !!!Warning!!!

The package is both experimental AND unstable

New features likely will break existing setups unless both client and server are not in sync

## Installation

Installation is done in two steps:

1. Server setup: Run a startup script on the server to pull all relevant images and take off
2. Client setup: Download a client and get authentication ready

### Server setup

Docker is a prerequisite, [so ensure docker is available and running on your server](https://docs.docker.com/engine/install/)

```bash
curl -fsSL https://deploywithjig.askh.at/init.sh | bash
```

This will load traefik, jig, ask you for an email, jwt signing key, launch everything and spit out a command to run on your machine to login

Login command will look something like `jig login loooooong+code` keep it for later

### Client setup

Any node package manager is a prerequisite

```bash
curl -fsSL https://deploywithjig.askh.at/install.sh | bash
```

Plug in your login command you got in previous steps with

```bash
jig login <loooooong+code>
```

Then initiate a project and create a config in your project directory with

```bash
jig init
```

Deploy your project with a single command. Jig will pack the project, send it to the server and build it remotely

```bash
jig deploy
```

Or build it locally using docker and deploy the image to the server. This is useful for CI and to save resources on the server

```bash
jig deploy -l
```

Let Traefik fetch certificates if you deploy with TLS enabled and you're done

### TODOs

- [x] Manage multiple servers 🌿
- [x] Check docker container resource consumption 💸
- [x] Fetch container logs (forgor to implement 💀), maybe attahcing to the container even 🤔
- [x] Deployment rollbacks!
- [ ] DNS management (? maybe)
- [x] Improve CLI outputs
- [ ] Complex deployments
