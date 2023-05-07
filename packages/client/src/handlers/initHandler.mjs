import inquirer from "inquirer";
import { existsSync, writeFileSync } from "fs";

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

async function detectProjectType() {
  if (existsSync("Dockerfile")) return;
  switch (true) {
    case existsSync("svelte.config.js"):
      {
        const { addDockerfile } = await inquirer.prompt({
          type: "confirm",
          message:
            "Looks like you have a sveltekit project, do you want a default Dockerfile?",
          name: "addDockerfile",
        });
        if (addDockerfile) {
          writeFileSync("Dockerfile", svelteDockerfileTxt);
        }
      }
      break;

    case existsSync("next.config.js"):
      {
        const { addDockerfile } = await inquirer.prompt({
          type: "confirm",
          message:
            "Looks like you have a Next.js project, do you want a default Dockerfile?",
          name: "addDockerfile",
        });
        if (addDockerfile) {
          writeFileSync("Dockerfile", svelteDockerfileTxt);
        }
      }
      break;

    case existsSync("package.json"):
      {
        const { addDockerfile, port } = await inquirer.prompt([
          {
            type: "confirm",
            message:
              "Looks like you have a node project, do you want a default Dockerfile?",
            name: "addDockerfile",
          },
          {
            type: "number",
            message: "What port your app is listening on?",
            name: "port",
          },
        ]);
        if (addDockerfile) {
          writeFileSync("Dockerfile", nodeDockerfile(port));
        }
      }
      break;

    default:
      console.log(
        `Looks like you do not have a Dockerfile, and we can't detect the type of your project 😔`
      );
      console.log(
        `However there are lots of tutorials online, so you won't get too lost 😀`
      );
      break;
  }
}

const nodeDockerfile = (port = 3000) =>
  ```
from node:alpine as base

# -- BUILDER

from base as builder
workdir /app
copy package.json package.json
run yarn
copy . .
run yarn build
expose ${port}
cmd ["yarn", "start"]
```;

const svelteDockerfileTxt = ```
from node:alpine as base

# -- BUILDER

from base as builder
workdir /app
copy package.json package.json
run yarn
copy . .
run yarn build

# -- PROD DEPS

from base as deps
workdir /app
copy package.json package.json
run yarn --prod

# -- RUNNER

from base as runner
workdir /app
copy package.json package.json
copy --from=builder /app/build build
copy --from=deps /app/node_modules node_modules
expose 3000
cmd ["node", "build"]
```;

const nextDockerfile = ```
FROM node:18-alpine as base

FROM base AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app

COPY package.json package-lock.json ./
RUN  yarn install --production

FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED 1

RUN yarn run build

FROM base AS runner
WORKDIR /app

ENV NODE_ENV production
ENV NEXT_TELEMETRY_DISABLED 1

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder --chown=nextjs:nodejs /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./package.json

USER nextjs

EXPOSE 3000

ENV PORT 3000

CMD ["yarn", "start"]
```;
