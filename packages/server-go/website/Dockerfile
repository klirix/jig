FROM oven/bun:alpine as builder
WORKDIR /app
COPY package.json bun.lockb ./
RUN bun install
COPY . .
RUN bun run build

# Stage 2: Serve the static website using Nginx
FROM nginx
COPY --from=builder /app/build /usr/share/nginx/html
# CMD ["nginx", "-g", "daemon off;"]