import { defineConfig } from "@playwright/test"

export default defineConfig({
  testDir: "./e2e",
  testMatch: "announcements.integration.spec.ts",
  forbidOnly: Boolean(process.env.CI),
  workers: 1,
  timeout: 60000,
  reporter: "list",
  outputDir: "/tmp/charge-announcement-integration-results",
  use: {
    baseURL: "http://127.0.0.1:18081",
    screenshot: "only-on-failure",
    actionTimeout: 10000,
    navigationTimeout: 20000,
  },
  webServer: {
    command:
      process.env.SKIP_STATIC_BUILD === "1"
        ? "bash ../scripts/announcement-test-server.sh"
        : "pnpm run build:static && bash ../scripts/announcement-test-server.sh",
    url: "http://127.0.0.1:18081/healthz",
    reuseExistingServer: false,
    timeout: 120000,
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
})
