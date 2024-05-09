FROM golang:alpine as builder
WORKDIR /build
COPY ./ ./
RUN go mod download
RUN go build ./pkgs/server/


FROM alpine:latest
COPY --from=builder /build/server /app/server
EXPOSE 5000
WORKDIR /app
CMD ["/app/server"]