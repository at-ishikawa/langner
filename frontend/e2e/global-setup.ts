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
import { join } from "node:path";
import { Client } from "pg";

const HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const PORT = Number(process.env.LANGNER_TEST_DB_PORT ?? 5432);
const USER = process.env.LANGNER_TEST_DB_USER ?? "postgres";
const PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";

const REPO_ROOT = join(__dirname, "..", "..");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";

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
  await admin.query(`DROP DATABASE IF EXISTS "${NAME}"`);
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
}
