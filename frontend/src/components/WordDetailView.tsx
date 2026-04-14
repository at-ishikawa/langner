"use client";

import { Box, Text, VStack } from "@chakra-ui/react";
import type { WordDetail } from "@/store/quizStore";

interface WordDetailViewProps {
  wordDetail?: WordDetail | null;
}

// WordDetailView renders the body fields of a WordDetail: origin (prose),
// origin parts (structured etymology), synonyms, antonyms, and memo. Each
// section is only rendered when its field has content. Renders nothing when
// wordDetail is null/undefined or when all fields are empty.
//
// Note: pronunciation and partOfSpeech are intentionally NOT rendered here —
// quiz feedback pages show them inline in the word header, not as a section.
export function WordDetailView({ wordDetail }: WordDetailViewProps) {
  if (!wordDetail) return null;

  const origin = wordDetail.origin?.trim();
  const originParts = wordDetail.originParts ?? [];
  const synonyms = wordDetail.synonyms ?? [];
  const antonyms = wordDetail.antonyms ?? [];
  const memo = wordDetail.memo?.trim();

  const hasAny =
    !!origin ||
    originParts.length > 0 ||
    synonyms.length > 0 ||
    antonyms.length > 0 ||
    !!memo;

  if (!hasAny) return null;

  return (
    <VStack align="stretch" gap={3}>
      {origin && (
        <Box>
          <Text fontWeight="bold" fontSize="sm">
            Origin
          </Text>
          <Text whiteSpace="pre-wrap" fontSize="sm" color="fg.muted">
            {origin}
          </Text>
        </Box>
      )}

      {originParts.length > 0 && (
        <Box>
          <Text fontWeight="bold" fontSize="sm">
            Etymology
          </Text>
          <Box display="flex" gap={2} alignItems="center" flexWrap="wrap">
            {originParts.map((p, i) => (
              <Box key={i} display="flex" alignItems="center" gap={1}>
                {i > 0 && <Text color="fg.muted">+</Text>}
                <Text
                  color="blue.600"
                  _dark={{ color: "blue.300" }}
                  fontWeight="medium"
                  fontSize="sm"
                >
                  {p.origin}
                </Text>
                <Text fontSize="sm" color="fg.muted">
                  ({p.meaning})
                </Text>
                {p.language && (
                  <Box
                    px={1.5}
                    py={0}
                    borderRadius="full"
                    bg="gray.100"
                    _dark={{ bg: "gray.700" }}
                  >
                    <Text
                      fontSize="xs"
                      color="gray.600"
                      _dark={{ color: "gray.300" }}
                    >
                      {p.language}
                    </Text>
                  </Box>
                )}
              </Box>
            ))}
          </Box>
        </Box>
      )}

      {synonyms.length > 0 && (
        <Box>
          <Text fontWeight="bold" fontSize="sm">
            Synonyms
          </Text>
          <Text fontSize="sm" color="fg.muted">
            {synonyms.join(", ")}
          </Text>
        </Box>
      )}

      {antonyms.length > 0 && (
        <Box>
          <Text fontWeight="bold" fontSize="sm">
            Antonyms
          </Text>
          <Text fontSize="sm" color="fg.muted">
            {antonyms.join(", ")}
          </Text>
        </Box>
      )}

      {memo && (
        <Box>
          <Text fontWeight="bold" fontSize="sm">
            Note
          </Text>
          <Text whiteSpace="pre-wrap" fontSize="sm" color="fg.muted">
            {memo}
          </Text>
        </Box>
      )}
    </VStack>
  );
}
