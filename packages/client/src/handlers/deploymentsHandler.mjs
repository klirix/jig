import { makeHttpClient } from "../httpClient.mjs";
import chalk from "chalk";

/**
 *
 * @param {string[]} fields
 * @returns {(list: Array) => Record<string, number>} field max lengths
 */
const maxLengthForFields = (fields) => {
  return (objList) => {
    const result = {};
    for (const obj of objList) {
      for (const field of fields) {
        if (obj[field].length > (result[field] || 0))
          result[field] = obj[field].length;
      }
    }
    return result;
  };
};

const maxLengthForDeploymentList = maxLengthForFields([
  "name",
  "rule",
  "status",
]);

export const listDeploymentHandler = async ({ ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);
    const { data: deployments } = await httpClient.get("/deployments");
    const lens = maxLengthForDeploymentList(deployments);
    console.log("> Current deployments:\n");
    console.log(
      `  ${chalk.grey("name".padEnd(lens.name))}  ${chalk.grey(
        "rule".padEnd(lens.rule)
      )}  ${chalk.grey("status")} `
    );
    for (const deployment of deployments) {
      const [name, rule, status] = [
        deployment.name.padEnd(lens.name),
        deployment.rule.padEnd(lens.rule),
        deployment.status,
      ];
      console.log(`  ${name}  ${rule}  ${status}`);
    }
  } catch (error) {
    console.log(chalk.red`> Failed to fetch deployments`);
    if (error instanceof Error) console.log(chalk.red(`> ${error.message}`));
  }
};

const getStatsLengths = maxLengthForFields([
  "name",
  "mem",
  "memPercentage",
  "cpuPercentage",
]);

const GAP = 3;
/**
 *
 * @param {{name: string, mem: number, memPercentage: number, cpuPercentage: number}[]} stats
 */
function printStats(stats) {
  const humanReadable = stats.map((s) => ({
    name: s.name,
    mem: s.mem + `MB`,
    memPercentage: s.memPercentage + `%`,
    cpuPercentage: s.cpuPercentage + "%",
  }));
  const lens = getStatsLengths([
    ...humanReadable,
    {
      name: "Deployment name",
      mem: "Memory used",
      memPercentage: "%",
      cpuPercentage: "CPU usage percentage",
    },
  ]);

  console.log(`> Current resource usage\n`);
  console.log(
    [
      "  ",
      chalk.grey("Deployment name".padEnd(lens.name + GAP, " ")),
      chalk.grey("Memory used".padEnd(lens.mem + GAP, " ")),
      chalk.grey("%".padEnd(lens.memPercentage + GAP, " ")),
      chalk.grey("CPU usage percentage".padEnd(lens.cpuPercentage + GAP, " ")),
      "\n",
    ].join("")
  );
  for (const deployment of humanReadable) {
    console.log(
      [
        "  ",
        deployment.name.padEnd(lens.name + GAP, " "),
        deployment.mem.padEnd(lens.mem + GAP, " "),
        deployment.memPercentage.padEnd(lens.memPercentage + GAP, " "),
        deployment.cpuPercentage.padEnd(lens.cpuPercentage + GAP, " "),
      ].join("")
    );
  }
}

export const checkResourceUsage = async ({ ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);
    const { data: stats } = await httpClient.get("/instance/stats");
    printStats(stats);
  } catch (error) {
    console.log(chalk.red`> Failed to fetch stats`);
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

export const getDeploymentLogsHandler = async ({ name, ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);
    const { data: logs } = await httpClient.get(`/deployments/${name}/logs`);

    for (const { stream } of logs) console.log(stream);
  } catch (error) {
    console.log(chalk.red`> Failed to fetch logs: ` + name);
    if (error instanceof Error) console.log(chalk.red(`> ${error.message}`));
  }
};
