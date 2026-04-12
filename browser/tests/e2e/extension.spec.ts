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
    await expect(page.locator("p")).toContainText(
      "Capture this YouTube video",
    );

    // Capture button is visible and enabled
    const btn = page.locator("#capture-btn");
    await expect(btn).toBeVisible();
    await expect(btn).toBeEnabled();
    await expect(btn).toHaveText("Capture Transcript");
  });

  test("shows 'no transcript' error when capturing a non-YouTube page", async ({
    page,
  }) => {
    await page.goto(`http://localhost:${PORT}/popup.html`);

    const btn = page.locator("#capture-btn");
    await btn.click();

    // The popup falls back to scraping the current document (the popup
    // itself), which has no transcript elements. It should show the "no
    // transcript" error.
    const error = page.locator(".error");
    await expect(error).toBeVisible({ timeout: 5000 });
    await expect(error).toContainText("No transcript found");

    // The capture button should be re-enabled after the error.
    await expect(btn).toBeEnabled();
    await expect(btn).toHaveText("Capture Transcript");
  });

  test("shows preview and download button when transcript is present", async ({
    page,
  }) => {
    // Serve a page that has YouTube fixture content embedded.
    // We'll inject the fixture HTML into the page before clicking capture.
    await page.goto(`http://localhost:${PORT}/popup.html`);

    // Inject YouTube fixture DOM elements into the page
    await page.evaluate(() => {
      // Add a fake ytd-watch-metadata element
      const meta = document.createElement("ytd-watch-metadata");
      meta.innerHTML =
        '<div id="title"><h1><yt-formatted-string>Test Video Title</yt-formatted-string></h1></div>';
      document.body.appendChild(meta);

      // Add channel
      const owner = document.createElement("ytd-video-owner-renderer");
      owner.innerHTML =
        '<div id="channel-name"><a>Test Channel</a></div>';
      document.body.appendChild(owner);

      // Add transcript segments
      const transcript = document.createElement("ytd-transcript-renderer");
      const list = document.createElement(
        "ytd-transcript-segment-list-renderer",
      );
      for (let i = 0; i < 3; i++) {
        const seg = document.createElement(
          "ytd-transcript-segment-renderer",
        );
        seg.innerHTML = `
          <div class="segment-timestamp">${i}:00</div>
          <yt-formatted-string class="segment-text">Caption line ${i + 1}.</yt-formatted-string>
        `;
        list.appendChild(seg);
      }
      transcript.appendChild(list);
      document.body.appendChild(transcript);
    });

    // Click capture — the fallback path scrapes the current document
    await page.locator("#capture-btn").click();

    // Preview should appear
    await expect(page.locator(".preview-header .title")).toBeVisible({
      timeout: 5000,
    });
    await expect(page.locator(".preview-header .title")).toHaveText(
      "Test Video Title",
    );
    await expect(page.locator(".preview-header .meta")).toHaveText(
      "Test Channel",
    );

    // Stats should show scene and caption counts
    await expect(page.locator(".stats")).toContainText("scene");
    await expect(page.locator(".stats")).toContainText("3 captions");

    // Scenes list should be present
    const sceneItems = page.locator(".scene-item");
    await expect(sceneItems.first()).toBeVisible();

    // Download and Copy buttons should be visible
    await expect(page.locator("#download-btn")).toBeVisible();
    await expect(page.locator("#copy-btn")).toBeVisible();
  });

  test("copy button copies YAML to clipboard", async ({ page, context }) => {
    // Grant clipboard permissions
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);

    await page.goto(`http://localhost:${PORT}/popup.html`);

    // Inject minimal fixture content
    await page.evaluate(() => {
      const meta = document.createElement("ytd-watch-metadata");
      meta.innerHTML =
        '<div id="title"><h1><yt-formatted-string>Clipboard Test</yt-formatted-string></h1></div>';
      document.body.appendChild(meta);

      const transcript = document.createElement("ytd-transcript-renderer");
      const list = document.createElement(
        "ytd-transcript-segment-list-renderer",
      );
      const seg = document.createElement("ytd-transcript-segment-renderer");
      seg.innerHTML = `
        <div class="segment-timestamp">0:00</div>
        <yt-formatted-string class="segment-text">Hello world.</yt-formatted-string>
      `;
      list.appendChild(seg);
      transcript.appendChild(list);
      document.body.appendChild(transcript);
    });

    await page.locator("#capture-btn").click();
    await expect(page.locator("#copy-btn")).toBeVisible({ timeout: 5000 });

    await page.locator("#copy-btn").click();

    // Verify success message
    await expect(page.locator(".success")).toContainText("Copied to clipboard");

    // Verify clipboard content is valid YAML
    const clipboardText = await page.evaluate(() =>
      navigator.clipboard.readText(),
    );
    expect(clipboardText).toContain("event:");
    expect(clipboardText).toContain("Clipboard Test");
    expect(clipboardText).toContain("Hello world.");
    expect(clipboardText).toContain("scenes:");
  });
});
