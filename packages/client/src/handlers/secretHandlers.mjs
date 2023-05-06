import chalk from "chalk";
import { makeHttpClient } from "../httpClient.mjs";

export const listSecrets = async (authOptions) => {
  try {
    const httpClient = makeHttpClient(authOptions);

    const { data: secrets } = await httpClient.get("/secrets");

    if (secrets.length) {
      console.log(`> Secrets: \n`);
      console.log(`  ${chalk.grey("name")}`);
      for (const { key } of secrets) {
        console.log(`  ${key}`);
      }
    } else {
      console.log(`> No secrets are set yet!`);
    }
  } catch (error) {
    console.log(error);
  }
};
export const removeSecretHanlder = async ({ key, ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);

    const { data: secret } = await httpClient.delete(`/secrets/${key}`);

    console.log(`> Secret ${secret} removed!`);
  } catch (error) {
    console.log(error);
  }
};
export const addSecretHandler = async ({ key, value, ...auth }) => {
  try {
    const httpClient = makeHttpClient(auth);

    const { data: secret } = await httpClient.put(`/secrets`, {
      key,
      value,
    });

    console.log(`> Secret ${secret} added!`);
  } catch (error) {
    console.log(error);
  }
};
