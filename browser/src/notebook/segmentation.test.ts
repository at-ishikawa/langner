import { describe, expect, it } from "vitest";
import { segmentIntoScenes, DEFAULT_SEGMENTATION } from "./segmentation.js";
import type { Chapter, TranscriptCue } from "../types.js";

function makeCue(offsetMs: number, text: string): TranscriptCue {
  return { offsetMs, text };
}

describe("segmentIntoScenes", () => {
  describe("with chapters", () => {
    it("creates one scene per chapter, using chapter titles", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "Hello"),
        makeCue(2000, "Welcome"),
        makeCue(60000, "Now let us look at part two"),
        makeCue(62000, "This is interesting"),
      ];
      const chapters: Chapter[] = [
        { startMs: 0, title: "Introduction" },
        { startMs: 60000, title: "Part Two" },
      ];

      const scenes = segmentIntoScenes(cues, chapters);
      expect(scenes).toHaveLength(2);
      expect(scenes[0]!.scene).toBe("Introduction");
      expect(scenes[0]!.conversations).toHaveLength(2);
      expect(scenes[1]!.scene).toBe("Part Two");
      expect(scenes[1]!.conversations).toHaveLength(2);
    });

    it("assigns cues to the correct chapter based on offset", () => {
      const cues: TranscriptCue[] = [
        makeCue(500, "First"),
        makeCue(10000, "Second"),
        makeCue(20000, "Third"),
      ];
      const chapters: Chapter[] = [
        { startMs: 0, title: "A" },
        { startMs: 15000, title: "B" },
      ];

      const scenes = segmentIntoScenes(cues, chapters);
      expect(scenes[0]!.conversations).toHaveLength(2);
      expect(scenes[1]!.conversations).toHaveLength(1);
      expect(scenes[1]!.conversations[0]!.quote).toBe("Third");
    });

    it("drops chapters that have no matching cues", () => {
      const cues: TranscriptCue[] = [makeCue(0, "Hello")];
      const chapters: Chapter[] = [
        { startMs: 0, title: "Has cue" },
        { startMs: 100000, title: "Empty chapter" },
      ];

      const scenes = segmentIntoScenes(cues, chapters);
      expect(scenes).toHaveLength(1);
      expect(scenes[0]!.scene).toBe("Has cue");
    });

    it("sorts unsorted chapters by startMs", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "First"),
        makeCue(50000, "Second"),
      ];
      const chapters: Chapter[] = [
        { startMs: 50000, title: "B" },
        { startMs: 0, title: "A" },
      ];

      const scenes = segmentIntoScenes(cues, chapters);
      expect(scenes[0]!.scene).toBe("A");
      expect(scenes[1]!.scene).toBe("B");
    });

    it("falls back to numbered chapter title when title is empty", () => {
      const cues: TranscriptCue[] = [makeCue(0, "Hello")];
      const chapters: Chapter[] = [{ startMs: 0, title: "" }];

      const scenes = segmentIntoScenes(cues, chapters);
      expect(scenes[0]!.scene).toBe("Chapter 1");
    });
  });

  describe("without chapters (gap-based)", () => {
    it("keeps cues in a single scene when there are no gaps", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "First"),
        makeCue(1000, "Second"),
        makeCue(2000, "Third"),
      ];

      const scenes = segmentIntoScenes(cues, []);
      expect(scenes).toHaveLength(1);
      expect(scenes[0]!.conversations).toHaveLength(3);
    });

    it("splits on gaps exceeding the threshold", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "First group a"),
        makeCue(1000, "First group b"),
        makeCue(10000, "Second group a"), // 9s gap > default 4s threshold
        makeCue(11000, "Second group b"),
      ];

      const scenes = segmentIntoScenes(cues, []);
      expect(scenes).toHaveLength(2);
      expect(scenes[0]!.conversations).toHaveLength(2);
      expect(scenes[1]!.conversations).toHaveLength(2);
    });

    it("splits when scene duration exceeds maxSceneDurationMs", () => {
      // 65 cues, 1s apart — exceeds default 60s max
      const cues: TranscriptCue[] = [];
      for (let i = 0; i < 65; i++) {
        cues.push(makeCue(i * 1000, `Line ${i + 1}`));
      }

      const scenes = segmentIntoScenes(cues, []);
      expect(scenes.length).toBeGreaterThanOrEqual(2);
    });

    it("labels scenes with segment numbers and time ranges", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "A"),
        makeCue(1000, "B"),
        makeCue(10000, "C"), // gap triggers new scene
      ];

      const scenes = segmentIntoScenes(cues, []);
      expect(scenes).toHaveLength(2);
      expect(scenes[0]!.scene).toMatch(/^Segment 1/);
      expect(scenes[1]!.scene).toMatch(/^Segment 2/);
    });

    it("respects custom options", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "A"),
        makeCue(3000, "B"), // 3s gap — exceeds custom 2s threshold
        makeCue(4000, "C"),
      ];

      const scenes = segmentIntoScenes(cues, [], {
        gapThresholdMs: 2000,
        maxSceneDurationMs: 120_000,
      });
      expect(scenes).toHaveLength(2);
    });
  });

  describe("edge cases", () => {
    it("returns empty array for empty cues", () => {
      expect(segmentIntoScenes([], [])).toEqual([]);
      expect(segmentIntoScenes([], [{ startMs: 0, title: "A" }])).toEqual([]);
    });

    it("handles a single cue", () => {
      const scenes = segmentIntoScenes([makeCue(0, "Only one")], []);
      expect(scenes).toHaveLength(1);
      expect(scenes[0]!.conversations).toHaveLength(1);
    });

    it("filters out cues with blank text", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "Keep this"),
        makeCue(1000, "   "),
        makeCue(2000, ""),
        makeCue(3000, "And this"),
      ];

      const scenes = segmentIntoScenes(cues, []);
      const allQuotes = scenes.flatMap((s) =>
        s.conversations.map((c) => c.quote),
      );
      expect(allQuotes).toEqual(["Keep this", "And this"]);
    });

    it("leaves speaker empty on every conversation", () => {
      const cues: TranscriptCue[] = [
        makeCue(0, "Hello"),
        makeCue(1000, "World"),
      ];
      const scenes = segmentIntoScenes(cues, []);
      for (const scene of scenes) {
        for (const conv of scene.conversations) {
          expect(conv.speaker).toBe("");
        }
      }
    });

    it("always produces empty definitions arrays", () => {
      const cues: TranscriptCue[] = [makeCue(0, "Hello")];
      const scenes = segmentIntoScenes(cues, []);
      for (const scene of scenes) {
        expect(scene.definitions).toEqual([]);
      }
    });
  });
});
