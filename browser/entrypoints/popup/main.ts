import { parseSubtitle } from "../../src/subtitle/parsers.js";
import { buildStoryNotebook } from "../../src/notebook/builder.js";
import { serializeStoryNotebooks } from "../../src/notebook/yaml.js";
import type { ScrapedVideo, StoryNotebook, TranscriptCue } from "../../src/types.js";

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
      Capture subtitles from the current video as a Langner story notebook.
      Make sure captions are enabled on the video.
    </p>
    <button id="capture-btn" class="primary">Capture Subtitles</button>
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
    const video = await getVideoData();
    if (video.cues.length === 0) {
      msgDiv.innerHTML = `<div class="error">
        No subtitles captured. Make sure:<br>
        1. You are on a video page (YouTube, Netflix, etc.)<br>
        2. Captions/subtitles are turned on<br>
        3. The video has played at least a few seconds with captions visible<br>
        <br>
        The extension captures subtitle data as it loads. Try refreshing the page,
        enabling captions, and then clicking Capture again.
      </div>`;
      btn.disabled = false;
      btn.textContent = "Capture Subtitles";
      return;
    }

    currentNotebook = buildStoryNotebook(video);
    currentYaml = serializeStoryNotebooks([currentNotebook]);
    showPreview(video, currentNotebook);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    msgDiv.innerHTML = `<div class="error">${escapeHtml(message)}</div>`;
    btn.disabled = false;
    btn.textContent = "Capture Subtitles";
  }
}

function showPreview(video: ScrapedVideo, notebook: StoryNotebook) {
  const totalCues = video.cues.length;
  const totalScenes = notebook.scenes.length;

  app.innerHTML = `
    <h1><span class="logo">L</span> Captured</h1>
    <div class="preview-header">
      <div class="title">${escapeHtml(video.title || "Untitled Video")}</div>
      <div class="meta">${escapeHtml(video.channel || "Unknown channel")}</div>
    </div>
    <div class="stats">
      <span><strong>${totalScenes}</strong> scene${totalScenes !== 1 ? "s" : ""}</span>
      <span><strong>${totalCues}</strong> caption${totalCues !== 1 ? "s" : ""}</span>
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

// Gets video data by messaging the content script for intercepted subtitles
// and page metadata.
async function getVideoData(): Promise<ScrapedVideo> {
  if (typeof chrome === "undefined" || !chrome.tabs) {
    // Fallback for testing outside extension context
    return {
      videoId: "",
      title: document.title,
      channel: "",
      url: window.location.href,
      cues: [],
      chapters: [],
    };
  }

  const [tab] = await chrome.tabs.query({
    active: true,
    currentWindow: true,
  });
  if (!tab?.id) {
    throw new Error("No active tab found.");
  }

  // Get captured subtitles from the content script
  const subtitleResponse = await chrome.tabs.sendMessage(tab.id, {
    type: "GET_CAPTURED_SUBTITLES",
  });

  // Get page metadata from the content script
  const metaResponse = await chrome.tabs.sendMessage(tab.id, {
    type: "GET_PAGE_METADATA",
  });

  // Parse all captured subtitle responses into cues
  const allCues: TranscriptCue[] = [];
  if (subtitleResponse?.subtitles) {
    for (const sub of subtitleResponse.subtitles) {
      const cues = parseSubtitle(sub.content, sub.url);
      if (cues.length > allCues.length) {
        // Use the subtitle response that yielded the most cues
        // (there may be multiple languages; pick the longest)
        allCues.length = 0;
        allCues.push(...cues);
      }
    }
  }

  let videoId = "";
  try {
    videoId = new URL(metaResponse?.url ?? "").searchParams.get("v") ?? "";
  } catch { /* noop */ }

  return {
    videoId,
    title: metaResponse?.title ?? "",
    channel: metaResponse?.channel ?? "",
    url: metaResponse?.url ?? tab.url ?? "",
    cues: allCues,
    chapters: [],
  };
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
