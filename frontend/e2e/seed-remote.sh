#!/usr/bin/env bash
# Seed a REMOTE database (the preview Supabase the PR's Vercel backend uses) for
# running the e2e suite against the deployed Vercel preview — the counterpart to
# seed-and-serve.sh, but it starts NO server (the server is the Vercel
# deployment) and does NOT create/drop the database (it is managed externally).
#
# It applies the schema, provisions the allowlisted e2e user, seeds the
# deterministic history + notebook content, and writes that user's bearer token
# to TOKEN_FILE — exactly the artifacts the Playwright preview run needs
# (common.ts injects the token; the content/history back the scenarios).
#
# DB coordinates come from the DB_* env (bound over config.e2e.yml by the
# config-from-env layer), so point them at the preview Supabase. All output to
# stderr + `set -x` so CI logs are legible.
set -euo pipefail
exec 1>&2
set -x

: "${TEST_CONFIG_PATH:=config.e2e.yml}"
: "${E2E_EMAIL:=e2e@example.com}"
: "${TOKEN_FILE:=frontend/e2e/.auth/access-token.txt}"

# Run from the repo root (this script lives in frontend/e2e).
cd "$(dirname "$0")/../.."

echo "[seed-remote] building langner-admin"
(cd backend && go build -o ../langner-admin ./cmd/langner-admin)

echo "[seed-remote] applying schema (idempotent)"
./langner-admin migrate schema --config "${TEST_CONFIG_PATH}"

echo "[seed-remote] provisioning auth accounts"
./langner-admin auth provision --config "${TEST_CONFIG_PATH}"

# seed-e2e imports the notebook content into notebook_files AND seeds the
# deterministic learning history. reset-e2e normalises STATE on a reused DB
# (content persists); run it so re-runs start from the same baseline.
echo "[seed-remote] seeding content + history"
./langner-admin migrate seed-e2e --config "${TEST_CONFIG_PATH}"
./langner-admin migrate reset-e2e --config "${TEST_CONFIG_PATH}" || true

echo "[seed-remote] minting e2e access token for ${E2E_EMAIL}"
mkdir -p "$(dirname "${TOKEN_FILE}")"
./langner-admin auth issue-test-token --email "${E2E_EMAIL}" --config "${TEST_CONFIG_PATH}" >"${TOKEN_FILE}"
test -s "${TOKEN_FILE}" || { echo "[seed-remote] FATAL: token file empty"; exit 1; }
echo "[seed-remote] done"
