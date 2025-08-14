FROM node:21-alpine as base

# Fool sveltekit auto adapter into using the node adapter
ENV GCP_BUILDPACKS=true
# -- BUILDER

FROM base as builder
WORKDIR /app
COPY package.json package.json
RUN yarn
COPY . .
RUN yarn build

# -- PROD DEPS

FROM base as deps
WORKDIR /app
COPY package.json package.json
RUN yarn --prod

# -- RUNNER

FROM base as runner
WORKDIR /app
COPY package.json package.json
COPY --from=builder /app/build build
COPY --from=deps /app/node_modules node_modules
EXPOSE 3000
CMD ["node", "build"]