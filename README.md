# Jig — Dead simple deployment tool

noun

&nbsp; _a device that holds a piece of work and guides the tool operating on it._

## Installation

Installation is in two steps:

1. Server setup: Run a startup script on the server to pull all relevant images and take off
2. Client setup: Download a client and get authentication ready

### Server setup

Docker is a prerequisite, [so ensure docker is available and running on your server](https://docs.docker.com/engine/install/)

```bash
wget -q https://getjig.askh.at/install.sh && bash install.sh && rm install.sh
```

This will load traefik, jig, ask you for an email, jwt signing key, launch everything and spit out a command to run on your machine to login

Login command will look something like `jig login loooooong+code` keep it for later

### Client setup

Any node package manager is a prerequisite

```bash
npm install -g jig-client
```

After that plug in the `jig login ...` command you received server side and you should be ready to go

### TODOs

- [] Manage multiple servers 🌿
- [] Check docker container resource consumption 💸
- [] Fetch container logs (forgor to implement 💀), maybe attahcing to the container even 🤔
- [] DNS management (? maybe)
