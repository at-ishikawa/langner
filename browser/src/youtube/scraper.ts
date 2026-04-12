import type { Chapter, ScrapedVideo, TranscriptCue } from "../types.js";

// Scrapes video metadata, transcript cues, and chapter markers from the
// YouTube page DOM.
//
// The function is designed to run inside a content script or via
// `chrome.scripting.executeScript` — it takes a Document (normally `document`)
// and returns the structured data. Tests inject a jsdom document instead.
//
// YouTube's DOM structure changes occasionally. If a selector stops matching,
// the function degrades gracefully: fields default to empty strings / empty
// arrays and the caller decides how to handle missing data.
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

function extractTranscriptCues(doc: Document): TranscriptCue[] {
  const segments = doc.querySelectorAll("ytd-transcript-segment-renderer");
  const cues: TranscriptCue[] = [];

  for (const seg of segments) {
    const timestampEl = seg.querySelector(".segment-timestamp");
    const textEl = seg.querySelector(".segment-text");
    if (!timestampEl || !textEl) {
      continue;
    }

    const timestamp = timestampEl.textContent?.trim() ?? "";
    const text = textEl.textContent?.trim() ?? "";
    if (!text) {
      continue;
    }

    cues.push({
      offsetMs: parseTimestamp(timestamp),
      text,
    });
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
