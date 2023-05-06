import { makeHttpClient } from "../httpClient.mjs";
import chalk from "chalk";

export const listDeploymentHandler = async ({ ...auth }) => {
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
    if (error instanceof Error) console.log(chalk.red(`> ${error.message}`));
  }
};

export const deleteDeploymentHandler = async ({ name, ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);
    await httpClient.delete("/deployments/" + name);

    console.log(chalk.green`> Successfully removed: ` + name);
  } catch (error) {
    console.log(chalk.red`> Failed to remove: ` + name);
    if (error instanceof Error) console.log(chalk.red(`> ${error.message}`));
  }
};
