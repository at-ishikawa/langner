"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { useQuizStore } from "@/store/quizStore";
import { type ResultItem } from "@/components/QuizResultCard";
import { QuizResultsGroupedList } from "@/components/QuizResultsGroupedList";
import {
  standardResultToItem,
  reverseResultToItem,
  freeformResultToItem,
  etymologyResultToItem,
} from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const etymologyResults = useQuizStore((s) => s.etymologyOriginResults);
  const quizType = useQuizStore((s) => s.quizType);
  const reset = useQuizStore((s) => s.reset);
  const isEtymologyQuiz = quizType === "etymology-standard" || quizType === "etymology-reverse" || quizType === "etymology-freeform";

  const allResults = useMemo((): ResultItem[] => {
    if (results.length > 0) return results.map(standardResultToItem);
    if (reverseResults.length > 0) return reverseResults.map(reverseResultToItem);
    if (freeformResults.length > 0) return freeformResults.map(freeformResultToItem);
    if (etymologyResults.length > 0) return etymologyResults.map(etymologyResultToItem);
    return [];
  }, [results, reverseResults, freeformResults, etymologyResults]);

  useEffect(() => {
    if (allResults.length === 0) {
      router.push("/");
    }
  }, [allResults, router]);

  const { handleOverride, handleUndo, handleSkip, handleResume } = useQuizResultActions(quizType);

  if (allResults.length === 0) {
    return null;
  }

  const correctCount = allResults.filter((r) => r.correct && !r.isSkipped).length;
  const incorrectCount = allResults.filter((r) => !r.correct && !r.isSkipped).length;

  const handleBackToStart = () => {
    reset();
    router.push("/quiz");
  };

  return (
    <Box p={4} maxW="sm" mx="auto">
      <Heading size="lg" mb={4}>
        Session Complete
      </Heading>

      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Total: {allResults.length} words</Text>
        <Text color="green.600" _dark={{ color: "green.300" }} fontWeight="bold">
          Correct: {correctCount}
        </Text>
        <Text color="red.600" _dark={{ color: "red.300" }} fontWeight="bold">
          Incorrect: {incorrectCount}
        </Text>
      </VStack>

      <QuizResultsGroupedList
        items={allResults}
        isEtymology={isEtymologyQuiz}
        onOverride={handleOverride}
        onUndo={handleUndo}
        onSkip={handleSkip}
        onResume={handleResume}
      />

      <Button w="full" colorPalette="blue" onClick={handleBackToStart}>
        Back to Start
      </Button>
    </Box>
  );
}
