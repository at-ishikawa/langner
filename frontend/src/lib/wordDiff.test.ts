import { describe, it, expect } from "vitest";
import { wordDiff } from "./wordDiff";

const changed = (tokens: { text: string; changed: boolean }[]) =>
  tokens.filter((t) => t.changed).map((t) => t.text);

describe("wordDiff", () => {
  it("marks only the words that differ", () => {
    // A partial correction: two words still wrong.
    const { left, right } = wordDiff(
      "some kinds of magic by the tool",
      "some kind of magic with the tool",
    );
    expect(changed(left)).toEqual(["kinds", "by"]);
    expect(changed(right)).toEqual(["kind", "with"]);
  });

  it("marks nothing when the answer matches (case/punctuation-insensitive)", () => {
    const { left, right } = wordDiff("Went to school.", "went to school");
    expect(changed(left)).toEqual([]);
    expect(changed(right)).toEqual([]);
  });

  it("handles an empty answer", () => {
    const { left, right } = wordDiff("", "the correct fix");
    expect(left).toEqual([]);
    expect(changed(right)).toEqual(["the", "correct", "fix"]);
  });
});
