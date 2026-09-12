import { defineConfig } from "@playwright/test"

const port = process.env.WORKBENCH_TEST_PORT || "3100"

// Browser regression checks use intercepted API responses and no backend.
export default defineConfig({
  testDir: "./e2e",
  testMatch: "workbench.spec.ts",
  fullyParallel: true,
  reporter: "list",
  outputDir: "/tmp/charge-workbench-results",
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    locale: "zh-CN",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: {
    command: `pnpm dev --hostname 127.0.0.1 --port ${port}`,
    url: `http://127.0.0.1:${port}/login`,
    reuseExistingServer: !process.env.CI,
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
})
