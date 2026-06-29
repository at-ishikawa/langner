// Playwright globalTeardown: drop the ephemeral test DB after the suite.
// Skipped when LANGNER_KEEP_TEST_DB=1 (useful when debugging a failed run).

import { Client } from "pg";

const HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const PORT = Number(process.env.LANGNER_TEST_DB_PORT ?? 5432);
const USER = process.env.LANGNER_TEST_DB_USER ?? "postgres";
const PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";

export default async function globalTeardown() {
  if (process.env.LANGNER_KEEP_TEST_DB === "1") return;
  const conn = new Client({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
    database: "postgres",
  });
  await conn.connect();
  await conn.query(`DROP DATABASE IF EXISTS "${NAME}"`);
  await conn.end();
}
