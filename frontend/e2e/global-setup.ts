// Playwright globalSetup: translate the e2e session cookie into Playwright
// storage state so every spec runs authenticated.
//
// Playwright starts the webServers and awaits their readiness BEFORE running
// this globalSetup, so we cannot provision the database here — the backend
// server would already need it. Instead the backend webServer command
// (playwright.config.ts) (re)creates the test DB, imports notebooks, upserts
// the allowlisted e2e user and writes that user's signed session cookie to
// cookie.txt, all before it binds its port. By the time this runs, the server
// is ready, so the cookie file exists. We only read it and inject the cookie
// via `use.storageState` (auth is enabled in config.e2e.yml, so all RPCs are
// gated behind a session cookie).

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

// Cookie file written by the backend webServer command before it starts.
const COOKIE_PATH = join(__dirname, ".auth", "cookie.txt");
// Playwright storage state file written by this setup and consumed via
// `use.storageState` in playwright.config.ts.
export const STORAGE_STATE_PATH = join(__dirname, ".auth", "storageState.json");

export default async function globalSetup() {
  // The backend command writes only the cookie to stdout, but tolerate any
  // stray leading log line: take the last non-empty line.
  const cookieValue = readFileSync(COOKIE_PATH, "utf8")
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "")
    .pop();

  if (!cookieValue) {
    throw new Error(
      `no e2e session cookie in ${COOKIE_PATH}; the backend webServer seed step must run before globalSetup`,
    );
  }

  const storageState = {
    cookies: [
      {
        name: "langner_session",
        value: cookieValue,
        // Host-only "localhost" cookie: applies to both the frontend (3100)
        // and backend (8080) since cookies ignore the port.
        domain: "localhost",
        path: "/",
        expires: Math.floor(Date.now() / 1000) + 30 * 24 * 60 * 60,
        httpOnly: true,
        secure: false,
        sameSite: "Lax" as const,
      },
    ],
    origins: [],
  };
  mkdirSync(join(__dirname, ".auth"), { recursive: true });
  writeFileSync(STORAGE_STATE_PATH, JSON.stringify(storageState, null, 2));
}
