"use client";

import { Box, Heading, Text, VStack } from "@chakra-ui/react";
import { QuizResultCard, type ResultItem } from "./QuizResultCard";

interface QuizResultsGroupedListProps {
  items: ResultItem[];
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function QuizResultsGroupedList({
  items,
  onOverride,
  onUndo,
  onSkip,
  onResume,
}: QuizResultsGroupedListProps) {
  const correctResults = items.filter((r) => r.correct && !r.isSkipped);
  const incorrectResults = items.filter((r) => !r.correct && !r.isSkipped);
  const skippedResults = items.filter((r) => r.isSkipped);

  return (
    <>
      {incorrectResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.fg" mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrectResults.map((r) => (
              <QuizResultCard
                key={r.key}
                item={r}
                onOverride={onOverride}
                onUndo={onUndo}
                onSkip={onSkip}
                onResume={onResume}
              />
            ))}
          </VStack>
        </Box>
      )}

      {correctResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.fg" mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correctResults.map((r) => (
              <QuizResultCard
                key={r.key}
                item={r}
                onOverride={onOverride}
                onUndo={onUndo}
                onSkip={onSkip}
                onResume={onResume}
              />
            ))}
          </VStack>
        </Box>
      )}

      {skippedResults.length > 0 && (
        <Box mb={6}>
          <Text fontWeight="bold" mb={2} color="fg.muted">
            Excluded from Quizzes ({skippedResults.length})
          </Text>
          <VStack align="stretch" gap={2}>
            {skippedResults.map((r) => (
              <QuizResultCard
                key={r.key}
                item={r}
                onOverride={onOverride}
                onUndo={onUndo}
                onSkip={onSkip}
                onResume={onResume}
              />
            ))}
          </VStack>
        </Box>
      )}
    </>
  );
}
