"use client";

import { Box, Button, Heading, Text } from "@chakra-ui/react";
import { type ResultItem } from "./QuizResultCard";
import { QuizResultsGroupedList } from "./QuizResultsGroupedList";

interface BatchFeedbackProps {
  items: ResultItem[];
  isFinal: boolean;
  onContinue: () => void;
  onSeeResults: () => void;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function BatchFeedback({
  items,
  isFinal,
  onContinue,
  onSeeResults,
  onOverride,
  onUndo,
  onSkip,
  onResume,
}: BatchFeedbackProps) {
  const correctCount = items.filter((r) => r.correct && !r.isSkipped).length;
  const incorrectCount = items.filter((r) => !r.correct && !r.isSkipped).length;

  return (
    <Box>
      <Heading size="lg" mb={3}>
        Feedback
      </Heading>
      <Box mb={4}>
        <Text fontWeight="bold">Batch: {items.length} words</Text>
        <Text color="green.fg" fontWeight="bold">
          Correct: {correctCount}
        </Text>
        <Text color="red.fg" fontWeight="bold">
          Incorrect: {incorrectCount}
        </Text>
      </Box>

      <Box pb={20}>
        <QuizResultsGroupedList
          items={items}
          onOverride={onOverride}
          onUndo={onUndo}
          onSkip={onSkip}
          onResume={onResume}
        />
      </Box>

      <Box
        position="sticky"
        bottom={0}
        bg="bg"
        borderTopWidth="1px"
        borderTopColor="border"
        mx={-4}
        px={4}
        py={3}
        display="flex"
        gap={2}
      >
        <Button flex="1" colorPalette="brand" onClick={isFinal ? onSeeResults : onContinue}>
          {isFinal ? "See Results" : "Continue"}
        </Button>
        {!isFinal && (
          <Button flex="1" variant="outline" colorPalette="green" onClick={onSeeResults}>
            See Results
          </Button>
        )}
      </Box>
    </Box>
  );
}
