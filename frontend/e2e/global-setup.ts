// Playwright globalSetup: prepare an ephemeral MySQL DB before the backend
// webServer starts. The schema is dropped and recreated each run so seed
// contents are deterministic.
//
// Steps:
//   1. Drop + create the test database.
//   2. Build the langner CLI.
//   3. Run `langner migrate import-db`, which applies schema migrations and
//      imports notebook fixtures in one pass.
//
// Env vars (with defaults for local dev):
//   LANGNER_TEST_DB_HOST     127.0.0.1
//   LANGNER_TEST_DB_PORT     3306
//   LANGNER_TEST_DB_USER     root
//   LANGNER_TEST_DB_PASSWORD password
//   LANGNER_TEST_DB_NAME     langner_e2e

import { execSync } from "node:child_process";
import { join } from "node:path";
import mysql from "mysql2/promise";

const HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const PORT = Number(process.env.LANGNER_TEST_DB_PORT ?? 3306);
const USER = process.env.LANGNER_TEST_DB_USER ?? "root";
const PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";

const REPO_ROOT = join(__dirname, "..", "..");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.test.yml";

export default async function globalSetup() {
  const admin = await mysql.createConnection({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
  });
  await admin.query(`DROP DATABASE IF EXISTS \`${NAME}\``);
  await admin.query(
    `CREATE DATABASE \`${NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`,
  );
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
