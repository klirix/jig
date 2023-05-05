import z from "zod";
const envSchema = z.object({
  JIG_SSL_EMAIL: z.string().email(),
  JIG_SECRET: z.string(),
});

const env = envSchema.parse(process.env);

export default env;
