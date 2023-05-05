import http from "http";

const server = http.createServer((req, res) => {
  res.write("ok3");
});

server.listen(8080);
