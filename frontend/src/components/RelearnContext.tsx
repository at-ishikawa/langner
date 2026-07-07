"use client";

import { Box, Text, Wrap, WrapItem } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { RelearnContextScene } from "@/lib/client";
import type { GraphPrompt } from "@/gen-protos/api/v1/quiz_pb";
import { RelationGraph } from "./RelationGraph";

interface RelearnContextProps {
  entry: string;
  scenes: RelearnContextScene[];
  exampleWords: string[];
  graphContext?: GraphPrompt;
}

// highlightEntry bolds every case-insensitive occurrence of the expression in a
// context line so the learner's eye lands on the word, mirroring how the Learn
// page highlights defined expressions in prose.
export function highlightEntry(text: string, entry: string): ReactNode[] {
  const trimmed = entry.trim();
  if (!trimmed) {
    return [text];
  }
  const escaped = trimmed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const parts = text.split(new RegExp(`(${escaped})`, "ig"));
  return parts.map((part, i) =>
    part.toLowerCase() === trimmed.toLowerCase() ? (
      <Text as="span" key={i} fontWeight="bold" color="blue.600" _dark={{ color: "blue.300" }}>
        {part}
      </Text>
    ) : (
      <Text as="span" key={i}>
        {part}
      </Text>
    ),
  );
}

// RelearnContext renders the Learn-page context that accompanies a Relearn
// answer: the scenes the word appears in, its related words, and the etymology
// relation graph. Every sub-section is omitted when empty, so a word with no
// context or no etymology renders nothing extra.
export default function RelearnContext({ entry, scenes, exampleWords, graphContext }: RelearnContextProps) {
  const hasScenes = scenes.some((s) => s.statements.length > 0 || s.conversations.length > 0);
  const hasExamples = exampleWords.length > 0;
  if (!hasScenes && !hasExamples && !graphContext) {
    return null;
  }

  return (
    <Box mt={4} display="flex" flexDirection="column" gap={4}>
      {hasScenes && (
        <Box>
          <Text fontSize="xs" fontWeight="semibold" color="gray.500" _dark={{ color: "gray.400" }} mb={1}>
            Where it appears
          </Text>
          <Box display="flex" flexDirection="column" gap={2}>
            {scenes.map((scene, si) => (
              <Box key={si} bg="gray.50" _dark={{ bg: "gray.800" }} borderRadius="md" p={3}>
                {scene.sceneTitle && (
                  <Text fontSize="xs" color="gray.400" mb={1}>
                    {scene.sceneTitle}
                  </Text>
                )}
                {scene.statements.map((stmt, i) => (
                  <Text key={`s${i}`} fontSize="sm">
                    {highlightEntry(stmt, entry)}
                  </Text>
                ))}
                {scene.conversations.map((conv, i) => (
                  <Text key={`c${i}`} fontSize="sm">
                    {conv.speaker && (
                      <Text as="span" fontWeight="semibold">
                        {conv.speaker}:{" "}
                      </Text>
                    )}
                    {highlightEntry(conv.quote, entry)}
                  </Text>
                ))}
              </Box>
            ))}
          </Box>
        </Box>
      )}

      {hasExamples && (
        <Box>
          <Text fontSize="xs" fontWeight="semibold" color="gray.500" _dark={{ color: "gray.400" }} mb={1}>
            Related words
          </Text>
          <Wrap>
            {exampleWords.map((w) => (
              <WrapItem key={w}>
                <Box
                  px={2}
                  py={0.5}
                  bg="purple.50"
                  color="purple.700"
                  _dark={{ bg: "purple.900", color: "purple.200" }}
                  borderRadius="full"
                  fontSize="sm"
                >
                  {w}
                </Box>
              </WrapItem>
            ))}
          </Wrap>
        </Box>
      )}

      {graphContext && (
        <Box>
          <Text fontSize="xs" fontWeight="semibold" color="gray.500" _dark={{ color: "gray.400" }} mb={1}>
            Origin
          </Text>
          <RelationGraph prompt={graphContext} value="" onValueChange={() => {}} disabled compact />
        </Box>
      )}
    </Box>
  );
}
