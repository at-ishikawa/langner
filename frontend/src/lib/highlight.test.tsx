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

  it("bolds a lemma that appears as an exact whole word", () => {
    const nodes = highlightExpression("The obliterate spell is powerful.", [
      "obliterate",
    ]);
    expect(boldWords(nodes)).toEqual(["obliterate"]);
  });

  it("does NOT bold an inflected form via the lemma rule (highlight required)", () => {
    const nodes = highlightExpression(
      "The note obliterated every good feeling.",
      ["obliterate"],
    );
    // Exact-word only: \bobliterate\b cannot match "obliterated"; without a
    // `highlight` field nothing is bolded.
    expect(boldWords(nodes)).toEqual([]);
    expect(nodes).toEqual(["The note obliterated every good feeling."]);
  });

  it("bolds the inflected form once a matching highlight is supplied", () => {
    const nodes = highlightExpression(
      "The note obliterated every good feeling.",
      ["obliterate"],
      "obliterated",
    );
    expect(boldWords(nodes)).toEqual(["obliterated"]);
  });

  it("bolds a lemma as a whole word but not a longer word containing it", () => {
    const nodes = highlightExpression(
      "The high tower cast a highlight on the highway.",
      ["high"],
    );
    // \bhigh\b matches only the standalone "high", never "highlight"/"highway".
    expect(boldWords(nodes)).toEqual(["high"]);
  });

  it("does not bold a partial word via the highlight rule", () => {
    const nodes = highlightExpression("The wenten path.", ["go"], "went");
    // \bwent\b must not match inside "wenten".
    expect(boldWords(nodes)).toEqual([]);
    expect(nodes).toEqual(["The wenten path."]);
  });

  it("does NOT fall back to a substring when the whole-word rule finds nothing", () => {
    const nodes = highlightExpression("It was a clever device.", ["ice"]);
    // No substring fallback: "ice" inside "device" is left untouched.
    expect(boldWords(nodes)).toEqual([]);
    expect(nodes).toEqual(["It was a clever device."]);
  });

  it("returns the text unchanged when nothing matches", () => {
    const nodes = highlightExpression("Nothing to see here.", ["absent"]);
    expect(nodes).toEqual(["Nothing to see here."]);
  });
});
