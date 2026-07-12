import type { TranscriptCue } from "../types.js";

// Detects the subtitle format from content and parses it into cues.
export function parseSubtitle(content: string, url: string): TranscriptCue[] {
  const format = detectFormat(content, url);
  switch (format) {
    case "webvtt":
      return parseWebVTT(content);
    case "ttml":
      return parseTTML(content);
    case "youtube-json":
      return parseYouTubeJSON(content);
    default:
      return [];
  }
}

export type SubtitleFormat = "webvtt" | "ttml" | "youtube-json" | "unknown";

export function detectFormat(content: string, url: string): SubtitleFormat {
  const trimmed = content.trimStart();

  // WebVTT starts with "WEBVTT"
  if (trimmed.startsWith("WEBVTT")) {
    return "webvtt";
  }

  // TTML is XML with <tt> root element
  if (trimmed.startsWith("<?xml") || trimmed.startsWith("<tt")) {
    return "ttml";
  }

  // YouTube JSON — try parsing as JSON and check for known structure
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    try {
      const parsed = JSON.parse(content);
      // YouTube timedtext JSON has "events" array
      if (parsed.events && Array.isArray(parsed.events)) {
        return "youtube-json";
      }
      // YouTube transcript API response has "actions" array
      if (parsed.actions && Array.isArray(parsed.actions)) {
        return "youtube-json";
      }
    } catch {
      // not valid JSON
    }
  }

  // URL-based fallback
  if (url.includes(".vtt") || url.includes("fmt=vtt")) {
    return "webvtt";
  }
  if (url.includes(".ttml") || url.includes("fmt=ttml")) {
    return "ttml";
  }
  if (url.includes("timedtext") && url.includes("fmt=json")) {
    return "youtube-json";
  }

  return "unknown";
}

// Parses WebVTT format.
// Spec: https://www.w3.org/TR/webvtt1/
//
// Example:
//   WEBVTT
//
//   00:00:01.000 --> 00:00:04.000
//   Hello, world.
//
//   00:00:05.000 --> 00:00:08.000
//   This is a subtitle.
export function parseWebVTT(content: string): TranscriptCue[] {
  const cues: TranscriptCue[] = [];
  // Split into blocks separated by blank lines
  const blocks = content.split(/\n\s*\n/);

  for (const block of blocks) {
    const lines = block.trim().split("\n");
    // Find the line with the timestamp arrow
    let timestampLineIdx = -1;
    for (let i = 0; i < lines.length; i++) {
      if (lines[i]!.includes("-->")) {
        timestampLineIdx = i;
        break;
      }
    }
    if (timestampLineIdx < 0) continue;

    const timestampLine = lines[timestampLineIdx]!;
    const match = timestampLine.match(
      /(\d{1,2}:)?(\d{2}):(\d{2})[.,](\d{3})\s*-->/,
    );
    if (!match) continue;

    const hours = match[1] ? parseInt(match[1], 10) : 0;
    const minutes = parseInt(match[2]!, 10);
    const seconds = parseInt(match[3]!, 10);
    const ms = parseInt(match[4]!, 10);
    const offsetMs = hours * 3600000 + minutes * 60000 + seconds * 1000 + ms;

    // Text is everything after the timestamp line
    const text = lines
      .slice(timestampLineIdx + 1)
      .join(" ")
      .replace(/<[^>]+>/g, "") // strip VTT tags like <b>, <i>, <c>
      .trim();

    if (text) {
      cues.push({ offsetMs, text });
    }
  }

  return cues;
}

// Parses TTML (Timed Text Markup Language) format.
// Used by Netflix and some other services.
//
// Example:
//   <?xml version="1.0" encoding="UTF-8"?>
//   <tt xmlns="http://www.w3.org/ns/ttml">
//     <body>
//       <div>
//         <p begin="00:00:01.000" end="00:00:04.000">Hello, world.</p>
//         <p begin="00:00:05.000" end="00:00:08.000">This is a subtitle.</p>
//       </div>
//     </body>
//   </tt>
export function parseTTML(content: string): TranscriptCue[] {
  const cues: TranscriptCue[] = [];

  // Use regex to extract <p> elements with begin attributes.
  // This avoids needing a full XML parser in the browser extension.
  const pPattern = /<p[^>]*\bbegin=["']([^"']+)["'][^>]*>([\s\S]*?)<\/p>/gi;
  let match;

  while ((match = pPattern.exec(content)) !== null) {
    const beginStr = match[1]!;
    const rawText = match[2]!;

    const offsetMs = parseTTMLTimestamp(beginStr);
    const text = rawText
      .replace(/<br\s*\/?>/gi, " ")
      .replace(/<[^>]+>/g, "")
      .replace(/\s+/g, " ")
      .trim();

    if (text) {
      cues.push({ offsetMs, text });
    }
  }

  return cues;
}

// Parses TTML timestamp format: "00:00:01.000" or "00:00:01,000" or "01:23.456"
function parseTTMLTimestamp(ts: string): number {
  // Handle HH:MM:SS.mmm or HH:MM:SS,mmm
  const full = ts.match(/(\d+):(\d+):(\d+)[.,](\d+)/);
  if (full) {
    return (
      parseInt(full[1]!, 10) * 3600000 +
      parseInt(full[2]!, 10) * 60000 +
      parseInt(full[3]!, 10) * 1000 +
      parseInt(full[4]!.padEnd(3, "0").slice(0, 3), 10)
    );
  }
  // Handle MM:SS.mmm
  const short = ts.match(/(\d+):(\d+)[.,](\d+)/);
  if (short) {
    return (
      parseInt(short[1]!, 10) * 60000 +
      parseInt(short[2]!, 10) * 1000 +
      parseInt(short[3]!.padEnd(3, "0").slice(0, 3), 10)
    );
  }
  return 0;
}

// Parses YouTube's timedtext JSON format.
//
// YouTube uses two JSON structures:
// 1. "events" format (from timedtext API with fmt=json3):
//    { "events": [{ "tStartMs": 0, "dDurationMs": 5000, "segs": [{ "utf8": "Hello" }] }] }
//
// 2. "actions" format (from get_transcript API):
//    { "actions": [{ "updateEngagementPanelAction": { "content": { "transcriptRenderer": {
//        "body": { "transcriptBodyRenderer": { "cueGroups": [
//          { "transcriptCueGroupRenderer": { "cues": [
//            { "transcriptCueRenderer": { "cue": { "simpleText": "Hello" },
//              "startOffsetMs": "0", "durationMs": "5000" } }
//          ] } }
//        ] } } } } } } }] }
export function parseYouTubeJSON(content: string): TranscriptCue[] {
  const parsed = JSON.parse(content);

  // Format 1: "events" array (timedtext JSON3)
  if (parsed.events && Array.isArray(parsed.events)) {
    return parseYouTubeEvents(parsed.events);
  }

  // Format 2: "actions" array (get_transcript API)
  if (parsed.actions && Array.isArray(parsed.actions)) {
    return parseYouTubeActions(parsed.actions);
  }

  return [];
}

interface YouTubeEvent {
  tStartMs?: number;
  segs?: Array<{ utf8?: string }>;
}

function parseYouTubeEvents(events: YouTubeEvent[]): TranscriptCue[] {
  const cues: TranscriptCue[] = [];

  for (const event of events) {
    if (!event.segs || event.tStartMs === undefined) continue;

    const text = event.segs
      .map((seg) => seg.utf8 ?? "")
      .join("")
      .replace(/\n/g, " ")
      .trim();

    if (text) {
      cues.push({ offsetMs: event.tStartMs, text });
    }
  }

  return cues;
}

function parseYouTubeActions(actions: unknown[]): TranscriptCue[] {
  const cues: TranscriptCue[] = [];

  for (const action of actions) {
    const cueGroups = extractCueGroups(action);
    for (const group of cueGroups) {
      const renderer = group?.transcriptCueGroupRenderer;
      if (!renderer?.cues) continue;

      for (const cue of renderer.cues) {
        const cr = cue?.transcriptCueRenderer;
        if (!cr) continue;

        const text = (cr.cue?.simpleText ?? "").trim();
        const offsetMs = parseInt(cr.startOffsetMs ?? "0", 10);

        if (text) {
          cues.push({ offsetMs, text });
        }
      }
    }
  }

  return cues;
}

// Navigates the deeply nested YouTube transcript API response to find cueGroups.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function extractCueGroups(action: any): any[] {
  try {
    return (
      action?.updateEngagementPanelAction?.content?.transcriptRenderer?.body
        ?.transcriptBodyRenderer?.cueGroups ?? []
    );
  } catch {
    return [];
  }
}
