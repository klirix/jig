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
    Cmd: [
      "--api.insecure=true",
      "--entrypoints.web.address=:80",
      "--entrypoints.websecure.address=:443",
      "--providers.docker=true",
      "--providers.docker.exposedbydefault=false",
      "--certificatesresolvers.defaultresolver=true",
      "--certificatesresolvers.defaultresolver.acme.httpchallenge=true",
      "--certificatesresolvers.defaultresolver.acme.httpchallenge.entrypoint=web",
      "--certificatesresolvers.defaultresolver.acme.email=" + env.JIG_SSL_EMAIL,
    ],
    Labels: {
      "--traefik.http.middlewares.https-only.redirectscheme.scheme": "https",
    },
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
