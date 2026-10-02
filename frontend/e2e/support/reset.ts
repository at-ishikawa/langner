// Per-scenario state reset for the e2e harness.
//
// The e2e stack runs the backend in DB mode against ONE Postgres seeded once in
// global-setup, with workers:1 and no reset between scenarios. DB-mode reads
// reflect earlier scenarios' quiz writes (a word answered correctly in
// quiz-freeform is no longer due in quiz-standard), so we restore the seeded
// baseline before every scenario.
//
// Restoring the baseline is a single `langner-admin migrate reset-e2e`: it
// TRUNCATEs the learning-content STATE tables (undoing the prior scenario's
// quiz writes) and re-seeds the SAME deterministic fixtures the server started
// with, through the fixtures library (internal/dbseed) — NOT the import pipeline
// (reset-db) and NOT the removed learning_notes YAML. Notebook CONTENT is served
// from the on-disk fixtures, never the DB, so there is nothing to re-import.
//
// reset-e2e deliberately leaves users, notebook ownership, and the CLI auth
// tables untouched, so the pre-minted access token (see global-setup.ts) keeps
// resolving to the same user id=1 — no re-provision or re-mint needed.

import { execFileSync } from "node:child_process";
import { join } from "node:path";

// frontend/e2e/support -> repo root
const REPO_ROOT = join(__dirname, "..", "..", "..");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";
const DB_PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";

/**
 * Restore the seeded baseline: truncate the learning-content state tables and
 * re-seed the deterministic fixtures via the fixtures library. Throws with
 * captured output on failure so a broken reset is visible in CI rather than
 * silently corrupting later scenarios.
 */
export function resetState(): void {
  execFileSync("./langner-admin", ["migrate", "reset-e2e", "--config", CONFIG_PATH], {
    cwd: REPO_ROOT,
    stdio: "pipe",
    env: { ...process.env, DB_PASSWORD },
  });
}
