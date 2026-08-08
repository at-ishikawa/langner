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
// Precedence (first rule that matches wins):
//  1. highlight — bold that exact word/phrase as a WHOLE word (case-insensitive
//     `\bHIGHLIGHT\b`, multi-word phrases allowed). Use this for irregular
//     inflections the lemma can't derive (e.g. lemma "go", highlight "went").
//  2. auto whole-word of each lemma candidate: `\bLEMMA\w*`, so "obliterate"
//     bolds "obliterated"/"obliterates". Both the entry and its Definition
//     alt-form are tried.
//  3. plain substring of a lemma candidate (the legacy behavior) as a last
//     resort.
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
    const re = new RegExp(`(\\b${escapeRegex(lemma)}\\w*)`, "gi");
    const { nodes, matched } = splitBold(text, re);
    if (matched) {
      return nodes;
    }
  }

  for (const lemma of candidates) {
    const re = new RegExp(`(${escapeRegex(lemma)})`, "gi");
    const { nodes, matched } = splitBold(text, re);
    if (matched) {
      return nodes;
    }
  }

  return [text];
}
