import { makeHttpClient } from "../httpClient.mjs";
import { existsSync, readFileSync } from "fs";
import { glob } from "glob";
import tar from "tar";
import chalk from "chalk";
import { isAxiosError } from "axios";

export const deployHandler = async ({ config, ...authOptions }) => {
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
};

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
      if (isAxiosError(err)) {
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
