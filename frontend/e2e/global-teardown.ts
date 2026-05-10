// Playwright globalTeardown: drop the ephemeral test DB after the suite.
// Skipped when LANGNER_KEEP_TEST_DB=1 (useful when debugging a failed run).

import mysql from "mysql2/promise";

const HOST = process.env.LANGNER_TEST_DB_HOST ?? "127.0.0.1";
const PORT = Number(process.env.LANGNER_TEST_DB_PORT ?? 3306);
const USER = process.env.LANGNER_TEST_DB_USER ?? "root";
const PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
const NAME = process.env.LANGNER_TEST_DB_NAME ?? "langner_e2e";

export default async function globalTeardown() {
  if (process.env.LANGNER_KEEP_TEST_DB === "1") return;
  const conn = await mysql.createConnection({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
  });
  await conn.query(`DROP DATABASE IF EXISTS \`${NAME}\``);
  await conn.end();
}
