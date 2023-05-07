import "dotenv/config";
import express from "express";
import requestLog from "morgan";
import eWS from "@wll8/express-ws";
import { initTraefik } from "./lib/initTraefik";
import secretsRouter from "./routers/secrets.router";
import deploymentsRouter from "./routers/deployments.router";

const { app } = eWS(express());

app.use(requestLog("dev"));

app.use("/secrets", secretsRouter);
app.use("/deployments", deploymentsRouter);

app.get("/", (req, res) => {
  res.redirect(`https://deploywithjig.askh.at/welcome`);
});

async function main() {
  await initTraefik();
  app.listen(8888, () => {
    console.log("Jig is online, letsssgooooo!!! 🚀🚀🚀");
  });
}

main();
