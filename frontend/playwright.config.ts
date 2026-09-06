import { defineConfig, devices } from "@playwright/test";
import { defineBddConfig } from "playwright-bdd";

const FRONTEND_PORT = 3100;
const BACKEND_PORT = 8080;

const testDir = defineBddConfig({
  features: "e2e/features/**/*.feature",
  steps: "e2e/steps/**/*.ts",
});

const TEST_CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";

// The backend webServer and globalSetup run concurrently: globalSetup drops +
// recreates the DB, imports notebooks, and (via `auth issue-test-cookie`)
// upserts the e2e user, while Playwright launches `langner-server`. Without a
// gate the server begins serving before the user is seeded, so `/auth/me`
// returns user-not-found and every authenticated page redirects to /login.
// Block the server on the seeded `users` row so it never serves an unseeded DB.
// DB coords mirror global-setup.ts (same LANGNER_TEST_DB_* env / config.e2e.yml).
const DB_HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const DB_PORT = process.env.LANGNER_TEST_DB_PORT ?? "5432";
const DB_USER = process.env.LANGNER_TEST_DB_USER ?? "postgres";
const DB_PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const DB_NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";
const waitForAuthSeed =
  `until PGPASSWORD=${DB_PASSWORD} psql -h ${DB_HOST} -p ${DB_PORT} -U ${DB_USER} ` +
  `-d ${DB_NAME} -tAc 'SELECT 1 FROM users LIMIT 1' 2>/dev/null | grep -q 1; ` +
  `do echo 'waiting for e2e auth seed…'; sleep 1; done`;

export default defineConfig({
  testDir,
  timeout: 60000,
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  reporter: [
    ["list"],
    ["html", { open: "never" }],
    ["./e2e/reporters/coverage-reporter.ts"],
  ],
  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    // Authenticate every spec with the session cookie minted in global-setup
    // (auth is enabled in config.e2e.yml, so all RPCs are gated).
    storageState: "e2e/.auth/storageState.json",
  },
  projects: [
    {
      name: "e2e",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: `cd .. && make -C backend build && ${waitForAuthSeed} && ./langner-server --config ${TEST_CONFIG_PATH}`,
      port: BACKEND_PORT,
      reuseExistingServer: !process.env.CI,
      timeout: 120000,
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
