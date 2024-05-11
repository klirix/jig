const jwt = require("jsonwebtoken");
const { randomUUID } = require("crypto");

const token = jwt.sign({ sub: randomUUID() }, process.env.JIG_SECRET);

console.log(`To start deploying run this in your terminal:`);
console.log(`jig login ${process.env.JIG_DOMAIN}+${token}`);
