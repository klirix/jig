import yargs from "yargs/yargs";
import { hideBin } from "yargs/helpers";
import chalk from "chalk";
import { config } from "./httpClient.mjs";
import {
  listSecrets,
  initHandler,
  deployHandler,
  listDeploymentHandler,
  addSecretHandler,
  removeSecretHanlder,
  deleteDeploymentHandler,
} from "./handlers/index.mjs";

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

const defaultOptionBuilder = (y) => {
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
};
yargs(hideBin(process.argv))
  .command("init", "Initialize project", NOOP, initHandler)
  .command(
    "login <loginString>",
    "Use this command for easy login",
    NOOP,
    ({ loginString }) => {
      const [endpoint, token] = loginString.split("+");
      if (endpoint && token) {
        config.set("token", token);
        config.set("endpoint", `https://` + endpoint);
        console.log(`${chalk.green`> Successfully logged in, jig on! `}`);
      } else {
        console.log(`${chalk.red`> Error: `}That's a weird login string`);
      }
    }
  )
  .command(
    "endpoint [endpoint]",
    "Set or get endpoint",
    (y) =>
      y.positional("endpoint", {
        description: "if not set logs current endpoint",
      }),
    getSetEndpointHandler
  )
  .command("token <token>", "Set token", NOOP, setTokenHandler)
  .command(
    "ls",
    "List deployments",
    (y) => y.positional("token", { description: "Token to set" }),
    listDeploymentHandler
  )
  .command(
    "rm <name>",
    "Delete deployment",
    defaultOptionBuilder,
    deleteDeploymentHandler
  )
  .command(
    [`$0`, "deploy"],
    "Create deployment",
    (y) => {
      defaultOptionBuilder(y);
      y.option("config", {
        alias: "c",
        string: true,
        description: "deployment config file",
        default: "./jig.json",
      });
      y.option("ignore", {
        alias: "i",
        string: true,
        description: "ignore file",
      });
    },
    deployHandler
  )
  .command(
    `secrets-ls`,
    "List available secrets",
    defaultOptionBuilder,
    listSecrets
  )
  .command(
    `secrets-rm <key>`,
    "Remove secret",
    (y) => {
      defaultOptionBuilder(y);
      y.positional("key", {
        description: "Secret reference key",
      });
    },
    removeSecretHanlder
  )
  .command(
    `secrets-add <key> <value>`,
    "Add secret value",
    (y) => {
      defaultOptionBuilder(y);
      y.positional("key", {
        description: "Secret reference key",
      });
      y.positional("value", {
        description: "Secret value",
      });
    },
    addSecretHandler
  )
  .help().argv;
