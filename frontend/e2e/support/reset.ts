// Per-scenario state reset for the e2e harness.
//
// The e2e stack runs the backend in DB mode against ONE Postgres seeded once
// in global-setup, with workers:1 and no reset between scenarios. Once PR #26
// serves learning-history reads from the database, a quiz answer written in an
// earlier scenario PERSISTS and changes what a later scenario sees (a word
// answered correctly in quiz-freeform is no longer due in quiz-standard). The
// scenarios are each self-contained, so we restore the seeded baseline before
// every one.
//
// Two stores must return to baseline:
//   1. The database (vocabulary / etymology history) — `langner-admin migrate
//      reset-db` clears every data table and re-imports + re-seeds from the
//      source YAML.
//   2. The on-disk learning_notes YAML — the app dual-writes to it, and grammar
//      history is read from it (grammar has no DB home), so restore it from git
//      before re-importing.
//
// The auth accounts + notebook ownership `reset-db` CANNOT restore: auth
// provisioning is now DECOUPLED from reset-db (it no longer runs implicitly), so
// we run `langner-admin auth provision` explicitly after every reset to re-upsert
// the allowlist/admin accounts and re-assign notebook ownership from the config's
// notebook_ownership block. `reset-db` drops + re-migrates the managed tables
// (`users` among them, migration 024) and re-imports the YAML, which wipes those
// rows and would leave the pre-minted session cookie (see global-setup.ts)
// pointing at a nonexistent user — so `/auth/me` 401s and every authenticated
// page redirects to /login. We therefore re-provision and re-mint the same
// allowlisted user after every reset. The scoped rebuild restarts the BIGSERIAL
// sequence, so the re-inserted user reclaims id=1 and the fixed storageState
// cookie keeps resolving.

import { execFileSync } from "node:child_process";
import { join } from "node:path";

// frontend/e2e/support -> repo root
const REPO_ROOT = join(__dirname, "..", "..", "..");
const CONFIG_PATH = process.env.LANGNER_TEST_CONFIG ?? "config.e2e.yml";
const DB_PASSWORD = process.env.LANGNER_TEST_DB_PASSWORD ?? "password";
// Must match the email seed-and-serve.sh mints the session cookie for.
const E2E_EMAIL = process.env.E2E_EMAIL ?? "e2e@example.com";
const LEARNING_NOTES = "frontend/e2e/fixtures/learning_notes";

function git(args: string[]): void {
  execFileSync("git", args, { cwd: REPO_ROOT, stdio: "pipe" });
}

/**
 * Restore the seeded baseline: revert any learning_notes the app mutated, then
 * rebuild the database from that clean YAML. Fast (clear + import + seed, no
 * roundtrip diff). Throws with captured output on failure so a broken reset is
 * visible in CI rather than silently corrupting later scenarios.
 */
export function resetState(): void {
  // 1. Restore mutated + drop any newly-created learning-note YAML files.
  git(["checkout", "--", LEARNING_NOTES]);
  git(["clean", "-fdq", LEARNING_NOTES]);

  // 2. Rebuild the DB to the seeded baseline from the restored YAML.
  execFileSync("./langner-admin", ["migrate", "reset-db", "--config", CONFIG_PATH], {
    cwd: REPO_ROOT,
    stdio: "pipe",
    env: { ...process.env, DB_PASSWORD },
  });

  // 3. Re-provision auth accounts + notebook ownership the rebuild wiped (auth
  //    provisioning is decoupled from reset-db, so it must run explicitly).
  execFileSync("./langner-admin", ["auth", "provision", "--config", CONFIG_PATH], {
    cwd: REPO_ROOT,
    stdio: "pipe",
    env: { ...process.env, DB_PASSWORD },
  });

  // 4. Re-mint the allowlisted auth user the rebuild just wiped, so the
  //    pre-minted session cookie keeps resolving (upsert is idempotent; the
  //    re-inserted user reclaims id=1 after the sequence restart). We only need
  //    the user row — the printed cookie is discarded.
  execFileSync(
    "./langner-admin",
    ["auth", "issue-test-cookie", "--email", E2E_EMAIL, "--config", CONFIG_PATH],
    { cwd: REPO_ROOT, stdio: "pipe", env: { ...process.env, DB_PASSWORD } },
  );
}
