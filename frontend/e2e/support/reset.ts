// Per-scenario state reset for the e2e harness.
//
// The e2e stack runs the backend in DB mode against ONE Postgres seeded once in
// global-setup, with workers:1 and no reset between scenarios. DB-mode reads
// reflect earlier scenarios' quiz writes (a word answered correctly in
// quiz-freeform is no longer due in quiz-standard), so we restore the seeded
// baseline before every scenario.
//
// LOCAL mode: a single `langner-admin migrate reset-e2e` against the local DB —
// it TRUNCATEs the learning-content STATE tables (undoing the prior scenario's
// quiz writes) and re-seeds the SAME deterministic fixtures through the fixtures
// library (internal/dbseed). Users, notebook ownership, and CLI auth tables are
// left untouched, so the pre-minted access token keeps resolving.
//
// PREVIEW mode (running against the deployed Vercel preview): there is no local
// Postgres or langner-admin binary on the runner, so reset STATE via the
// backend's own seed endpoint — a POST with NO body runs the exact same
// schema+provision+truncate+reseed as reset-e2e, server-side, leaving the
// already-imported notebook CONTENT in place. E2E_PREVIEW_BACKEND_URL and
// E2E_SEED_SECRET are provided by the e2e-preview workflow.

import { execFileSync } from "node:child_process";
import { join } from "node:path";

// frontend/e2e/support -> repo root
const REPO_ROOT = join(__dirname, "..", "..", "..");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";
const DB_PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";

const PREVIEW_BACKEND = process.env.E2E_PREVIEW_BACKEND_URL;
const SEED_SECRET = process.env.E2E_SEED_SECRET;

/**
 * Restore the seeded baseline: truncate the learning-content state tables and
 * re-seed the deterministic fixtures. Throws with captured output on failure so
 * a broken reset is visible in CI rather than silently corrupting later
 * scenarios.
 */
export function resetState(): void {
  if (PREVIEW_BACKEND && SEED_SECRET) {
    // Preview: reset STATE via the seed endpoint (no body ⇒ no content import;
    // the catalog persists across resets). Same baseline as reset-e2e.
    execFileSync(
      "curl",
      [
        "--fail",
        "-sS",
        "-X",
        "POST",
        `${PREVIEW_BACKEND}/internal/e2e/seed`,
        "-H",
        `X-E2E-Seed-Secret: ${SEED_SECRET}`,
      ],
      { stdio: "pipe" },
    );
    return;
  }
  execFileSync("./langner-admin", ["migrate", "reset-e2e", "--config", CONFIG_PATH], {
    cwd: REPO_ROOT,
    stdio: "pipe",
    env: { ...process.env, DB_PASSWORD },
  });
}
