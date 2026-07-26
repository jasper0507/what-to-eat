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
  webServer: {
    command: "npm run build && go run -tags testserver ./tests/testserver",
    url: "http://127.0.0.1:4174/api/healthz",
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      GOCACHE: "/tmp/what2eat-browser-go-cache",
      PORT: "4174",
    },
  },
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
