import Dockerode from "dockerode";
import { docker, stopContainerIfExists } from "../docker";
import env from "../env";

export async function initTraefik() {
  const images = await docker.listImages();
  if (!images.some((x) => x.RepoTags?.some((x) => x.includes("traefik")))) {
    await docker.pull("traefik:latest");
  }

  await stopContainerIfExists((x) => x.Names.includes("/traefik"));

  const container = await docker.createContainer({
    Image: "traefik:latest",
    name: `traefik`,
    ExposedPorts: {
      "80/tcp": {},
      "8080/tcp": {},
      "443/tcp": {},
    },
    Env: [
      "TRAEFIK_API_INSECURE=true",
      "TRAEFIK_ENTRYPOINTS_web_ADDRESS=:80",
      "TRAEFIK_ENTRYPOINTS_websecure_ADDRESS=:443",
      "TRAEFIK_PROVIDERS_DOCKER=true",
      "TRAEFIK_PROVIDERS_DOCKER_EXPOSEDBYDEFAULT=false",
      "TRAEFIK_CERTIFICATESRESOLVERS_defaultResolver_ACME_HTTPCHALLENGE_ENTRYPOINT=web",
      "TRAEFIK_CERTIFICATESRESOLVERS_defaultResolver_ACME_EMAIL=" +
        env.JIG_SSL_EMAIL,
    ],
    HostConfig: {
      RestartPolicy: {
        Name: "unless-stopped",
      },
      Binds: ["/var/run/docker.sock:/var/run/docker.sock"],
      PortBindings: {
        "80/tcp": [{ HostPort: "80" }],
        "8080/tcp": [{ HostPort: "8080" }],
        "443/tcp": [{ HostPort: "443" }],
      },
    },
  });

  await container.start();
}
