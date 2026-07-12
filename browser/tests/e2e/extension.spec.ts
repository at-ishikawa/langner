import { test, expect } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync, existsSync } from "node:fs";
import { resolve, extname, dirname } from "node:path";
import { fileURLToPath } from "node:url";

// Serve the built extension output as a static site so Playwright can load
// popup.html in a normal browser context. This tests the built artifact,
// the popup UI, and state transitions without requiring the full Chrome
// extension loading flow (which needs a headed browser).

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DIST = resolve(__dirname, "../../.output/chrome-mv3");
const PORT = 9787;

const MIME: Record<string, string> = {
  ".html": "text/html",
  ".js": "application/javascript",
  ".css": "text/css",
  ".json": "application/json",
};

let server: Server;

test.beforeAll(async () => {
  if (!existsSync(DIST)) {
    throw new Error(
      `Built extension not found at ${DIST}. Run "pnpm build" before e2e tests.`,
    );
  }
  server = createServer((req, res) => {
    const url = req.url === "/" ? "/popup.html" : req.url ?? "/popup.html";
    const filePath = resolve(DIST, url.startsWith("/") ? url.slice(1) : url);
    if (!existsSync(filePath)) {
      res.writeHead(404);
      res.end("Not found");
      return;
    }
    const ext = extname(filePath);
    res.writeHead(200, { "Content-Type": MIME[ext] ?? "application/octet-stream" });
    res.end(readFileSync(filePath));
  });
  await new Promise<void>((resolve) => server.listen(PORT, resolve));
});

test.afterAll(async () => {
  await new Promise<void>((resolve) => server.close(() => resolve()));
});

test.describe("popup UI", () => {
  test("renders the initial capture view", async ({ page }) => {
    await page.goto(`http://localhost:${PORT}/popup.html`);

    // Title
    await expect(page.locator("h1")).toContainText("Langner Capture");

    // Description text
    await expect(page.locator("p")).toContainText("Capture subtitles");

    // Capture button is visible and enabled
    const btn = page.locator("#capture-btn");
    await expect(btn).toBeVisible();
    await expect(btn).toBeEnabled();
    await expect(btn).toHaveText("Capture Subtitles");
  });

  test("shows error when no subtitles are captured (no extension context)", async ({
    page,
  }) => {
    await page.goto(`http://localhost:${PORT}/popup.html`);

    const btn = page.locator("#capture-btn");
    await btn.click();

    // Outside extension context, chrome.tabs is not available.
    // The fallback returns empty cues, triggering the "no subtitles" error.
    const error = page.locator(".error");
    await expect(error).toBeVisible({ timeout: 5000 });
    await expect(error).toContainText("No subtitles captured");

    // The capture button should be re-enabled after the error.
    await expect(btn).toBeEnabled();
    await expect(btn).toHaveText("Capture Subtitles");
  });
});
