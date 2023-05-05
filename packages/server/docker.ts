import Dockerode from "dockerode";

export const docker = new Dockerode();

export async function stopContainerIfExists(
  predicate: (info: Dockerode.ContainerInfo) => boolean,
  force: boolean = false
) {
  const containers = await docker.listContainers({});
  const traefikContainerInfo = containers.find(predicate);
  if (traefikContainerInfo) {
    const container = docker.getContainer(traefikContainerInfo.Id);
    if (traefikContainerInfo.State == "running") {
      if (force) {
        await container.kill();
      } else {
        await container.stop();
      }
    }
    await container.remove();
  }
}
