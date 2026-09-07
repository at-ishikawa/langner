#!/usr/bin/env bash
# Provision the e2e database, seed notebooks, mint the allowlisted user's signed
# session cookie, then exec the backend server — all BEFORE the port opens, so
# Playwright's "webServer ready" implies the seed is committed and the server
# can never serve /auth/me against an empty users table (the race that
# redirected every authenticated page to /login).
#
# This lives in a committed script instead of a long `&&`-joined string in
# playwright.config.ts on purpose: a single well-quoted script removes the
# shell-parsing fragility of a giant one-liner, `set -euo pipefail` makes any
# failed step fail the webServer loudly (Playwright surfaces it) rather than
# silently starting an unseeded server, and the `[seed]` markers make each step
# visible in CI logs. globalSetup then only turns the cookie file into
# Playwright storage state (see global-setup.ts).
set -euo pipefail

: "${DB_HOST:=127.0.0.1}"
: "${DB_PORT:=5432}"
: "${DB_USER:=postgres}"
: "${DB_PASSWORD:=password}"
: "${DB_NAME:=langner_e2e}"
: "${E2E_EMAIL:=e2e@example.com}"
: "${TEST_CONFIG_PATH:=config.e2e.yml}"
: "${COOKIE_FILE:=frontend/e2e/.auth/cookie.txt}"

# Run from the repo root (this script lives in frontend/e2e).
cd "$(dirname "$0")/../.."

echo "[seed] building langner + langner-server"
(cd backend && go build -o ../langner ./cmd/langner && go build -o ../langner-server ./cmd/langner-server)

echo "[seed] (re)creating database ${DB_NAME}"
PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS ${DB_NAME} WITH (FORCE)" \
  -c "CREATE DATABASE ${DB_NAME} ENCODING 'UTF8'"

echo "[seed] importing notebooks (migrate import-db)"
DB_PASSWORD="${DB_PASSWORD}" ./langner migrate import-db --config "${TEST_CONFIG_PATH}"

echo "[seed] minting e2e session cookie for ${E2E_EMAIL}"
mkdir -p "$(dirname "${COOKIE_FILE}")"
DB_PASSWORD="${DB_PASSWORD}" ./langner auth issue-test-cookie --email "${E2E_EMAIL}" --config "${TEST_CONFIG_PATH}" >"${COOKIE_FILE}"

# Hard guard: fail the webServer if the user row is not actually present, so the
# server never starts against an unseeded DB. This turns the old silent
# "0 users -> every /auth/me 401" failure into a loud, obvious seed failure.
echo "[seed] verifying the e2e user row exists"
count="$(PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -v ON_ERROR_STOP=1 -tAc "SELECT count(*) FROM users")"
if [ "${count}" -lt 1 ]; then
  echo "[seed] FATAL: users table is empty after issue-test-cookie (seed did not persist)" >&2
  exit 1
fi
echo "[seed] ok: ${count} user row(s); starting langner-server"

exec ./langner-server --config "${TEST_CONFIG_PATH}"
