import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  reporter: "line",
  outputDir: "/tmp/what2eat-playwright-results",
  use: {
    baseURL: "http://127.0.0.1:4174",
  },
  webServer: [
    {
      command: "node tests/nim-stub.mjs",
      url: "http://127.0.0.1:4175/healthz",
      reuseExistingServer: false,
    },
    {
      command: "npm run build && go run ../cmd/server",
      url: "http://127.0.0.1:4174/api/healthz",
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        CATALOG_DIR: "../internal/server/testdata/catalog",
        DATABASE_PATH: ":memory:",
        GOCACHE: "/tmp/what2eat-browser-go-cache",
        NVIDIA_API_KEY: "browser-test-secret",
        NIM_BASE_URL: "http://127.0.0.1:4175/v1",
        PORT: "4174",
        WEB_DIR: "dist",
      },
    },
  ],
  projects: [
    {
      name: "mobile",
      use: { viewport: { width: 390, height: 844 } },
    },
    {
      name: "desktop",
      use: { viewport: { width: 1280, height: 800 } },
    },
  ],
});
