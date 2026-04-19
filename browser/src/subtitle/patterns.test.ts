import { describe, expect, it } from "vitest";
import { isSubtitleUrl } from "./patterns.js";

describe("isSubtitleUrl", () => {
  describe("YouTube", () => {
    it("matches timedtext API", () => {
      expect(
        isSubtitleUrl(
          "https://www.youtube.com/api/timedtext?v=abc123&lang=en&fmt=json3",
        ),
      ).toBe(true);
    });

    it("matches get_transcript API", () => {
      expect(
        isSubtitleUrl(
          "https://www.youtube.com/youtubei/v1/get_transcript?key=abc",
        ),
      ).toBe(true);
    });
  });

  describe("Netflix", () => {
    it("matches TTML on nflxvideo.net", () => {
      expect(
        isSubtitleUrl(
          "https://ipv4-c123-lax.nflxvideo.net/range/12345/67890.ttml?o=abc",
        ),
      ).toBe(true);
    });

    it("matches XML on nflxvideo.net", () => {
      expect(
        isSubtitleUrl(
          "https://cdn.nflxvideo.net/subs/en-us.xml",
        ),
      ).toBe(true);
    });

    it("matches DFXP on nflxvideo.net", () => {
      expect(
        isSubtitleUrl(
          "https://cdn.nflxvideo.net/subs/en-us.dfxp?t=12345",
        ),
      ).toBe(true);
    });
  });

  describe("Generic WebVTT", () => {
    it("matches .vtt files", () => {
      expect(isSubtitleUrl("https://cdn.example.com/subs/en.vtt")).toBe(true);
      expect(isSubtitleUrl("https://cdn.example.com/subs/en.vtt?token=abc")).toBe(true);
    });
  });

  describe("Generic TTML", () => {
    it("matches .ttml files", () => {
      expect(isSubtitleUrl("https://cdn.example.com/subs/en.ttml")).toBe(true);
    });
  });

  describe("non-subtitle URLs", () => {
    it("rejects regular page URLs", () => {
      expect(isSubtitleUrl("https://www.youtube.com/watch?v=abc123")).toBe(false);
    });

    it("rejects video stream URLs", () => {
      expect(isSubtitleUrl("https://rr1.googlevideo.com/videoplayback?itag=18")).toBe(false);
    });

    it("rejects random URLs", () => {
      expect(isSubtitleUrl("https://example.com/api/data.json")).toBe(false);
    });
  });
});
