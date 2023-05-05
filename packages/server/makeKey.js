const jwt = require("jsonwebtoken");
const { randomUUID } = require("crypto");

console.log(
  `Your key: ` + jwt.sign({ sub: randomUUID() }, process.env.JIG_SECRET)
);
