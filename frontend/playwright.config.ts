import { defineConfig, devices } from "@playwright/test";
import { defineBddConfig } from "playwright-bdd";

const FRONTEND_PORT = 3100;
const BACKEND_PORT = 8080;

const testDir = defineBddConfig({
  features: "e2e/features/**/*.feature",
  steps: "e2e/steps/**/*.ts",
});

const TEST_CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";

// Playwright starts the webServers and awaits their readiness BEFORE running
// globalSetup. So the backend server must provision its own database — nothing
// globalSetup does is available yet. The backend webServer command therefore
// (re)creates the test DB, imports notebooks, upserts the allowlisted e2e user
// and writes that user's signed session cookie to a file, all BEFORE binding
// its port. This makes the seed a hard prerequisite of "server ready": the
// server can never serve `/auth/me` before the user exists, which is the race
// that redirected every authenticated page to /login. globalSetup then only
// turns the cookie file into Playwright storage state (see global-setup.ts).
// DB coords come from the LANGNER_TEST_DB_* env (config.e2e.yml in CI).
const DB_HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const DB_PORT = process.env.LANGNER_TEST_DB_PORT ?? "5432";
const DB_USER = process.env.LANGNER_TEST_DB_USER ?? "postgres";
const DB_PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const DB_NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";
const E2E_EMAIL = "e2e@example.com";
// Cookie file the backend command writes and global-setup.ts reads. Kept next
// to the storage state under frontend/e2e/.auth/.
const COOKIE_FILE = "frontend/e2e/.auth/cookie.txt";
// Recreate the DB, seed it, mint the cookie, then start the server. `&&`
// chaining makes any failed step fail the webServer (surfaced by Playwright),
// and the seed is complete before `langner-server` binds BACKEND_PORT.
const backendSeedAndServe = [
  "cd ..",
  "(cd backend && go build -o ../langner ./cmd/langner && go build -o ../langner-server ./cmd/langner-server)",
  `PGPASSWORD=${DB_PASSWORD} psql -h ${DB_HOST} -p ${DB_PORT} -U ${DB_USER} -d postgres -v ON_ERROR_STOP=1 ` +
    `-c "DROP DATABASE IF EXISTS ${DB_NAME} WITH (FORCE)" -c "CREATE DATABASE ${DB_NAME} ENCODING 'UTF8'"`,
  `DB_PASSWORD=${DB_PASSWORD} ./langner migrate import-db --config ${TEST_CONFIG_PATH}`,
  `mkdir -p frontend/e2e/.auth`,
  `DB_PASSWORD=${DB_PASSWORD} ./langner auth issue-test-cookie --email ${E2E_EMAIL} --config ${TEST_CONFIG_PATH} > ${COOKIE_FILE}`,
  `./langner-server --config ${TEST_CONFIG_PATH}`,
].join(" && ");

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
      command: backendSeedAndServe,
      port: BACKEND_PORT,
      reuseExistingServer: !process.env.CI,
      // Generous: this build (two binaries) + DB recreate + import + seed all
      // run before the port opens.
      timeout: 180000,
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
