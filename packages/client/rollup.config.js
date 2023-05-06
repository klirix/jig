import { defineConfig } from "rollup";

export default defineConfig({
  input: "src/index.mjs",
  output: {
    file: "dist/index.mjs",
    banner: "#!/usr/bin/env node",
  },
});
