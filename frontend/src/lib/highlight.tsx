import React from "react";
import { Text } from "@chakra-ui/react";

// escapeRegex escapes a literal string for safe use inside a RegExp.
function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// splitBold splits text on a single-capture-group regex and bolds every
// captured span. A capturing split places matches at odd indices; matched
// reports whether the regex hit anything (so callers can fall through to the
// next rule).
function splitBold(
  text: string,
  re: RegExp,
): { nodes: React.ReactNode[]; matched: boolean } {
  const parts = text.split(re);
  const nodes = parts.map((part, i) =>
    i % 2 === 1 ? (
      <Text
        as="span"
        key={i}
        fontWeight="bold"
        color="blue.600"
        _dark={{ color: "blue.300" }}
      >
        {part}
      </Text>
    ) : (
      <React.Fragment key={i}>{part}</React.Fragment>
    ),
  );
  return { nodes, matched: parts.length > 1 };
}

// highlightExpression bolds the target word inside an example sentence.
//
// Matching is EXACT-WHOLE-WORD only (case-insensitive); precedence (first rule
// that matches wins):
//  1. highlight — bold that exact word/phrase as a WHOLE word
//     (`\bHIGHLIGHT\b`, multi-word phrases allowed). Use this for irregular or
//     inflected forms the lemma can't match exactly (e.g. lemma "go",
//     highlight "went"; lemma "obliterate", highlight "obliterated").
//  2. each lemma candidate as a WHOLE word (`\bLEMMA\b`), so lemma "high" bolds
//     "high" but never "highlight"/"highway". Both the entry and its Definition
//     alt-form are tried.
//
// There is no prefix (`\bLEMMA\w*`) or substring guessing: if neither the
// highlight nor a lemma matches as a whole word, nothing is bolded (the author
// is expected to add a `highlight` for forms the lemma can't match).
//
// lemmas is the ordered list of lemma candidates (typically [entry,
// originalEntry]); empty/blank entries are ignored.
export function highlightExpression(
  text: string,
  lemmas: string[],
  highlight?: string,
): React.ReactNode[] {
  const trimmedHighlight = highlight?.trim();
  if (trimmedHighlight) {
    const re = new RegExp(`(\\b${escapeRegex(trimmedHighlight)}\\b)`, "gi");
    const { nodes, matched } = splitBold(text, re);
    if (matched) {
      return nodes;
    }
  }

  const candidates = lemmas
    .map((lemma) => lemma?.trim())
    .filter((lemma): lemma is string => !!lemma);

  for (const lemma of candidates) {
    const re = new RegExp(`(\\b${escapeRegex(lemma)}\\b)`, "gi");
    const { nodes, matched } = splitBold(text, re);
    if (matched) {
      return nodes;
    }
  }

  return [text];
}
