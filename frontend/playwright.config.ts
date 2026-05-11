import { defineConfig, devices } from "@playwright/test";
import { defineBddConfig } from "playwright-bdd";

const FRONTEND_PORT = 3100;
const BACKEND_PORT = 8080;

const testDir = defineBddConfig({
  features: "e2e/features/**/*.feature",
  steps: "e2e/steps/**/*.ts",
});

const TEST_CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";

export default defineConfig({
  testDir,
  timeout: 60000,
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  reporter: [
    ["list"],
    ["./e2e/reporters/coverage-reporter.ts"],
  ],
  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
  },
  projects: [
    {
      name: "e2e",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: `cd .. && make -C backend build && ./langner-server --config ${TEST_CONFIG_PATH}`,
      port: BACKEND_PORT,
      reuseExistingServer: !process.env.CI,
      timeout: 60000,
    },
    {
      command: `pnpm dev --port ${FRONTEND_PORT}`,
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 60000,
      env: {
        NEXT_PUBLIC_API_BASE_URL: `http://localhost:${BACKEND_PORT}`,
        NEXT_PUBLIC_CONNECT_JSON: "true",
      },
    },
  ],
  workers: 1,
});
