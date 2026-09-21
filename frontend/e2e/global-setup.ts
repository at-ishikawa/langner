// Playwright globalSetup: wait for the backend seed to mint the e2e access
// token before any spec runs.
//
// Auth is a bearer access token held in the SPA's memory — there is no cookie,
// so nothing goes into Playwright `storageState`. Instead each scenario injects
// the token into `window.__LANGNER_ACCESS_TOKEN__` before the app loads (see
// e2e/steps/common.ts), the same seam the running app reads. This setup only
// ensures the token file the backend webServer wrote exists (auth is enabled in
// config.e2e.yml, so every RPC is gated behind a bearer).
//
// Playwright starts the webServers and awaits their readiness BEFORE running
// this globalSetup; the backend webServer command (seed-and-serve.sh) mints the
// token to access-token.txt before it binds its port, so by the time this runs
// the file exists. The order between globalSetup and the webServer is
// version-dependent, so we poll rather than assume.

import { readFileSync } from "node:fs";
import { join } from "node:path";

// Token file written by the backend webServer command; also read per-scenario
// by e2e/steps/common.ts.
export const TOKEN_PATH = join(__dirname, ".auth", "access-token.txt");

const TOKEN_WAIT_MS = 180_000;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// readToken returns the minted access token, or "" if the file is not present
// yet. Tolerate a stray leading log line: take the last non-empty line.
export function readToken(): string {
  try {
    return (
      readFileSync(TOKEN_PATH, "utf8")
        .split("\n")
        .map((line) => line.trim())
        .filter((line) => line !== "")
        .pop() ?? ""
    );
  } catch {
    return "";
  }
}

export default async function globalSetup() {
  let token = readToken();
  const deadline = Date.now() + TOKEN_WAIT_MS;
  while (!token && Date.now() < deadline) {
    await sleep(500);
    token = readToken();
  }

  if (!token) {
    throw new Error(
      `no e2e access token in ${TOKEN_PATH} after ${TOKEN_WAIT_MS}ms; the backend webServer seed step did not write it`,
    );
  }
}
