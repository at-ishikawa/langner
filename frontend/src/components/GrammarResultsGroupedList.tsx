"use client";

import { Box, Heading, Text, VStack } from "@chakra-ui/react";
import type { GrammarResultState } from "@/store/grammarStore";
import type { ResultItem } from "@/components/QuizResultCard";
import { GrammarFeedbackCard } from "@/components/GrammarFeedbackCard";

// Grammar version of QuizResultsGroupedList: same Incorrect / Correct / Excluded
// grouping and headings, but renders the labelled+diffed GrammarFeedbackCard.
interface GrammarResultsGroupedListProps {
  results: GrammarResultState[]; // paired with their global index for actions
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function GrammarResultsGroupedList({
  results,
  onOverride,
  onUndo,
  onSkip,
  onResume,
}: GrammarResultsGroupedListProps) {
  const withIndex = results.map((result, index) => ({ result, index }));
  const incorrect = withIndex.filter((r) => !r.result.correct && !r.result.isSkipped);
  const correct = withIndex.filter((r) => r.result.correct && !r.result.isSkipped);
  const skipped = withIndex.filter((r) => r.result.isSkipped);

  const render = ({ result, index }: { result: GrammarResultState; index: number }) => (
    <GrammarFeedbackCard
      key={result.noteId.toString()}
      result={result}
      index={index}
      onOverride={onOverride}
      onUndo={onUndo}
      onSkip={onSkip}
      onResume={onResume}
    />
  );

  return (
    <>
      {incorrect.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.600" _dark={{ color: "red.300" }} mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrect.map(render)}
          </VStack>
        </Box>
      )}

      {correct.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.600" _dark={{ color: "green.300" }} mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correct.map(render)}
          </VStack>
        </Box>
      )}

      {skipped.length > 0 && (
        <Box mb={6}>
          <Text fontWeight="bold" mb={2} color="gray.500">
            Excluded from Quizzes ({skipped.length})
          </Text>
          <VStack align="stretch" gap={2}>
            {skipped.map(render)}
          </VStack>
        </Box>
      )}
    </>
  );
}
