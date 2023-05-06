import Conf from "conf";
import axios from "axios";

export const config = new Conf({ projectName: "jig" });

export const makeHttpClient = ({
  endpoint = config.get("endpoint"),
  token = config.get("token"),
}) => {
  if (!token) {
    throw new Error("You need to login, use token command!");
  }
  if (!endpoint) {
    throw new Error("Jig endpoint needs to be set, try endpoint command");
  }
  return axios.create({
    baseURL: endpoint,
    headers: { Authorization: "Bearer " + token },
  });
};
