import React from "react";
import { describe, it, expect } from "vitest";
import { highlightExpression } from "./highlight";

// boldWords returns the text of every node the helper bolded (the <Text
// as="span" fontWeight="bold"> spans), in order.
function boldWords(nodes: React.ReactNode[]): string[] {
  const out: string[] = [];
  for (const node of nodes) {
    if (
      React.isValidElement(node) &&
      (node.props as { fontWeight?: string }).fontWeight === "bold"
    ) {
      out.push((node.props as { children: string }).children);
    }
  }
  return out;
}

describe("highlightExpression", () => {
  it("bolds the exact highlight word as a whole word when present", () => {
    const nodes = highlightExpression("She went home early yesterday.", ["go"], "went");
    expect(boldWords(nodes)).toEqual(["went"]);
  });

  it("supports a multi-word highlight phrase", () => {
    const nodes = highlightExpression(
      "They finally broke the ice at dinner.",
      ["break the ice"],
      "broke the ice",
    );
    expect(boldWords(nodes)).toEqual(["broke the ice"]);
  });

  it("falls back to the whole-word lemma rule when no highlight, bolding the full inflection", () => {
    const nodes = highlightExpression(
      "The note obliterated every good feeling.",
      ["obliterate"],
    );
    // \bobliterate\w* bolds the entire inflected form, not just the stem.
    expect(boldWords(nodes)).toEqual(["obliterated"]);
  });

  it("does not bold a partial word via the highlight rule", () => {
    const nodes = highlightExpression("The wenten path.", ["go"], "went");
    // \bwent\b must not match inside "wenten".
    expect(boldWords(nodes)).toEqual([]);
    expect(nodes).toEqual(["The wenten path."]);
  });

  it("falls back to a plain substring when the whole-word rule finds nothing", () => {
    const nodes = highlightExpression("It was a clever device.", ["ice"]);
    // \bice\w* can't match inside "device"; the substring rule still bolds it.
    expect(boldWords(nodes)).toEqual(["ice"]);
  });

  it("returns the text unchanged when nothing matches", () => {
    const nodes = highlightExpression("Nothing to see here.", ["absent"]);
    expect(nodes).toEqual(["Nothing to see here."]);
  });
});
