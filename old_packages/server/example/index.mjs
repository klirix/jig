import http from "http";

const server = http.createServer((req, res) => {
  res.write("ok1");
});

server.listen(8080);
