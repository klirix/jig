FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY ./ ./
RUN go mod download
RUN go build ./pkgs/server/


FROM alpine:latest
RUN apk add --no-cache docker-cli docker-cli-compose
COPY --from=builder /build/server /app/server
EXPOSE 5000
WORKDIR /app
CMD ["/app/server"]
