"use client";

import React from "react";
import { Box, Heading, Text, VStack } from "@chakra-ui/react";
import type { NotebookWord, StoryScene } from "@/lib/client";

// SceneContent is the canonical story reader used by both the vocabulary Learn
// page (/learn/[id]) and the journal detail page (/journals/[id]) so the two
// render story prose and dialogue identically. It highlights defined
// expressions inline and honors explicit {{ word }} markers.
//
// The interactive props (onTextSelect, targetWord, storyIndex, sceneIndex) are
// optional: the Learn page passes them to wire word-lookup selection and the
// deep-link scroll anchor, while the journal review view reuses the same
// visual renderer with none of that wiring.

// renderHighlightedText wraps every occurrence of a defined expression in the
// given text with a bold/colored span. It also honors explicit {{ word }}
// markers authored in the source notebook.
// renderHighlightedText, plus an optional targetWord that — when matched —
// marks the *first* hit with id="learn-deep-link-target" + a flash hook so
// the page-level effect can scroll to it. Only the first occurrence is
// tagged because a single anchor is enough to drive scrollIntoView.
function renderHighlightedText(
  text: string,
  definitions: NotebookWord[],
  targetWord?: string,
  targetFoundRef?: { current: boolean },
): React.ReactNode[] {
  const markerParts = text.split(/\{\{\s*([^}]+?)\s*\}\}/);
  const nodes: React.ReactNode[] = [];

  for (let i = 0; i < markerParts.length; i++) {
    if (i % 2 === 1) {
      nodes.push(
        <Text
          as="span"
          key={`m-${i}`}
          fontWeight="bold"
          color="blue.600"
          _dark={{ color: "blue.300" }}
        >
          {markerParts[i].trim()}
        </Text>,
      );
      continue;
    }

    const segment = markerParts[i];
    const expressions = definitions
      .map((d) => d.expression)
      .filter((e) => e.length > 0)
      .sort((a, b) => b.length - a.length);

    if (expressions.length === 0) {
      nodes.push(
        <Text as="span" key={`t-${i}`}>
          {segment}
        </Text>,
      );
      continue;
    }

    const escapedExpressions = expressions.map((e) =>
      e.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
    );
    const regex = new RegExp(`\\b(${escapedExpressions.join("|")})\\b`, "gi");
    const subSegments = segment.split(regex);

    subSegments.forEach((sub, j) => {
      const isDefined = expressions.some(
        (e) => e.toLowerCase() === sub.toLowerCase(),
      );
      if (isDefined) {
        const isTarget =
          !!targetWord &&
          !targetFoundRef?.current &&
          sub.toLowerCase() === targetWord.toLowerCase();
        if (isTarget && targetFoundRef) targetFoundRef.current = true;
        nodes.push(
          <Text
            as="span"
            key={`t-${i}-${j}`}
            id={isTarget ? "learn-deep-link-target" : undefined}
            data-deep-link-flash={isTarget ? "true" : undefined}
            fontWeight="bold"
            color="blue.600"
            _dark={{ color: "blue.300" }}
            css={
              isTarget
                ? {
                    "&[data-deep-link-flash='true']": {
                      backgroundColor: "var(--chakra-colors-yellow-100)",
                      borderRadius: "2px",
                      padding: "0 2px",
                    },
                  }
                : undefined
            }
          >
            {sub}
          </Text>,
        );
      } else {
        nodes.push(
          <Text as="span" key={`t-${i}-${j}`}>
            {sub}
          </Text>,
        );
      }
    });
  }

  return nodes;
}

export function SceneContent({
  scene,
  storyIndex = 0,
  sceneIndex = 0,
  onTextSelect,
  targetWord,
}: {
  scene: StoryScene;
  // storyIndex / sceneIndex identify this scene to onTextSelect (word lookup).
  // Only meaningful when onTextSelect is provided; default 0 otherwise.
  storyIndex?: number;
  sceneIndex?: number;
  // onTextSelect — when provided (Learn page), the prose/dialogue become
  // selectable for word lookup. Omitted on the journal review view.
  onTextSelect?: (storyIndex: number, sceneIndex: number) => void;
  // targetWord — when non-empty, the first highlighted occurrence within
  // this scene gets the id="learn-deep-link-target" anchor used by the
  // page's scroll effect. Across multiple scenes we still want exactly
  // one hit, so the parent passes a fresh ref per render below.
  targetWord?: string;
}) {
  // Shared across the prose and the dialogue blocks so only the first
  // occurrence in this scene wins the anchor.
  const targetFoundRef = { current: false };
  const hasStatements = scene.statements.length > 0;
  const hasConversations = scene.conversations.length > 0;

  // Hide scenes that carry neither prose nor dialogue (e.g. flashcard-style
  // scenes). The /notebooks/[id] word list is the right place for those.
  if (!hasStatements && !hasConversations) return null;

  return (
    <Box>
      {scene.title && (
        <Heading size="sm" mb={2} color="fg.muted">
          {scene.title}
        </Heading>
      )}

      {hasStatements && (
        <Box
          onMouseUp={
            onTextSelect ? () => onTextSelect(storyIndex, sceneIndex) : undefined
          }
          cursor={onTextSelect ? "text" : undefined}
          lineHeight="tall"
        >
          <VStack align="stretch" gap={3}>
            {scene.statements.map((stmt, i) => (
              <Text key={i} fontSize="md">
                {renderHighlightedText(stmt, scene.definitions, targetWord, targetFoundRef)}
              </Text>
            ))}
          </VStack>
        </Box>
      )}

      {hasConversations && (
        <Box
          onMouseUp={
            onTextSelect ? () => onTextSelect(storyIndex, sceneIndex) : undefined
          }
          cursor={onTextSelect ? "text" : undefined}
          mt={hasStatements ? 3 : 0}
        >
          <VStack align="stretch" gap={1}>
            {scene.conversations.map((conv, i) => (
              <Text key={i} fontSize="sm" color="fg.muted">
                <Text as="span" fontWeight="bold" color="fg.default">
                  {conv.speaker}:
                </Text>{" "}
                &ldquo;{renderHighlightedText(conv.quote, scene.definitions, targetWord, targetFoundRef)}&rdquo;
              </Text>
            ))}
          </VStack>
        </Box>
      )}
    </Box>
  );
}
