import express from "express";
import requireAuth from "../middlewares/requireAuth";
import z from "zod";
import bodyParser from "body-parser";
import { secrets } from "../dbs";
import { randomUUID } from "node:crypto";
import { createWriteStream } from "node:fs";
import { Readable } from "node:stream";
import tar, { extract } from "tar";
import { docker, stopContainerIfExists } from "../docker";

const runningDeployments = new Map<string, string>();

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

  try {
    const buildStream = await docker.buildImage(req, {
      t: config.name + ":latest",
      buildargs: config.buildEnv,
      dockerfile: "./Dockerfile",
      rm: true,
    });

    res.json(config.name);

    await new Promise((res, rej) => {
      docker.modem.followProgress(
        buildStream,
        (err, done) => (err ? rej(err) : res(done)),
        (progress) => {
          console.log("PROGRESS:", progress);
        }
      );
    });

    const restartTimes = config.restartPolicy?.match(/(:\d{1,3})/)?.[0];

    await stopContainerIfExists((c) => c.Labels["jig.name"] === config.name);
    console.log("Deleted old container!!!");

    const traiefikRouterName = `traefik.http.routers.${config.name}`;

    let rule: string;
    switch (true) {
      case config.rule != undefined:
        rule = config.rule as string;
        break;

      case !!config.domain:
        rule = `Host(\`${config.domain}\`)`;
        break;

      default:
        rule = "No-HTTP";
        break;
    }

    const container = await docker.createContainer({
      Image: config.name,
      ExposedPorts: { [config.port.toString() + `/tcp`]: {} },
      Labels: {
        [`${traiefikRouterName}.rule`]: rule,
        [`${traiefikRouterName}.tls`]: "true",
        [`${traiefikRouterName}.tls.certresolver`]: "defaultResolver",
        "traefik.enable": "true",
        "jig.name": config.name,
      },
      Env: Object.entries(config.env).map(([key, val]) => key + "=" + val),
      name: config.name,

      HostConfig: config.restartPolicy
        ? {
            RestartPolicy: {
              Name: config.restartPolicy,
              MaximumRetryCount: restartTimes
                ? Number.parseInt(restartTimes.replace(":", ""))
                : undefined,
            },
          }
        : undefined,
    });

    await container.start({});
    console.log("Container started!!!");
  } catch (error) {
    console.log("Error: ", error);
  }
});

deploymentsRouter.get("/", async (req, res) => {
  // res.json(Array.from(stripValues(secrets.values())));
  const containers = await docker.listContainers();
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

export default deploymentsRouter;
