import { describe, it, expect } from "vitest";
import { findDeepLinkStoryIndex } from "./deepLink";

// Mini fixture mirroring a Speak English Like an American-style notebook:
// each story is a lesson, each lesson has one scene whose `title` is the
// lesson's multi-paragraph plot summary, and definitions store the
// dictionary form in `definition` when the conversational form differs
// (e.g. plural "stuffed shirts" vs canonical "stuffed shirt").
const lessons = [
  {
    scenes: [
      {
        title: "Mark talks to Ron about a new competitor.",
        definitions: [
          { expression: "come out with", definition: "" },
          { expression: "head honcho", definition: "" },
        ],
      },
    ],
  },
  {
    scenes: [
      {
        title: "Mark regrets earlier choices.",
        definitions: [
          { expression: "kicking myself", definition: "kick oneself" },
        ],
      },
    ],
  },
  {
    scenes: [
      {
        title: "Cindy and Mark argue.",
        definitions: [
          { expression: "stuffed shirts", definition: "stuffed shirt" },
          { expression: "feather in one's cap", definition: "" },
        ],
      },
    ],
  },
];

describe("findDeepLinkStoryIndex", () => {
  it("returns the lesson where the canonical expression lives", () => {
    expect(findDeepLinkStoryIndex(lessons, "come out with", "")).toBe(0);
  });

  it("matches the definition alias when the URL carries the dictionary form", () => {
    expect(findDeepLinkStoryIndex(lessons, "kick oneself", "")).toBe(1);
  });

  it("matches the definition alias when the URL carries the conversational form recorded in learning history", () => {
    // Before the fix, comparing only `d.expression` returned -1 because
    // history records "stuffed shirt" (singular) and the YAML stores
    // "stuffed shirts" with `definition: stuffed shirt`. The fallback
    // sent the user to Lesson 1 — the regression this case pins.
    expect(findDeepLinkStoryIndex(lessons, "stuffed shirt", "")).toBe(2);
  });

  it("respects an explicit scene title filter", () => {
    expect(
      findDeepLinkStoryIndex(
        lessons,
        "stuffed shirt",
        "Cindy and Mark argue.",
      ),
    ).toBe(2);
  });

  it("returns -1 when the URL targets a word that isn't in any lesson", () => {
    expect(findDeepLinkStoryIndex(lessons, "nonexistent idiom", "")).toBe(-1);
  });

  it("returns -1 when the scene-title filter doesn't match any scene", () => {
    expect(
      findDeepLinkStoryIndex(lessons, "come out with", "Wrong scene title"),
    ).toBe(-1);
  });

  it("is case-insensitive", () => {
    expect(findDeepLinkStoryIndex(lessons, "HEAD HONCHO", "")).toBe(0);
  });
});
