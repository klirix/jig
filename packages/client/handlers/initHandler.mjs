import inquirer from "inquirer";
import { writeFileSync } from "fs";

export const initHandler = async () => {
  const cwd = process.cwd();
  const currentDir = cwd.substring(cwd.lastIndexOf("/") + 1);
  const {
    addEnvs,
    availableVia: _,
    ...configParts
  } = await inquirer.prompt([
    {
      type: "input",
      name: "name",
      message: "Deployment name",
      default: currentDir,
    },
    {
      type: "number",
      name: "port",
      message: "Port to listen to",
      default: 3000,
    },
    {
      type: "list",
      message: "Restart policy",
      choices: ["no", "always", "unless-stopped", "on-failure:2"],
      default: "unless-stopped",
      name: "restartPolicy",
    },
    {
      type: "list",
      message: "Make deployment available via",
      choices: ["Nothing", "Domain", "Traefik rule"],
      default: "unless-stopped",
      name: "availableVia",
    },
    {
      type: "input",
      message: "Domain to use:",
      name: "domain",
      when: (answers) => answers.availableVia === "Domain",
    },
    {
      type: "input",
      message: "Traefik rule to use:",
      name: "rule",
      when: (answers) => answers.availableVia === "Traefik rule",
    },
    {
      type: "confirm",
      message: "Add environment variables",
      name: "addEnvs",
      default: false,
    },
  ]);
  let addMore = false;
  const envs = {};
  if (addEnvs || addMore) {
    const env = await inquirer.prompt([
      {
        type: "input",
        message: "Environment variable",
        name: "name",
      },
      {
        type: "input",
        message: "Value",
        name: "value",
      },
    ]);
    envs[env.name] = env.value;
    addMore = (
      await inquirer.prompt({
        type: "confirm",
        message: "Add more",
        name: "addMore",
      })
    ).addMore;
  }
  writeFileSync("jig.json", JSON.stringify({ ...configParts, envs }, null, 2));
};
