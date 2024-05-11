import express from "express";
import requireAuth from "../middlewares/requireAuth";
import z, { ZodError } from "zod";
import { secrets } from "../dbs";
import { docker, stopContainerIfExists } from "../docker";
import type { HostRestartPolicy } from "dockerode";

const deploymentsRouter = express.Router();

deploymentsRouter.use(requireAuth);

const envRecordSchema = z
  .record(
    z.string(),
    z.string().transform((val, ctx) => {
      if (val.startsWith("@")) {
        const secret = secrets.get(val.slice(1));
        if (!secret)
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: "Secret doesnt exist",
          });
        return secret?.value || "";
      }
      return val;
    })
  )
  .default({});

const jigDeployConfigSchema = z.object({
  name: z.string().regex(/^[a-z0-9-]+$/),
  env: envRecordSchema,
  buildEnv: envRecordSchema,
  port: z.number().min(0).max(100000).default(80),
  domain: z.string().optional(),
  rule: z.string().optional(),
  restartPolicy: z
    .enum(["no", "always", "unless-stopped"])
    .or(z.string().regex(/on-failure(:\d{1,3})?/))
    .optional(),
});

deploymentsRouter.post("/", async (req, res) => {
  try {
    const configString = req.headers["x-jig-config"];
    if (!configString || typeof configString !== "string")
      return res.status(400).json("Config is required!");

    const config = jigDeployConfigSchema.parse(JSON.parse(configString));

    try {
      await docker.getImage(config.name + ":prev").remove();
    } catch (error) {
      console.log(error);
      // Ignore if it's not there
    }
    try {
      await docker
        .getImage(config.name + ":latest")
        .tag({ repo: config.name, tag: "prev" });
    } catch (error) {
      console.log(error);
      // Ignore if it's not there
    }
    res.header("Content-Type", "application/x-ndjson");

    const progress =
      req.headers["x-jig-image"] == "true"
        ? await docker.loadImage(req, { quiet: true })
        : await docker.buildImage(req, {
            t: config.name + ":latest",
            buildargs: config.buildEnv,
            rm: true,
          });

    await new Promise((resolve, rej) => {
      docker.modem.followProgress(
        progress,
        (err, done) => {
          console.log(err, done);
          if (done.find((x) => x.error))
            rej(new Error(done.find((x) => x.error).error));
          return err ? rej(err) : resolve(done);
        },
        (progress) => {
          if (progress.error) return rej(new Error(progress.error));
          res.write(JSON.stringify(progress) + "\n");
        }
      );
    });

    await stopContainerIfExists(
      (c) => c.Labels["jig.name"] === config.name,
      true
    );
    res.write(JSON.stringify({ stream: "Stopped old container" }) + "\n");
    console.log("Deleted old container!!!");

    const traiefikRouterName = `traefik.http.routers.${config.name}`;

    let rule: string | undefined = undefined;
    switch (true) {
      case config.rule != undefined:
        rule = config.rule as string;
        break;

      case !!config.domain:
        rule = `Host(\`${config.domain}\`)`;
        break;
    }

    let restartPolicy: HostRestartPolicy | undefined = undefined;
    if (config.restartPolicy) {
      if (config.restartPolicy.includes(":")) {
        const [name, num] = config.restartPolicy.split(":");
        restartPolicy = {
          Name: name,
          MaximumRetryCount: Number.parseInt(num),
        };
      } else {
        restartPolicy = {
          Name: config.restartPolicy,
        };
      }
    }

    const container = await docker.createContainer({
      Image: config.name,
      ExposedPorts: { [config.port.toString() + `/tcp`]: {} },
      Labels: {
        ...(rule
          ? {
              [`${traiefikRouterName}.rule`]: rule,
              [`${traiefikRouterName}.middlewares`]: "https-only",
              [`${traiefikRouterName}.entrypoints`]: "web",
              [`${traiefikRouterName}-secure.rule`]: rule,
              [`${traiefikRouterName}-secure.tls.certresolver`]:
                "defaultresolver",
              [`${traiefikRouterName}-secure.tls`]: "true",
              [`${traiefikRouterName}-secure.entrypoints`]: "websecure",
              "traefik.enable": "true",
            }
          : {
              "jig.nohttp": "true",
            }),
        "jig.name": config.name,
      },
      Env: Object.entries(config.env).map(([key, val]) => key + "=" + val),
      name: config.name,
      HostConfig: restartPolicy ? { RestartPolicy: restartPolicy } : undefined,
    });

    await container.start({});
    res.write(JSON.stringify({ stream: "Created new container" }));
    console.log("Container started!!!");
    res.end();
  } catch (error) {
    if (error instanceof ZodError) {
      res.status(400).write(
        JSON.stringify({
          error:
            `Errors in config: \n ` +
            Object.entries(error.flatten().fieldErrors)
              .map(([field, err]) => `${field}: ${err}`)
              .join(" \n "),
        })
      );
      return void res.end();
    }
    if (error instanceof Error) {
      res.status(500).write(JSON.stringify({ error: error.message }));
      return void res.end();
    }
    console.log("Error: ", error);
  }
});

deploymentsRouter.get("/", async (req, res) => {
  // res.json(Array.from(stripValues(secrets.values())));
  const containers = await docker.listContainers({ all: true });
  const deployments = containers
    .filter((c) => !!c.Labels["jig.name"])
    .map((c) => ({
      name: c.Labels["jig.name"],
      rule: c.Labels[`traefik.http.routers.${c.Labels["jig.name"]}.rule`],
      status: c.Status,
    }));

  res.json(deployments);
});

deploymentsRouter.delete("/:name", async (req, res) => {
  const { name } = req.params;

  await stopContainerIfExists((c) => c.Labels["jig.name"] === name);

  res.json(name);
});

deploymentsRouter.get("/:name/logs", async (req, res) => {
  const { name } = req.params;

  const containerInfo = (await docker.listContainers({ all: true })).find(
    (x) => x.Labels["jig.name"] == name
  );
  if (!containerInfo) return void res.sendStatus(404);
  const containerLogBuffer = await docker
    .getContainer(containerInfo.Id)
    .logs({ follow: false, stdout: true });
  const logs = containerLogBuffer
    .toString("utf-8")
    .split(`\n`)
    .map((stream) => ({ stream }));
  res.json(logs);
});

export default deploymentsRouter;
