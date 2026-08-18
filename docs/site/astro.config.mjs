import { defineConfig } from "astro/config";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import tidewave from "tidewave/vite-plugin";

const generatedVersion = new URL("./version.txt", import.meta.url);
const workspaceVersion = process.env.BUILD_WORKSPACE_DIRECTORY
  ? resolve(process.env.BUILD_WORKSPACE_DIRECTORY, "VERSION")
  : undefined;
const versionPath = existsSync(generatedVersion) ? generatedVersion : workspaceVersion;
if (!versionPath) throw new Error("Hovel version input is unavailable");
const hovelVersion = readFileSync(versionPath, "utf8").trim();
const tidewaveEnabled = process.env.HOVEL_TIDEWAVE === "1";

export default defineConfig({
  output: "static",
  trailingSlash: "ignore",
  compressHTML: false,
  build: {
    format: "preserve",
  },
  outDir: "./dist",
  cacheDir: `./${process.env.ASTRO_CACHE_DIR ?? "dist/.astro-cache"}`,
  vite: {
    plugins: tidewaveEnabled
      ? [
          tidewave({
            allowRemoteAccess: false,
            tmpDir: "dist/.tidewave",
          }),
        ]
      : [],
    define: {
      __HOVEL_RELEASE_TAG__: JSON.stringify(`v${hovelVersion}`),
      __HOVEL_VERSION__: JSON.stringify(hovelVersion),
    },
    build: {
      sourcemap: false,
    },
  },
});
