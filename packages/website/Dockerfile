from node:alpine as base

# -- BUILDER

from base as builder

workdir /app

copy package.json package.json

run yarn

copy . .

run yarn build

run ls

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
