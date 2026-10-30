import { defineConfig, devices } from "@playwright/test";
import { defineBddConfig } from "playwright-bdd";

const FRONTEND_PORT = 3100;
const BACKEND_PORT = 8080;
// Mock OAuth provider standing in for Google (config.e2e.yml auth.oauth_endpoint).
const MOCK_OAUTH_PORT = 8090;

const testDir = defineBddConfig({
  features: "e2e/features/**/*.feature",
  steps: "e2e/steps/**/*.ts",
});

const TEST_CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";

// Playwright starts the webServers and awaits their readiness BEFORE running
// globalSetup. So the backend server must provision its own database — nothing
// globalSetup does is available yet. The backend webServer command therefore
// (re)creates the test DB, imports notebooks, upserts the allowlisted e2e user
// and writes that user's bearer access token to a file, all BEFORE binding its
// port. This makes the seed a hard prerequisite of "server ready": the server
// can never serve `/auth/me` before the user exists, which is the race that
// redirected every authenticated page to /login. Auth is a bearer held in the
// SPA's memory (no cookie), so each scenario injects the token into
// `window.__LANGNER_ACCESS_TOKEN__` before the app loads (e2e/steps/common.ts);
// globalSetup only waits for the token file to exist (see global-setup.ts).
// DB coords come from the LANGNER_TEST_DB_* env (config.e2e.yml in CI).
const DB_HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const DB_PORT = process.env.LANGNER_TEST_DB_PORT ?? "5432";
const DB_USER = process.env.LANGNER_TEST_DB_USER ?? "postgres";
const DB_PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const DB_NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";
const E2E_EMAIL = "e2e@example.com";
// Access-token file the backend command writes and common.ts reads. Kept under
// frontend/e2e/.auth/.
const TOKEN_FILE = "frontend/e2e/.auth/access-token.txt";
// Recreate the DB, seed it, mint the token, verify the user persisted, then
// exec the server — all in a committed script (e2e/seed-and-serve.sh) rather
// than a long `&&`-joined string here. The script runs `set -euo pipefail`
// (any failed step fails the webServer, surfaced by Playwright, instead of
// silently starting an unseeded server), prints `[seed]` markers so CI shows
// which steps ran, and hard-fails if the users table is empty after seeding.
// The seed is complete before `langner-server` binds BACKEND_PORT. Coords are
// passed via env below.
const seedEnv: Record<string, string> = {
  DB_HOST,
  DB_PORT,
  DB_USER,
  DB_PASSWORD,
  DB_NAME,
  E2E_EMAIL,
  TEST_CONFIG_PATH,
  TOKEN_FILE,
};

// PREVIEW mode drives the DEPLOYED Vercel app, where a cold serverless function
// + Supabase pooler round-trip can take several seconds — far longer than a
// localhost server. Give assertions, actions, navigations, and the whole test
// generous timeouts there so cold-start latency doesn't masquerade as a failure;
// local keeps the snappy defaults.
const PREVIEW = Boolean(process.env.PLAYWRIGHT_BASE_URL);

export default defineConfig({
  testDir,
  timeout: PREVIEW ? 120_000 : 60_000,
  // Against the deployed preview, a cold serverless function + pooled DB
  // connection can occasionally stall a first request past the (already
  // generous) timeout. Retry twice there so irreducible cold-start latency is
  // surfaced as "flaky" rather than failing the run — a test that is genuinely
  // broken still fails on every attempt. Local (deterministic) keeps 0 retries.
  retries: PREVIEW ? 2 : 0,
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  expect: { timeout: PREVIEW ? 45_000 : 5_000 },
  reporter: [
    ["list"],
    ["html", { open: "never" }],
    ["./e2e/reporters/coverage-reporter.ts"],
  ],
  use: {
    actionTimeout: PREVIEW ? 30_000 : 0,
    navigationTimeout: PREVIEW ? 45_000 : 0,
    // In PREVIEW mode (PLAYWRIGHT_BASE_URL set by the e2e-preview CI job) the
    // app under test is the deployed Vercel frontend, which talks to the
    // deployed backend — so the exact same scenarios run against real serverless
    // + DB-backed wiring, the environment where filesystem-vs-DB and cold-start
    // bugs actually live. Locally it stays localhost with the spawned servers.
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? `http://localhost:${FRONTEND_PORT}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    // No storageState: auth is a bearer held in the SPA's memory, seeded per
    // scenario by injecting window.__LANGNER_ACCESS_TOKEN__ (e2e/steps/common.ts)
    // before the app loads. auth is enabled in config.e2e.yml, so all RPCs are
    // gated behind that bearer.
  },
  projects: [
    {
      name: "e2e",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  // Preview mode drives the deployed Vercel app, so spawn no local servers; the
  // e2e-preview CI job seeds the preview DB and writes the token file first.
  webServer: process.env.PLAYWRIGHT_BASE_URL ? [] : [
    {
      command: "bash e2e/seed-and-serve.sh",
      env: seedEnv,
      port: BACKEND_PORT,
      reuseExistingServer: !process.env.CI,
      // Generous: this build (two binaries) + DB recreate + import + seed all
      // run before the port opens.
      timeout: 180000,
    },
    {
      command: "node e2e/mock-oauth-server.mjs",
      env: { MOCK_OAUTH_PORT: String(MOCK_OAUTH_PORT), E2E_EMAIL },
      url: `http://localhost:${MOCK_OAUTH_PORT}/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 10000,
    },
    {
      command: `pnpm dev --port ${FRONTEND_PORT}`,
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 60000,
      env: {
        // 127.0.0.1, not localhost: the API must be a different cookie host than
        // the frontend, as in production (see config.e2e.yml auth comment).
        NEXT_PUBLIC_API_BASE_URL: `http://127.0.0.1:${BACKEND_PORT}`,
        NEXT_PUBLIC_CONNECT_JSON: "true",
      },
    },
  ],
  workers: 1,
});
