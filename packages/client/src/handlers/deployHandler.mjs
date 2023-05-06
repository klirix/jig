import { makeHttpClient } from "../httpClient.mjs";
import { existsSync, readFileSync } from "fs";
import { glob } from "glob";
import tar from "tar";
import chalk from "chalk";
import { isAxiosError } from "axios";
import Dockerode from "dockerode";
import progressBar from "cli-progress";

/**
 *
 * @param {Number} bytes number in bytes
 * @returns amount in MB
 */
const bytesToMb = (bytes) => Math.round(bytes / (1024 * 1024));

export const deployHandler = async ({ config, locally, ...authOptions }) => {
  try {
    const httpClient = makeHttpClient(authOptions);
    const configPath = config;
    if (!existsSync(configPath)) {
      console.log(chalk.red`> Cannot find config at ` + configPath);
      return;
    }
    const configFile = readFileSync(configPath).toString("utf-8");
    const jigConfig = JSON.parse(configFile);

    let ignorePaths = ["node_modules/**", "."];
    if (existsSync(".jigignore")) {
      ignorePaths = readFileSync(".jigignore")
        .toString("utf-8")
        .split("\n")
        .concat(".");
    }

    const files = await glob("**", { ignore: ignorePaths });

    console.log("> DEBUG: Files: ", files);

    const tarStream = tar.create({ cwd: "." }, files);

    if (locally) {
      const docker = new Dockerode();
      const progressStream = await docker.buildImage(tarStream, {
        rm: true,
        t: `${jigConfig.name}:latest`,
      });

      await new Promise((resolve, reject) => {
        docker.modem.followProgress(
          progressStream,
          (err, result) => {
            if (!err) resolve(result);
          },
          (progress) => {
            if (progress.error) {
              return reject(progress.error);
            }
            if (progress.stream) {
              console.log(`> ` + progress.stream.replace("\n", ""));
            }
          }
        );
      });

      const image = docker.getImage(`${jigConfig.name}:latest`);
      const uploadable = await image.get();
      const inspect = await image.inspect();
      const bar = new progressBar.SingleBar(
        { format: " {bar} | ETA: {eta}s | {value}MB /{total}MB" },
        progressBar.Presets.shades_classic
      );
      bar.start(bytesToMb(inspect.Size), 0);
      const { data: responseStream } = await httpClient({
        method: "post",
        headers: {
          Accept: "application/x-ndjson",
          "x-jig-config": JSON.stringify(jigConfig),
          "x-jig-image": "true",
        },
        url: "/deployments/",
        responseType: "stream",
        data: uploadable,
        onUploadProgress(e) {
          bar.update(bytesToMb(e.loaded));
        },
      });

      bar.stop();

      new Promise((resolve, reject) => {
        responseStream.on("data", (data) => {
          const decodedChunk = JSON.parse(data);
          if (decodedChunk.error) {
            console.log(
              chalk.red`> Error during deployemnt: `,
              decodedChunk.error
            );
            reject(new Error(decodedChunk.error));
          } else {
            console.log(`> ` + decodedChunk.stream.replace("\n", ""));
          }
        });

        responseStream.on("end", resolve);
      });
    } else {
      await uploadBuild(httpClient, tarStream, configFile, (progress) => {
        console.log(`> ` + progress.stream.replace("\n", ""));
      });
    }

    console.log(
      chalk.green`> Successfully deployed: ` + JSON.parse(configFile).name
    );
  } catch (error) {
    console.log(error);
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
