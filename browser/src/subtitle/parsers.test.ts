import { describe, expect, it } from "vitest";
import {
  detectFormat,
  parseSubtitle,
  parseWebVTT,
  parseTTML,
  parseYouTubeJSON,
} from "./parsers.js";

describe("detectFormat", () => {
  it("detects WebVTT from content", () => {
    expect(detectFormat("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi", "")).toBe("webvtt");
  });

  it("detects TTML from XML content", () => {
    expect(detectFormat('<?xml version="1.0"?><tt><body></body></tt>', "")).toBe("ttml");
    expect(detectFormat("<tt><body></body></tt>", "")).toBe("ttml");
  });

  it("detects YouTube JSON from events array", () => {
    const json = JSON.stringify({ events: [{ tStartMs: 0, segs: [{ utf8: "Hi" }] }] });
    expect(detectFormat(json, "")).toBe("youtube-json");
  });

  it("detects YouTube JSON from actions array", () => {
    const json = JSON.stringify({ actions: [{}] });
    expect(detectFormat(json, "")).toBe("youtube-json");
  });

  it("falls back to URL-based detection for .vtt", () => {
    expect(detectFormat("some content", "https://example.com/subs.vtt")).toBe("webvtt");
  });

  it("falls back to URL-based detection for .ttml", () => {
    expect(detectFormat("some content", "https://cdn.example.com/sub.ttml")).toBe("ttml");
  });

  it("falls back to URL-based detection for YouTube timedtext", () => {
    expect(detectFormat("some content", "https://www.youtube.com/api/timedtext?fmt=json&v=abc")).toBe("youtube-json");
  });

  it("returns unknown for unrecognized content and URL", () => {
    expect(detectFormat("random text", "https://example.com/page")).toBe("unknown");
  });
});

describe("parseWebVTT", () => {
  it("parses a standard WebVTT file", () => {
    const vtt = `WEBVTT

00:00:01.000 --> 00:00:04.000
Hello, world.

00:00:05.500 --> 00:00:08.000
This is a subtitle.`;

    const cues = parseWebVTT(vtt);
    expect(cues).toHaveLength(2);
    expect(cues[0]).toEqual({ offsetMs: 1000, text: "Hello, world." });
    expect(cues[1]).toEqual({ offsetMs: 5500, text: "This is a subtitle." });
  });

  it("handles cues with numeric IDs", () => {
    const vtt = `WEBVTT

1
00:00:01.000 --> 00:00:02.000
First cue

2
00:00:03.000 --> 00:00:04.000
Second cue`;

    const cues = parseWebVTT(vtt);
    expect(cues).toHaveLength(2);
    expect(cues[0]!.text).toBe("First cue");
    expect(cues[1]!.text).toBe("Second cue");
  });

  it("handles hours in timestamps", () => {
    const vtt = `WEBVTT

01:30:00.000 --> 01:30:05.000
One hour thirty minutes in`;

    const cues = parseWebVTT(vtt);
    expect(cues).toHaveLength(1);
    expect(cues[0]!.offsetMs).toBe(5400000);
  });

  it("strips VTT formatting tags", () => {
    const vtt = `WEBVTT

00:00:01.000 --> 00:00:02.000
<b>Bold</b> and <i>italic</i> text`;

    const cues = parseWebVTT(vtt);
    expect(cues[0]!.text).toBe("Bold and italic text");
  });

  it("joins multi-line cue text", () => {
    const vtt = `WEBVTT

00:00:01.000 --> 00:00:04.000
Line one
Line two`;

    const cues = parseWebVTT(vtt);
    expect(cues[0]!.text).toBe("Line one Line two");
  });

  it("handles comma as decimal separator", () => {
    const vtt = `WEBVTT

00:00:01,500 --> 00:00:02,000
Comma separator`;

    const cues = parseWebVTT(vtt);
    expect(cues[0]!.offsetMs).toBe(1500);
  });

  it("returns empty array for empty content", () => {
    expect(parseWebVTT("")).toEqual([]);
    expect(parseWebVTT("WEBVTT")).toEqual([]);
  });

  it("skips cues with empty text", () => {
    const vtt = `WEBVTT

00:00:01.000 --> 00:00:02.000


00:00:03.000 --> 00:00:04.000
Has text`;

    const cues = parseWebVTT(vtt);
    expect(cues).toHaveLength(1);
    expect(cues[0]!.text).toBe("Has text");
  });
});

describe("parseTTML", () => {
  it("parses a standard TTML document", () => {
    const ttml = `<?xml version="1.0" encoding="UTF-8"?>
<tt xmlns="http://www.w3.org/ns/ttml">
  <body>
    <div>
      <p begin="00:00:01.000" end="00:00:04.000">Hello from TTML.</p>
      <p begin="00:00:05.500" end="00:00:08.000">Second subtitle.</p>
    </div>
  </body>
</tt>`;

    const cues = parseTTML(ttml);
    expect(cues).toHaveLength(2);
    expect(cues[0]).toEqual({ offsetMs: 1000, text: "Hello from TTML." });
    expect(cues[1]).toEqual({ offsetMs: 5500, text: "Second subtitle." });
  });

  it("handles <br> tags as spaces", () => {
    const ttml = `<tt><body><div>
      <p begin="00:00:01.000" end="00:00:02.000">Line one<br/>Line two</p>
    </div></body></tt>`;

    const cues = parseTTML(ttml);
    expect(cues[0]!.text).toBe("Line one Line two");
  });

  it("strips inline formatting tags", () => {
    const ttml = `<tt><body><div>
      <p begin="00:00:01.000" end="00:00:02.000"><span style="color:white">Styled text</span></p>
    </div></body></tt>`;

    const cues = parseTTML(ttml);
    expect(cues[0]!.text).toBe("Styled text");
  });

  it("handles comma as decimal separator", () => {
    const ttml = `<tt><body><div>
      <p begin="00:00:01,500" end="00:00:02,000">Comma separator</p>
    </div></body></tt>`;

    const cues = parseTTML(ttml);
    expect(cues[0]!.offsetMs).toBe(1500);
  });

  it("handles MM:SS.mmm without hours", () => {
    const ttml = `<tt><body><div>
      <p begin="01:30.000" end="01:35.000">Short format</p>
    </div></body></tt>`;

    const cues = parseTTML(ttml);
    expect(cues[0]!.offsetMs).toBe(90000);
  });

  it("returns empty array for content with no <p> elements", () => {
    expect(parseTTML("<tt><body></body></tt>")).toEqual([]);
    expect(parseTTML("not xml at all")).toEqual([]);
  });
});

describe("parseYouTubeJSON", () => {
  describe("events format (timedtext JSON3)", () => {
    it("parses events with segments", () => {
      const json = JSON.stringify({
        events: [
          { tStartMs: 0, segs: [{ utf8: "Hello " }, { utf8: "world" }] },
          { tStartMs: 5000, segs: [{ utf8: "Second line" }] },
        ],
      });

      const cues = parseYouTubeJSON(json);
      expect(cues).toHaveLength(2);
      expect(cues[0]).toEqual({ offsetMs: 0, text: "Hello world" });
      expect(cues[1]).toEqual({ offsetMs: 5000, text: "Second line" });
    });

    it("skips events without segs", () => {
      const json = JSON.stringify({
        events: [
          { tStartMs: 0 },
          { tStartMs: 1000, segs: [{ utf8: "Has text" }] },
        ],
      });

      const cues = parseYouTubeJSON(json);
      expect(cues).toHaveLength(1);
      expect(cues[0]!.text).toBe("Has text");
    });

    it("skips events with only whitespace text", () => {
      const json = JSON.stringify({
        events: [
          { tStartMs: 0, segs: [{ utf8: "\n" }] },
          { tStartMs: 1000, segs: [{ utf8: "Real text" }] },
        ],
      });

      const cues = parseYouTubeJSON(json);
      expect(cues).toHaveLength(1);
      expect(cues[0]!.text).toBe("Real text");
    });

    it("replaces newlines with spaces", () => {
      const json = JSON.stringify({
        events: [{ tStartMs: 0, segs: [{ utf8: "Line one\nLine two" }] }],
      });

      const cues = parseYouTubeJSON(json);
      expect(cues[0]!.text).toBe("Line one Line two");
    });
  });

  describe("actions format (get_transcript API)", () => {
    it("parses the nested cueGroups structure", () => {
      const json = JSON.stringify({
        actions: [
          {
            updateEngagementPanelAction: {
              content: {
                transcriptRenderer: {
                  body: {
                    transcriptBodyRenderer: {
                      cueGroups: [
                        {
                          transcriptCueGroupRenderer: {
                            cues: [
                              {
                                transcriptCueRenderer: {
                                  cue: { simpleText: "First cue" },
                                  startOffsetMs: "0",
                                  durationMs: "3000",
                                },
                              },
                            ],
                          },
                        },
                        {
                          transcriptCueGroupRenderer: {
                            cues: [
                              {
                                transcriptCueRenderer: {
                                  cue: { simpleText: "Second cue" },
                                  startOffsetMs: "5000",
                                  durationMs: "2000",
                                },
                              },
                            ],
                          },
                        },
                      ],
                    },
                  },
                },
              },
            },
          },
        ],
      });

      const cues = parseYouTubeJSON(json);
      expect(cues).toHaveLength(2);
      expect(cues[0]).toEqual({ offsetMs: 0, text: "First cue" });
      expect(cues[1]).toEqual({ offsetMs: 5000, text: "Second cue" });
    });
  });

  it("returns empty array for empty events", () => {
    expect(parseYouTubeJSON(JSON.stringify({ events: [] }))).toEqual([]);
  });

  it("returns empty array for unrecognized JSON structure", () => {
    expect(parseYouTubeJSON(JSON.stringify({ something: "else" }))).toEqual([]);
  });
});

describe("parseSubtitle (auto-detect)", () => {
  it("auto-detects and parses WebVTT", () => {
    const vtt = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello";
    const cues = parseSubtitle(vtt, "");
    expect(cues).toHaveLength(1);
    expect(cues[0]!.text).toBe("Hello");
  });

  it("auto-detects and parses TTML", () => {
    const ttml = '<tt><body><div><p begin="00:00:01.000" end="00:00:02.000">Hello</p></div></body></tt>';
    const cues = parseSubtitle(ttml, "");
    expect(cues).toHaveLength(1);
    expect(cues[0]!.text).toBe("Hello");
  });

  it("auto-detects and parses YouTube JSON", () => {
    const json = JSON.stringify({ events: [{ tStartMs: 0, segs: [{ utf8: "Hello" }] }] });
    const cues = parseSubtitle(json, "");
    expect(cues).toHaveLength(1);
    expect(cues[0]!.text).toBe("Hello");
  });

  it("returns empty array for unknown format", () => {
    expect(parseSubtitle("random garbage", "https://example.com")).toEqual([]);
  });
});
