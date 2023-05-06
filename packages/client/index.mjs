import yargs from "yargs/yargs";
import { hideBin } from "yargs/helpers";
import chalk from "chalk";
import { readFileSync, existsSync, createWriteStream, cpSync } from "fs";
import { config, makeHttpClient } from "./httpClient.mjs";
import tar from "tar";
import { glob } from "glob";
import { Axios } from "axios";
import { AxiosError } from "axios";

const NOOP = () => {};

const setTokenHandler = ({ token }) => {
  config.set("token", token);
  console.log("Token successfully set!");
};
const getSetEndpointHandler = ({ endpoint }) => {
  if (endpoint) {
    config.set("endpoint", endpoint);
  } else {
    console.log(`Current enpoint is: ${config.get("endpoint")}`);
  }
};
const deleteDeploymentHandler = async ({ name, ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);
    await httpClient.delete("/deployments/" + name);

    console.log(chalk.green`> Successfully removed: ` + name);
  } catch (error) {
    console.log(chalk.red`> Failed to remove: ` + name);
    if (error instanceof Error) console.log(chalk.red`> ${error.message}`);
  }
};

const listDeploymentHandler = async ({ ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);
    const { data: deployments } = await httpClient.get("/deployments");
    const tableLengths = deployments.reduce(
      (acc, d) => ({
        name: Math.max(d.name.length, acc.name),
        rule: Math.max(d.rule.length, acc.rule),
      }),
      {
        name: 0,
        rule: 0,
      }
    );
    console.log("Current deployments:\n");
    console.log(
      `  ${chalk.grey("name".padEnd(tableLengths.name))}  ${chalk.grey(
        "rule".padEnd(tableLengths.rule)
      )}  ${chalk.grey("status")} `
    );
    for (const deployment of deployments) {
      const [name, rule, status] = [
        deployment.name.padEnd(tableLengths.name),
        deployment.rule.padEnd(tableLengths.rule),
        deployment.status,
      ];
      console.log(`  ${name}  ${rule}  ${status}`);
    }
  } catch (error) {
    console.log(chalk.red`> Failed to fetch deployments`);
    if (error instanceof Error) console.log(chalk.red`> ${error.message}`);
  }
};

yargs(hideBin(process.argv))
  .command("endpoint [endpoint]", "Set endpoint", NOOP, getSetEndpointHandler)
  .command("token <token>", "Set token", (y) => {}, setTokenHandler)
  .command(
    "ls",
    "List deployments",
    (y) => {
      y.option("endpoint", {
        alias: "e",
        string: true,
        description:
          "override endpoint, default is in config or env variable $JIG_ENDPOINT",
      });
      y.option("token", {
        alias: "t",
        string: true,
        description: "override token, default in config or $JIG_TOKEN",
      });
    },
    listDeploymentHandler
  )
  .command("rm <name>", "Delete deployment", NOOP, deleteDeploymentHandler)
  .command(
    [`$0`, "deploy"],
    "Create deployment",
    (y) => {
      y.option("config", {
        alias: "c",
        string: true,
        description: "deployment config file",
        default: "./jig.json",
      });
      y.option("endpoint", {
        alias: "e",
        string: true,
        description:
          "override endpoint, default is in config or env variable $JIG_ENDPOINT",
      });
      y.option("token", {
        alias: "t",
        string: true,
        description: "override token, default in config or $JIG_TOKEN",
      });
      y.option("ignore", {
        alias: "i",
        string: true,
        description: "ignore file",
      });
    },
    async ({ config, ...authOptions }) => {
      try {
        const httpClient = makeHttpClient(authOptions);
        const configPath = config;
        if (!existsSync(configPath)) {
          console.log(chalk.red`> Cannot find config at ` + configPath);
          return;
        }
        const configFile = readFileSync(configPath).toString("utf-8");

        let ignorePaths = ["node_modules/**", "."];
        if (existsSync(".jigignore")) {
          ignorePaths = readFileSync(".jigignore")
            .toString("utf-8")
            .split("\n")
            .concat(".");
        }

        const files = await glob("**", { ignore: ignorePaths });

        console.log("> DEBUG: Files: ", files);

        const tarStream = tar.create(
          {
            cwd: ".",
          },
          files
        );

        await uploadBuild(httpClient, tarStream, configFile, (progress) => {
          console.log(`> ` + progress.stream);
        });

        console.log(chalk.green`> Successfully deployed: ` + config.name);
      } catch (error) {
        console.log(chalk.red`> Failed to deploy`);
      }
    }
  )
  .help().argv;

const uploadBuild = (httpClient, data, config, onProgress) =>
  new Promise(async (res, rej) => {
    try {
      const response = await httpClient({
        method: "post",
        headers: {
          Accept: "application/x-ndjson",
          "x-jig-config": JSON.stringify(JSON.parse(config)),
        },
        url: "/deployments/",
        responseType: "stream",
        data,
      });
      const stream = response.data;
      console.log(`Build process:`);

      stream.on("data", (data) => {
        const decodedChunk = JSON.parse(data);
        if (decodedChunk.error) {
          console.log(
            chalk.red`> Error during deployemnt: `,
            decodedChunk.error
          );
          rej(new Error(decodedChunk.error));
        } else {
          onProgress(decodedChunk);
        }
      });

      stream.on("end", res);
    } catch (err) {
      if (err instanceof AxiosError) {
        err.response.data.on("data", (err) => {
          console.log(chalk.red(`> ` + JSON.parse(err.toString()).error));
        });
        err.response.data.on("end", () => {
          rej(err);
        });
      } else {
        rej(err);
      }
    }
  });
