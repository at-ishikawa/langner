import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { createBdd } from "playwright-bdd";

const { Given, When, Then, AfterScenario } = createBdd();

// The sign-in scenarios need the e2e mock OAuth provider, which only the local
// stack runs; the deployed preview signs in with real Google.
const skipOnPreview = () =>
  test.skip(Boolean(process.env.PLAYWRIGHT_BASE_URL), "needs the local mock OAuth provider");

// The `langner` CLI binary built by e2e/seed-and-serve.sh at the repo root, and
// the API it talks to (the SPA's API host, see playwright.config.ts).
const CLI_PATH = join(__dirname, "..", "..", "..", "langner");
const CLI_SERVER = "http://127.0.0.1:8080";

interface CLILogin {
  process: ChildProcess;
  configDir: string;
  output: string;
  exitCode: Promise<number | null>;
}

let cliLogin: CLILogin | undefined;

AfterScenario(() => {
  if (!cliLogin) return;
  cliLogin.process.kill();
  rmSync(cliLogin.configDir, { recursive: true, force: true });
  cliLogin = undefined;
});

// cliEnv isolates the CLI's credential store in a temp dir so the scenario
// never touches the developer's ~/.config/langner.
function cliEnv(configDir: string): NodeJS.ProcessEnv {
  return { ...process.env, XDG_CONFIG_HOME: configDir, LANGNER_SERVER: "", LANGNER_TOKEN: "" };
}

// covers route: /login — the Google sign-in entry point. The SessionProvider
// guard treats /login as public: it never silently re-authenticates away from
// /login, so this renders even under the injected access token.
Given("I am on the login page", async ({ page }) => {
  await page.goto("/login");
});

// Undo the access token common.ts injects for every scenario, so the app only
// becomes signed in through the sign-in flow itself. Init scripts run in the
// order they were added, so this one runs after the injection.
Given("I am signed out on the login page", async ({ page }) => {
  skipOnPreview();
  await page.addInitScript(() => {
    delete (window as unknown as { __LANGNER_ACCESS_TOKEN__?: string }).__LANGNER_ACCESS_TOKEN__;
  });
  await page.goto("/login");
});

Then("I see the {string} button", async ({ page }, name: string) => {
  await expect(page.getByRole("button", { name })).toBeVisible();
});

When("I sign in with Google", async ({ page }) => {
  await page.getByRole("button", { name: "Sign in with Google" }).click();
});

Then("I am signed in", async ({ page }) => {
  await expect(page).toHaveURL((url) => url.pathname === "/");
  await expect(page.getByRole("link", { name: "Quiz" }).first()).toBeVisible();
});

Given("I started {string} in a terminal", async ({}, command: string) => {
  skipOnPreview();
  expect(command).toBe("langner login");
  const configDir = mkdtempSync(join(tmpdir(), "langner-cli-"));
  const child = spawn(CLI_PATH, ["login", "--server", CLI_SERVER], { env: cliEnv(configDir) });
  const login: CLILogin = {
    process: child,
    configDir,
    output: "",
    exitCode: new Promise((resolve) => child.on("exit", resolve)),
  };
  child.stdout?.on("data", (chunk) => (login.output += chunk));
  child.stderr?.on("data", (chunk) => (login.output += chunk));
  cliLogin = login;
  await expect.poll(() => login.output, { timeout: 15_000 }).toMatch(/enter code:\s+\S+/);
});

When("I open the printed sign-in page and approve the code", async ({ page }) => {
  const output = cliLogin?.output ?? "";
  const verifyURL = output.match(/To sign in, open:\s+(\S+)/)?.[1];
  const userCode = output.match(/enter code:\s+(\S+)/)?.[1];
  expect(verifyURL, output).toBeTruthy();
  expect(userCode, output).toBeTruthy();

  await page.goto(verifyURL!);
  await page.getByPlaceholder("XXXX-XXXX").fill(userCode!);
  await page.getByRole("button", { name: "Approve & sign in with Google" }).click();
});

Then("the CLI is logged in", async () => {
  const login = cliLogin!;
  // The CLI polls every 5s (the server's device poll interval).
  const exitCode = await Promise.race([
    login.exitCode,
    new Promise<"timeout">((resolve) => setTimeout(() => resolve("timeout"), 30_000)),
  ]);
  expect(exitCode, login.output).toBe(0);
  expect(login.output).toContain("Logged in");

  const whoami = spawnSync(CLI_PATH, ["whoami", "--server", CLI_SERVER], {
    env: cliEnv(login.configDir),
    encoding: "utf8",
  });
  expect(whoami.status, whoami.stdout + whoami.stderr).toBe(0);
});
