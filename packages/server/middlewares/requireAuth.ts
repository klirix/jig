import { RequestHandler } from "express";
import { randomUUID } from "crypto";
import jwt from "jsonwebtoken";
import env from "../env";

const requireAuth: RequestHandler = (req, res, next) => {
  // implement this middleware to check if the user is authenticated based on jwt in header
  const authorization = req.headers.authorization;
  if (!authorization) return next(new Error("Unauthorized"));

  const token = authorization.split(" ")[1];

  try {
    jwt.verify(token, env.JIG_SECRET);
    next();
  } catch (error) {
    return next(new Error("Unauthorized"));
  }
};

export default requireAuth;

export const makeKey = () => {
  return jwt.sign({ sub: randomUUID() }, env.JIG_SECRET);
};
