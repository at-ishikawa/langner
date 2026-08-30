// Playwright globalSetup: prepare an ephemeral PostgreSQL DB before the
// backend webServer starts. The schema is dropped and recreated each run
// so seed contents are deterministic.
//
// Steps:
//   1. Drop + create the test database.
//   2. Build the langner CLI.
//   3. Run `langner migrate import-db`, which applies schema migrations and
//      imports notebook fixtures in one pass.
//
// Env vars (with defaults for local dev):
//   LANGNER_TEST_DB_HOST     127.0.0.1
//   LANGNER_TEST_DB_PORT     5432
//   LANGNER_TEST_DB_USER     postgres
//   LANGNER_TEST_DB_PASSWORD password
//   LANGNER_TEST_DB_NAME     langner_e2e

import { execSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { Client } from "pg";

const HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const PORT = Number(process.env.LANGNER_TEST_DB_PORT ?? 5432);
const USER = process.env.LANGNER_TEST_DB_USER ?? "postgres";
const PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";

const REPO_ROOT = join(__dirname, "..", "..");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";
// Allowlisted email in config.e2e.yml. The minted cookie authenticates as this
// user for every spec.
const E2E_EMAIL = "e2e@example.com";
// Playwright storage state file written by this setup and consumed via
// `use.storageState` in playwright.config.ts.
export const STORAGE_STATE_PATH = join(__dirname, ".auth", "storageState.json");

export default async function globalSetup() {
  // Connect to the maintenance "postgres" database to drop/create the test DB,
  // since you can't drop a database you're connected to.
  const admin = new Client({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
    database: "postgres",
  });
  await admin.connect();
  // WITH (FORCE) lets us recover from a previous run that left connections
  // open (e.g. a crashed langner-server). Requires Postgres >= 13.
  await admin.query(`DROP DATABASE IF EXISTS "${NAME}" WITH (FORCE)`);
  await admin.query(`CREATE DATABASE "${NAME}" ENCODING 'UTF8'`);
  await admin.end();

  execSync("go build -o ../langner ./cmd/langner", {
    cwd: join(REPO_ROOT, "backend"),
    stdio: "inherit",
  });
  execSync(`./langner migrate import-db --config ${CONFIG_PATH}`, {
    cwd: REPO_ROOT,
    stdio: "inherit",
    env: { ...process.env, DB_PASSWORD: PASSWORD },
  });

  // Auth is enabled in the e2e config, so every RPC is gated behind a session
  // cookie. Mint one for the allowlisted e2e email and inject it via Playwright
  // storage state so all specs run authenticated (no real Google round-trip).
  const cookieValue = execSync(
    `./langner auth issue-test-cookie --email ${E2E_EMAIL} --config ${CONFIG_PATH}`,
    {
      cwd: REPO_ROOT,
      encoding: "utf8",
      env: { ...process.env, DB_PASSWORD: PASSWORD },
    },
  )
    .trim()
    .split("\n")
    .filter((line) => line.trim() !== "")
    .pop();

  if (!cookieValue) {
    throw new Error("failed to issue e2e session cookie");
  }

  const storageState = {
    cookies: [
      {
        name: "langner_session",
        value: cookieValue,
        // Host-only "localhost" cookie: applies to both the frontend (3100)
        // and backend (8080) since cookies ignore the port.
        domain: "localhost",
        path: "/",
        expires: Math.floor(Date.now() / 1000) + 30 * 24 * 60 * 60,
        httpOnly: true,
        secure: false,
        sameSite: "Lax" as const,
      },
    ],
    origins: [],
  };
  mkdirSync(join(__dirname, ".auth"), { recursive: true });
  writeFileSync(STORAGE_STATE_PATH, JSON.stringify(storageState, null, 2));
}
