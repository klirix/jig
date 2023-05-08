import express from "express";
import requireAuth from "../middlewares/requireAuth";
import bodyParser from "body-parser";
import { docker } from "../docker";

const instanceRouter = express.Router();

instanceRouter.use(requireAuth, bodyParser.json());

instanceRouter.get("/stats", async (_, res) => {
  const runningContainers = await docker.listContainers();
  const stats = await Promise.all(
    runningContainers.map(async (c) => {
      const containerStats = await docker
        .getContainer(c.Id)
        .stats({ "one-shot": true });
      const usedMemory =
        containerStats.memory_stats.usage -
        containerStats.memory_stats.stats.cache;

      const cpuD =
        containerStats.cpu_stats.cpu_usage.total_usage -
        containerStats.precpu_stats.cpu_usage.total_usage;

      const sysCpuD =
        containerStats.cpu_stats.system_cpu_usage -
        containerStats.precpu_stats.system_cpu_usage;

      const cpuNum = containerStats.cpu_stats.online_cpus;

      return {
        name: c.Names[0],
        stats: {
          mem: usedMemory,
          limit: containerStats.memory_stats.limit,
          cpuPercentage: (cpuD / sysCpuD) * cpuNum * 100.0,
        },
      };
    })
  );

  res.json(stats);
});

export default instanceRouter;
