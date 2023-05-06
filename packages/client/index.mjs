import Conf from "conf";
import yargs from "yargs/yargs";
import { hideBin } from "yargs/helpers";
import axios from "axios";
import chalk from "chalk";

const NOOP = () => {};

const config = new Conf({ projectName: "jig" });

const httpClient = axios.create({
  baseURL: config.get("endpoint"),
  // timeout: 1000,
  headers: { Authorization: "Bearer " + config.get("token") },
});

const isLoggedIn = () => {
  if (!config.get("token")) {
    console.log("You need to login, use token command!");
    return false;
  }
  if (!config.get("endpoint")) {
    console.log("Jig endpoint needs to be set, try endpoint command");
    return false;
  }
  return true;
};

yargs(hideBin(process.argv))
  .command(
    "endpoint [endpoint]",
    "Set endpoint",
    (y) => {},
    ({ endpoint }) => {
      if (endpoint) {
        config.set("endpoint", endpoint);
      } else {
        console.log(`Current enpoint is: ${config.get("endpoint")}`);
      }
    }
  )
  .command(
    "token <token>",
    "Set token",
    (y) => {},
    ({ token }) => {
      config.set("token", token);
      console.log("Token successfully set!");
    }
  )
  .command("ls", "List deployments", NOOP, async () => {
    if (!isLoggedIn()) return;

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
  })
  .command("rm <name>", "Delete deployment", NOOP, async ({ name }) => {
    if (!isLoggedIn()) return;

    try {
      await httpClient.delete("/deployments/" + name);

      console.log(chalk.green`> Successfully removed: ` + name);
    } catch (error) {
      console.log(chalk.red`> Failed to remove: ` + name);
    }
  })
  .help().argv;
