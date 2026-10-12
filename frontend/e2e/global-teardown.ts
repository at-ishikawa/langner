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
  // PREVIEW mode: the DB is the remote preview Supabase (managed externally,
  // reset per scenario via the seed endpoint) and there is no local Postgres to
  // drop — skip teardown entirely.
  if (process.env.E2E_PREVIEW_BACKEND_URL) return;
  const conn = new Client({
    host: HOST,
    port: PORT,
    user: USER,
    password: PASSWORD,
    database: "postgres",
  });
  await conn.connect();
  // WITH (FORCE) terminates any lingering backend connections to the test DB
  // before dropping it. Without this, the running langner-server webServer's
  // pool still holds connections and Postgres refuses with "database is being
  // accessed by other users". Requires Postgres >= 13; we're on 16.
  await conn.query(`DROP DATABASE IF EXISTS "${NAME}" WITH (FORCE)`);
  await conn.end();
}
