import type { Chapter, ScrapedVideo, TranscriptCue } from "../types.js";

// Scrapes video metadata, transcript cues, and chapter markers from the
// YouTube page DOM.
//
// The function is designed to run inside a content script or via
// `chrome.scripting.executeScript` — it takes a Document (normally `document`)
// and returns the structured data. Tests inject a jsdom document instead.
//
// YouTube's DOM structure changes frequently. The scraper tries multiple
// selector strategies (newest first, then older fallbacks) so it keeps working
// across YouTube updates. If every strategy fails, fields default to empty
// strings / empty arrays and the caller decides how to handle missing data.
export function scrapeYouTubePage(doc: Document, pageUrl: string): ScrapedVideo {
  return {
    videoId: extractVideoId(pageUrl),
    title: extractTitle(doc),
    channel: extractChannel(doc),
    url: pageUrl,
    cues: extractTranscriptCues(doc),
    chapters: extractChapters(doc),
  };
}

function extractVideoId(url: string): string {
  try {
    const u = new URL(url);
    return u.searchParams.get("v") ?? "";
  } catch {
    return "";
  }
}

function extractTitle(doc: Document): string {
  // Primary: the title inside the metadata section
  const meta = doc.querySelector("ytd-watch-metadata #title h1");
  if (meta?.textContent?.trim()) {
    return meta.textContent.trim();
  }
  // Fallback: page <title>, which YouTube sets to "Video Title - YouTube"
  const pageTitle = doc.querySelector("title")?.textContent?.trim() ?? "";
  return pageTitle.replace(/\s*-\s*YouTube\s*$/i, "");
}

function extractChannel(doc: Document): string {
  const el = doc.querySelector("ytd-video-owner-renderer #channel-name a");
  return el?.textContent?.trim() ?? "";
}

// Tries multiple selector strategies for transcript cues. YouTube changes its
// DOM structure regularly, so we try the most common current layouts first and
// fall back to older ones.
function extractTranscriptCues(doc: Document): TranscriptCue[] {
  // Strategy 1: #segments-container (current YouTube layout as of 2025-2026).
  // The transcript panel renders segments inside a container with this ID.
  const container = doc.querySelector("#segments-container");
  if (container) {
    // 1a: <ytd-transcript-segment-renderer> inside the container.
    //     Each segment has a .segment-timestamp and .segment-text child.
    const renderers = container.querySelectorAll("ytd-transcript-segment-renderer");
    if (renderers.length > 0) {
      const cues = extractFromRenderers(renderers);
      if (cues.length > 0) return cues;
    }

    // 1b: <transcript-segment-view-model> (newer variant seen March 2026).
    const viewModels = container.querySelectorAll("transcript-segment-view-model");
    if (viewModels.length > 0) {
      const cues = extractFromRenderers(viewModels);
      if (cues.length > 0) return cues;
    }

    // 1c: Flat list of yt-formatted-string elements directly in the container.
    //     Some YouTube builds put timestamps and text as alternating
    //     yt-formatted-string children.
    const cues = extractFromFlatFormattedStrings(container);
    if (cues.length > 0) return cues;
  }

  // Strategy 2: <ytd-transcript-segment-renderer> outside #segments-container
  // (older layout or different page structure).
  const globalRenderers = doc.querySelectorAll("ytd-transcript-segment-renderer");
  if (globalRenderers.length > 0) {
    const cues = extractFromRenderers(globalRenderers);
    if (cues.length > 0) return cues;
  }

  return [];
}

// Extracts cues from segment wrapper elements (ytd-transcript-segment-renderer
// or transcript-segment-view-model). Each wrapper is expected to contain a
// timestamp element and a text element. We try multiple selectors for each.
function extractFromRenderers(segments: NodeListOf<Element>): TranscriptCue[] {
  const cues: TranscriptCue[] = [];

  for (const seg of segments) {
    const timestamp = findTimestamp(seg);
    const text = findText(seg);
    if (!text) continue;

    cues.push({
      offsetMs: parseTimestamp(timestamp),
      text,
    });
  }

  return cues;
}

// Tries multiple selectors for the timestamp element inside a segment.
function findTimestamp(seg: Element): string {
  const selectors = [
    ".segment-timestamp",
    "[class*='timestamp']",
    "div.segment-start-offset",
  ];
  for (const sel of selectors) {
    const el = seg.querySelector(sel);
    if (el?.textContent?.trim()) {
      return el.textContent.trim();
    }
  }
  return "";
}

// Tries multiple selectors for the text element inside a segment.
function findText(seg: Element): string {
  const selectors = [
    ".segment-text",
    "yt-formatted-string.segment-text",
    ".yt-core-attributed-string",
    "yt-formatted-string:not([class*='timestamp'])",
  ];
  for (const sel of selectors) {
    const el = seg.querySelector(sel);
    const text = el?.textContent?.trim();
    if (text) return text;
  }
  return "";
}

// Fallback: extract cues from a flat list of yt-formatted-string elements
// inside a container. YouTube sometimes renders timestamps and text as
// alternating <yt-formatted-string> children. We detect timestamps by
// matching the mm:ss or h:mm:ss pattern.
function extractFromFlatFormattedStrings(container: Element): TranscriptCue[] {
  const elements = container.querySelectorAll("yt-formatted-string");
  if (elements.length === 0) return [];

  const timestampPattern = /^\d{1,2}:\d{2}(:\d{2})?$/;
  const cues: TranscriptCue[] = [];
  let pendingTimestamp = "";

  for (const el of elements) {
    const text = el.textContent?.trim() ?? "";
    if (!text) continue;

    if (timestampPattern.test(text)) {
      pendingTimestamp = text;
    } else if (pendingTimestamp) {
      cues.push({ offsetMs: parseTimestamp(pendingTimestamp), text });
      pendingTimestamp = "";
    } else {
      // Text without a preceding timestamp — still useful
      cues.push({ offsetMs: 0, text });
    }
  }

  return cues;
}

function extractChapters(doc: Document): Chapter[] {
  const items = doc.querySelectorAll("ytd-macro-markers-list-item-renderer");
  const chapters: Chapter[] = [];

  for (const item of items) {
    // YouTube's DOM contains duplicate id="title" across many elements.
    // CSS ID selectors (#title) are unreliable when IDs are duplicated, so
    // we use attribute selectors instead.
    const titleEl = item.querySelector('[id="title"]');
    const timeEl = item.querySelector('[id="time"]');
    if (!titleEl || !timeEl) {
      continue;
    }

    const title = titleEl.textContent?.trim() ?? "";
    const time = timeEl.textContent?.trim() ?? "";

    chapters.push({
      startMs: parseTimestamp(time),
      title,
    });
  }

  return chapters;
}

// Parses a YouTube timestamp string ("1:23", "1:02:30") into milliseconds.
export function parseTimestamp(ts: string): number {
  const parts = ts.split(":").map((p) => parseInt(p, 10));
  // Filter out NaN from malformed input
  const valid = parts.filter((n) => !isNaN(n));
  if (valid.length === 0) {
    return 0;
  }

  let seconds = 0;
  // [ss], [mm, ss], [hh, mm, ss]
  if (valid.length === 1) {
    seconds = valid[0]!;
  } else if (valid.length === 2) {
    seconds = valid[0]! * 60 + valid[1]!;
  } else {
    seconds = valid[0]! * 3600 + valid[1]! * 60 + valid[2]!;
  }

  return seconds * 1000;
}
