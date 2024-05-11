import express from "express";
import requireAuth from "../middlewares/requireAuth";
import bodyParser from "body-parser";
import { docker } from "../docker";

const instanceRouter = express.Router();

instanceRouter.use(requireAuth, bodyParser.json());

const MB = 1024 * 1024;

instanceRouter.get("/stats", async (_, res) => {
  const runningContainers = await docker.listContainers();
  const stats = await Promise.all(
    runningContainers
      .filter(({ Labels }) => Labels["jig.name"])
      .map(async (c) => {
        const containerStats = await docker
          .getContainer(c.Id)
          .stats({ "one-shot": true, stream: false });
        const usedMemory =
          containerStats.memory_stats.usage -
          (containerStats.memory_stats.stats.cache || 0);

        const cpuD =
          containerStats.cpu_stats.cpu_usage.total_usage -
          containerStats.precpu_stats.cpu_usage.total_usage;

        const sysCpuD =
          containerStats.cpu_stats.system_cpu_usage -
          containerStats.precpu_stats.cpu_usage.total_usage;

        const cpuNum = containerStats.cpu_stats.online_cpus;

        console.log(
          containerStats.memory_stats.usage,
          containerStats.memory_stats.stats,
          containerStats.memory_stats.limit
        );

        return {
          name: c.Names[0].replace("/", ""),
          mem: Math.round((usedMemory / MB) * 100) / 100,
          memPercentage:
            Math.round(
              (usedMemory / containerStats.memory_stats.limit) * 10000
            ) / 100,
          cpuPercentage: Math.round((cpuD / sysCpuD) * cpuNum * 10000) / 100,
        };
      })
  );

  res.json(stats);
});

export default instanceRouter;
