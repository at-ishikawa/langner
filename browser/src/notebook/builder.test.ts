import { describe, expect, it } from "vitest";
import { buildStoryNotebook } from "./builder.js";
import type { ScrapedVideo } from "../types.js";

function sampleVideo(overrides: Partial<ScrapedVideo> = {}): ScrapedVideo {
  return {
    videoId: "abc123",
    title: "Learn English with Stories",
    channel: "English Lessons",
    url: "https://www.youtube.com/watch?v=abc123",
    cues: [
      { offsetMs: 0, text: "Hello everyone." },
      { offsetMs: 2000, text: "Welcome to the lesson." },
      { offsetMs: 60000, text: "Now let us begin part two." },
      { offsetMs: 62000, text: "This is the second section." },
    ],
    chapters: [],
    ...overrides,
  };
}

describe("buildStoryNotebook", () => {
  const fixedDate = new Date("2026-04-11T00:00:00.000Z");

  it("builds a notebook with the video title and URL in event", () => {
    const notebook = buildStoryNotebook(sampleVideo(), undefined, fixedDate);
    expect(notebook.event).toBe(
      "Learn English with Stories: https://www.youtube.com/watch?v=abc123",
    );
  });

  it("uses just the title when URL is empty", () => {
    const notebook = buildStoryNotebook(
      sampleVideo({ url: "" }),
      undefined,
      fixedDate,
    );
    expect(notebook.event).toBe("Learn English with Stories");
  });

  it("sets metadata.series to the channel name", () => {
    const notebook = buildStoryNotebook(sampleVideo(), undefined, fixedDate);
    expect(notebook.metadata.series).toBe("English Lessons");
  });

  it("falls back to YouTube when channel is empty", () => {
    const notebook = buildStoryNotebook(
      sampleVideo({ channel: "" }),
      undefined,
      fixedDate,
    );
    expect(notebook.metadata.series).toBe("YouTube");
  });

  it("sets season and episode to 0", () => {
    const notebook = buildStoryNotebook(sampleVideo(), undefined, fixedDate);
    expect(notebook.metadata.season).toBe(0);
    expect(notebook.metadata.episode).toBe(0);
  });

  it("uses the provided capture date", () => {
    const notebook = buildStoryNotebook(sampleVideo(), undefined, fixedDate);
    expect(notebook.date).toEqual(fixedDate);
  });

  it("segments cues into scenes via gap-based logic", () => {
    const notebook = buildStoryNotebook(
      sampleVideo(),
      { gapThresholdMs: 4000, maxSceneDurationMs: 60_000 },
      fixedDate,
    );
    // 58s gap between cue at 2000ms and cue at 60000ms exceeds both
    // maxSceneDurationMs and gapThresholdMs, so we expect 2 scenes.
    expect(notebook.scenes.length).toBeGreaterThanOrEqual(2);
  });

  it("uses chapter-based segmentation when chapters are provided", () => {
    const video = sampleVideo({
      chapters: [
        { startMs: 0, title: "Greeting" },
        { startMs: 60000, title: "Part Two" },
      ],
    });

    const notebook = buildStoryNotebook(video, undefined, fixedDate);
    expect(notebook.scenes).toHaveLength(2);
    expect(notebook.scenes[0]!.scene).toBe("Greeting");
    expect(notebook.scenes[1]!.scene).toBe("Part Two");
  });

  it("returns a notebook with empty scenes for a video with no cues", () => {
    const video = sampleVideo({ cues: [] });
    const notebook = buildStoryNotebook(video, undefined, fixedDate);
    expect(notebook.scenes).toEqual([]);
  });

  it("produces scenes with empty definitions", () => {
    const notebook = buildStoryNotebook(sampleVideo(), undefined, fixedDate);
    for (const scene of notebook.scenes) {
      expect(scene.definitions).toEqual([]);
    }
  });
});
