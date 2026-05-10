// Playwright globalSetup: prepare an ephemeral MySQL DB before the backend
// webServer starts. The schema is dropped and recreated each run so seed
// contents are deterministic.
//
// Steps:
//   1. Drop + create the test database.
//   2. Apply backend/schemas/migrations/*.up.sql in numeric order.
//   3. Build the langner CLI and import notebook fixtures into the DB.
//
// Env vars (with defaults for local dev):
//   LANGNER_TEST_DB_HOST     127.0.0.1
//   LANGNER_TEST_DB_PORT     3306
//   LANGNER_TEST_DB_USER     root
//   LANGNER_TEST_DB_PASSWORD password
//   LANGNER_TEST_DB_NAME     langner_e2e

import { execSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import mysql from "mysql2/promise";

const HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const PORT = Number(process.env.LANGNER_TEST_DB_PORT ?? 3306);
const USER = process.env.LANGNER_TEST_DB_USER ?? "root";
const PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";

const REPO_ROOT = join(__dirname, "..", "..");
const MIGRATIONS_DIR = join(REPO_ROOT, "backend", "schemas", "migrations");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.test.yml";

export default async function globalSetup() {
  // Step 1: drop + create the schema.
  const admin = await mysql.createConnection({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
    multipleStatements: true,
  });
  await admin.query(`DROP DATABASE IF EXISTS \`${NAME}\``);
  await admin.query(
    `CREATE DATABASE \`${NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`,
  );
  await admin.end();

  // Step 2: apply schema migrations in order.
  const migrations = readdirSync(MIGRATIONS_DIR)
    .filter((f) => f.endsWith(".up.sql"))
    .sort();
  const conn = await mysql.createConnection({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
    database: NAME,
    multipleStatements: true,
  });
  for (const file of migrations) {
    const sql = readFileSync(join(MIGRATIONS_DIR, file), "utf8");
    if (sql.trim()) {
      await conn.query(sql);
    }
  }
  await conn.end();

  // Step 3: build langner CLI and import notebook fixtures.
  execSync("make -C backend build", { cwd: REPO_ROOT, stdio: "inherit" });
  execSync(`./langner migrate import-db --config ${CONFIG_PATH}`, {
    cwd: REPO_ROOT,
    stdio: "inherit",
    env: { ...process.env, DB_PASSWORD: PASSWORD },
  });
}
