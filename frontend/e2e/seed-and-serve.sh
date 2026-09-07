#!/usr/bin/env bash
# Provision the e2e database, seed notebooks, mint the allowlisted user's signed
# session cookie, then exec the backend server — all BEFORE the port opens, so
# Playwright's "webServer ready" implies the seed is committed and the server
# can never serve /auth/me against an empty users table (the race that
# redirected every authenticated page to /login).
#
# All diagnostics go to STDERR: Playwright captures the webServer's stderr but
# swallows its stdout, so `echo`/import-db summaries were invisible in CI. We
# redirect fd1->fd2 for the whole script and `set -x` so every step is legible
# in the CI log. `set -euo pipefail` fails the webServer loudly on any bad step
# instead of silently starting an unseeded server.
set -euo pipefail
exec 1>&2
set -x

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

export PGPASSWORD="${DB_PASSWORD}"
psql_e2e() { psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -v ON_ERROR_STOP=1 -tAc "$1"; }

echo "[seed] building langner + langner-server"
(cd backend && go build -o ../langner ./cmd/langner && go build -o ../langner-server ./cmd/langner-server)

# Count existing e2e users (0 if the DB or table doesn't exist yet). If a prior
# invocation already seeded, DON'T drop/reseed — a second DROP would wipe the
# user the first server is already using.
existing="$(psql_e2e 'SELECT count(*) FROM users' 2>/dev/null || echo 0)"
echo "[seed] existing user rows: ${existing}"

if [ "${existing}" -lt 1 ]; then
  echo "[seed] (re)creating database ${DB_NAME}"
  psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE IF EXISTS ${DB_NAME} WITH (FORCE)" \
    -c "CREATE DATABASE ${DB_NAME} ENCODING 'UTF8'"

  echo "[seed] importing notebooks (migrate import-db)"
  DB_PASSWORD="${DB_PASSWORD}" ./langner migrate import-db --config "${TEST_CONFIG_PATH}"

  echo "[seed] minting e2e session cookie for ${E2E_EMAIL}"
  mkdir -p "$(dirname "${COOKIE_FILE}")"
  DB_PASSWORD="${DB_PASSWORD}" ./langner auth issue-test-cookie --email "${E2E_EMAIL}" --config "${TEST_CONFIG_PATH}" >"${COOKIE_FILE}"
fi

# Hard guard: the server must NOT start unless the user row is really present.
count="$(psql_e2e 'SELECT count(*) FROM users')"
echo "[seed] users after seed: ${count}"
if [ "${count}" -lt 1 ]; then
  echo "[seed] FATAL: users table is empty after seeding — refusing to start an unseeded server"
  exit 1
fi
# Also require the cookie file so globalSetup can build storage state.
if [ ! -s "${COOKIE_FILE}" ]; then
  echo "[seed] FATAL: cookie file ${COOKIE_FILE} is missing/empty"
  exit 1
fi

echo "[seed] ok: ${count} user row(s); starting langner-server"
exec ./langner-server --config "${TEST_CONFIG_PATH}"
