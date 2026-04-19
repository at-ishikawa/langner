"use client";

import { Box, Button, Heading, Text } from "@chakra-ui/react";
import { type ResultItem } from "./QuizResultCard";
import { QuizResultsGroupedList } from "./QuizResultsGroupedList";

interface BatchFeedbackProps {
  items: ResultItem[];
  isEtymology: boolean;
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
  isEtymology,
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
        {isFinal ? "Session Complete" : "Batch Feedback"}
      </Heading>
      <Box mb={4}>
        <Text fontWeight="bold">Batch: {items.length} words</Text>
        <Text color="green.600" _dark={{ color: "green.300" }} fontWeight="bold">
          Correct: {correctCount}
        </Text>
        <Text color="red.600" _dark={{ color: "red.300" }} fontWeight="bold">
          Incorrect: {incorrectCount}
        </Text>
      </Box>

      <QuizResultsGroupedList
        items={items}
        isEtymology={isEtymology}
        onOverride={onOverride}
        onUndo={onUndo}
        onSkip={onSkip}
        onResume={onResume}
      />

      <Button w="full" colorPalette="blue" onClick={isFinal ? onSeeResults : onContinue}>
        {isFinal ? "See Results" : "Continue"}
      </Button>
      {!isFinal && (
        <Button w="full" mt={2} variant="outline" colorPalette="green" onClick={onSeeResults}>
          See Results
        </Button>
      )}
    </Box>
  );
}
