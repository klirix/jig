import express from "express";
import requireAuth from "../middlewares/requireAuth";
import z from "zod";
import bodyParser from "body-parser";
import { Secret, secrets } from "../dbs";

const secretsRouter = express.Router();

secretsRouter.use(requireAuth, bodyParser.json());

const stripValues = function* (secrets: IterableIterator<Secret>) {
  for (const secret of secrets) {
    yield { key: secret.key };
  }
};

const putSecretSchema = z.object({
  key: z.string(),
  value: z.string(),
});

secretsRouter.put("/", (req, res) => {
  const body = putSecretSchema.parse(req.body);
  secrets.set(body.key, body);
  res.json(body.key);
});

secretsRouter.get("/", (req, res) => {
  res.json(Array.from(stripValues(secrets.values())));
});

secretsRouter.get("/:key/inspect", (req, res) => {
  const { key } = req.params;
  if (!secrets.has(key)) return res.sendStatus(404);
  res.json(secrets.get(key));
});

secretsRouter.delete("/:key", (req, res) => {
  const { key } = req.params;
  secrets.delete(key);
  res.json(key);
});

export default secretsRouter;
