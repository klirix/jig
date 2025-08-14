FROM oven/bun:slim as base

FROM base as builder
WORKDIR /app
# Install dependencies
COPY package.json package.json
RUN bun install
# Copy source code
COPY . .
# Build
RUN bun run build


FROM base as proddeps
WORKDIR /app
# Install dependencies
COPY package.json package.json
RUN bun install --production

FROM base
WORKDIR /app
# Copy dependencies
COPY --from=proddeps /app/node_modules /app/node_modules
# Copy built source code
COPY --from=builder /app/.next /app/.next
COPY package.json package.json
# Start the app
CMD ["bun", "run", "--smol", "start"]
