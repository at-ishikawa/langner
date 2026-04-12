import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { JSDOM } from "jsdom";
import { describe, expect, it } from "vitest";
import { parseTimestamp, scrapeYouTubePage } from "./scraper.js";

function loadFixture(name: string): Document {
  const path = resolve(__dirname, "../../test-fixtures", name);
  const html = readFileSync(path, "utf-8");
  const dom = new JSDOM(html);
  return dom.window.document;
}

describe("scrapeYouTubePage", () => {
  describe("video with chapters", () => {
    const doc = loadFixture("youtube-with-chapters.html");
    const result = scrapeYouTubePage(
      doc,
      "https://www.youtube.com/watch?v=abc123",
    );

    it("extracts the video title", () => {
      expect(result.title).toBe("Learn English with Stories");
    });

    it("extracts the channel name", () => {
      expect(result.channel).toBe("English Lessons");
    });

    it("extracts the video ID from the URL", () => {
      expect(result.videoId).toBe("abc123");
    });

    it("preserves the page URL", () => {
      expect(result.url).toBe("https://www.youtube.com/watch?v=abc123");
    });

    it("extracts all transcript cues", () => {
      expect(result.cues).toHaveLength(4);
      expect(result.cues[0]!.text).toBe("Hello everyone and welcome.");
      expect(result.cues[0]!.offsetMs).toBe(0);
      expect(result.cues[2]!.text).toBe("Now let us look at part two.");
      expect(result.cues[2]!.offsetMs).toBe(120_000);
    });

    it("extracts chapter markers", () => {
      expect(result.chapters).toHaveLength(2);
      expect(result.chapters[0]).toEqual({
        startMs: 0,
        title: "Introduction",
      });
      expect(result.chapters[1]).toEqual({
        startMs: 120_000,
        title: "Vocabulary",
      });
    });
  });

  describe("video without chapters", () => {
    const doc = loadFixture("youtube-without-chapters.html");
    const result = scrapeYouTubePage(
      doc,
      "https://www.youtube.com/watch?v=xyz789",
    );

    it("extracts the video title", () => {
      expect(result.title).toBe("My Cooking Vlog");
    });

    it("extracts the channel name", () => {
      expect(result.channel).toBe("Cooking Channel");
    });

    it("extracts transcript cues", () => {
      expect(result.cues).toHaveLength(3);
    });

    it("returns empty chapters array", () => {
      expect(result.chapters).toEqual([]);
    });
  });

  describe("video with no transcript", () => {
    const doc = loadFixture("youtube-no-transcript.html");
    const result = scrapeYouTubePage(
      doc,
      "https://www.youtube.com/watch?v=zzz",
    );

    it("still extracts title and channel", () => {
      expect(result.title).toBe("Music Video");
      expect(result.channel).toBe("Music Artist");
    });

    it("returns empty cues array", () => {
      expect(result.cues).toEqual([]);
    });

    it("returns empty chapters array", () => {
      expect(result.chapters).toEqual([]);
    });
  });

  describe("title fallback", () => {
    it("falls back to page title when metadata element is missing", () => {
      const dom = new JSDOM(
        '<!doctype html><html><head><title>Fallback Title - YouTube</title></head><body></body></html>',
      );
      const result = scrapeYouTubePage(
        dom.window.document,
        "https://www.youtube.com/watch?v=test",
      );
      expect(result.title).toBe("Fallback Title");
    });
  });

  describe("videoId extraction", () => {
    it("handles mobile URLs", () => {
      const dom = new JSDOM("<!doctype html><html><body></body></html>");
      const result = scrapeYouTubePage(
        dom.window.document,
        "https://m.youtube.com/watch?v=mobile123",
      );
      expect(result.videoId).toBe("mobile123");
    });

    it("returns empty string for malformed URLs", () => {
      const dom = new JSDOM("<!doctype html><html><body></body></html>");
      const result = scrapeYouTubePage(dom.window.document, "not-a-url");
      expect(result.videoId).toBe("");
    });
  });
});

describe("parseTimestamp", () => {
  it("parses mm:ss format", () => {
    expect(parseTimestamp("0:00")).toBe(0);
    expect(parseTimestamp("1:23")).toBe(83_000);
    expect(parseTimestamp("10:05")).toBe(605_000);
  });

  it("parses hh:mm:ss format", () => {
    expect(parseTimestamp("1:00:00")).toBe(3_600_000);
    expect(parseTimestamp("1:30:45")).toBe(5_445_000);
  });

  it("returns 0 for empty or malformed input", () => {
    expect(parseTimestamp("")).toBe(0);
    expect(parseTimestamp("abc")).toBe(0);
  });
});
