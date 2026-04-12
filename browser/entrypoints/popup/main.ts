import { scrapeYouTubePage } from "../../src/youtube/scraper.js";
import { buildStoryNotebook } from "../../src/notebook/builder.js";
import { serializeStoryNotebooks } from "../../src/notebook/yaml.js";
import type { ScrapedVideo, StoryNotebook } from "../../src/types.js";

// DOM references
const app = document.getElementById("app")!;

// State
let currentNotebook: StoryNotebook | null = null;
let currentYaml: string = "";

function render() {
  app.innerHTML = "";
  app.appendChild(createInitialView());
}

function createInitialView(): HTMLElement {
  const div = document.createElement("div");
  div.innerHTML = `
    <h1><span class="logo">L</span> Langner Capture</h1>
    <p style="color: var(--muted); margin-bottom: 12px; font-size: 13px;">
      Capture this YouTube video's transcript as a Langner story notebook.
    </p>
    <button id="capture-btn" class="primary">Capture Transcript</button>
    <div id="message"></div>
  `;

  const btn = div.querySelector("#capture-btn") as HTMLButtonElement;
  btn.addEventListener("click", handleCapture);

  return div;
}

async function handleCapture() {
  const btn = document.getElementById("capture-btn") as HTMLButtonElement;
  const msgDiv = document.getElementById("message")!;
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Capturing...';
  msgDiv.innerHTML = "";

  try {
    const video = await scrapeActiveTab();
    if (video.cues.length === 0) {
      msgDiv.innerHTML = `<div class="error">
        No transcript found on this page. Make sure you are on a YouTube video
        and that the transcript panel is open (click the "..." menu below the video,
        then "Show transcript").
      </div>`;
      btn.disabled = false;
      btn.textContent = "Capture Transcript";
      return;
    }

    currentNotebook = buildStoryNotebook(video);
    currentYaml = serializeStoryNotebooks([currentNotebook]);
    showPreview(video, currentNotebook);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    msgDiv.innerHTML = `<div class="error">${escapeHtml(message)}</div>`;
    btn.disabled = false;
    btn.textContent = "Capture Transcript";
  }
}

function showPreview(video: ScrapedVideo, notebook: StoryNotebook) {
  const totalCues = video.cues.length;
  const totalScenes = notebook.scenes.length;

  app.innerHTML = `
    <h1><span class="logo">L</span> Captured</h1>
    <div class="preview-header">
      <div class="title">${escapeHtml(video.title)}</div>
      <div class="meta">${escapeHtml(video.channel)}</div>
    </div>
    <div class="stats">
      <span><strong>${totalScenes}</strong> scene${totalScenes !== 1 ? "s" : ""}</span>
      <span><strong>${totalCues}</strong> caption${totalCues !== 1 ? "s" : ""}</span>
      ${video.chapters.length > 0 ? `<span><strong>${video.chapters.length}</strong> chapter${video.chapters.length !== 1 ? "s" : ""}</span>` : ""}
    </div>
    <div class="scenes-list" id="scenes-list"></div>
    <div class="button-row">
      <button id="download-btn" class="primary">Download YAML</button>
      <button id="copy-btn" class="secondary">Copy</button>
    </div>
    <div id="message"></div>
  `;

  const list = document.getElementById("scenes-list")!;
  for (const scene of notebook.scenes) {
    const item = document.createElement("div");
    item.className = "scene-item";
    item.innerHTML = `
      <div class="scene-title">${escapeHtml(scene.scene)}</div>
      <div class="scene-detail">${scene.conversations.length} line${scene.conversations.length !== 1 ? "s" : ""}</div>
    `;
    list.appendChild(item);
  }

  document.getElementById("download-btn")!.addEventListener("click", handleDownload);
  document.getElementById("copy-btn")!.addEventListener("click", handleCopy);
}

function handleDownload() {
  if (!currentYaml || !currentNotebook) return;

  const blob = new Blob([currentYaml], { type: "text/yaml" });
  const url = URL.createObjectURL(blob);
  const filename = sanitizeFilename(currentNotebook.event) + ".yml";

  // Use chrome.downloads if available (extension context), fallback for tests
  if (typeof chrome !== "undefined" && chrome.downloads) {
    chrome.downloads.download({ url, filename, saveAs: true });
  } else {
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  const msgDiv = document.getElementById("message")!;
  msgDiv.innerHTML = '<div class="success">YAML file download started.</div>';
}

async function handleCopy() {
  if (!currentYaml) return;

  try {
    await navigator.clipboard.writeText(currentYaml);
    const msgDiv = document.getElementById("message")!;
    msgDiv.innerHTML = '<div class="success">Copied to clipboard.</div>';
  } catch {
    const msgDiv = document.getElementById("message")!;
    msgDiv.innerHTML = '<div class="error">Failed to copy to clipboard.</div>';
  }
}

async function scrapeActiveTab(): Promise<ScrapedVideo> {
  // In a real extension, use chrome.scripting.executeScript to run the
  // scraper in the active tab. For the PoC we check if the API exists.
  if (typeof chrome !== "undefined" && chrome.tabs && chrome.scripting) {
    const [tab] = await chrome.tabs.query({
      active: true,
      currentWindow: true,
    });
    if (!tab?.id || !tab.url) {
      throw new Error("No active tab found.");
    }
    if (!tab.url.includes("youtube.com/watch")) {
      throw new Error("The active tab is not a YouTube video page.");
    }

    const results = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: scrapeInPage,
    });

    if (!results || results.length === 0 || !results[0]?.result) {
      throw new Error("Failed to run capture script in the page.");
    }
    return results[0].result as ScrapedVideo;
  }

  // Fallback: if running outside extension context (e.g. testing in a
  // regular browser tab), try to scrape the current page directly.
  return scrapeYouTubePage(document, window.location.href);
}

// This function is serialized and injected into the YouTube tab via
// chrome.scripting.executeScript. It cannot reference closures — all
// logic must be self-contained.
function scrapeInPage(): ScrapedVideo {
  function parseTs(ts: string): number {
    const parts = ts.split(":").map((p) => parseInt(p, 10)).filter((n) => !isNaN(n));
    if (parts.length === 0) return 0;
    if (parts.length === 1) return parts[0]! * 1000;
    if (parts.length === 2) return (parts[0]! * 60 + parts[1]!) * 1000;
    return (parts[0]! * 3600 + parts[1]! * 60 + parts[2]!) * 1000;
  }

  const titleEl = document.querySelector("ytd-watch-metadata #title h1");
  const title = titleEl?.textContent?.trim() ??
    (document.querySelector("title")?.textContent?.trim() ?? "").replace(/\s*-\s*YouTube\s*$/i, "");

  const channel = document.querySelector("ytd-video-owner-renderer #channel-name a")?.textContent?.trim() ?? "";
  const url = window.location.href;

  let videoId = "";
  try { videoId = new URL(url).searchParams.get("v") ?? ""; } catch { /* noop */ }

  const cues: Array<{ offsetMs: number; text: string }> = [];
  for (const seg of document.querySelectorAll("ytd-transcript-segment-renderer")) {
    const ts = seg.querySelector(".segment-timestamp")?.textContent?.trim() ?? "";
    const txt = seg.querySelector(".segment-text")?.textContent?.trim() ?? "";
    if (txt) cues.push({ offsetMs: parseTs(ts), text: txt });
  }

  const chapters: Array<{ startMs: number; title: string }> = [];
  for (const item of document.querySelectorAll("ytd-macro-markers-list-item-renderer")) {
    const t = item.querySelector('[id="title"]')?.textContent?.trim() ?? "";
    const tm = item.querySelector('[id="time"]')?.textContent?.trim() ?? "";
    if (t || tm) chapters.push({ startMs: parseTs(tm), title: t });
  }

  return { videoId, title, channel, url, cues, chapters };
}

function escapeHtml(str: string): string {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

function sanitizeFilename(str: string): string {
  return str
    .replace(/[^a-zA-Z0-9_\- ]/g, "")
    .replace(/\s+/g, "-")
    .toLowerCase()
    .slice(0, 80) || "notebook";
}

// Boot
render();
