"use client";

import { useEffect, useMemo } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";

// Today as YYYY-MM-DD in the browser's local zone. The backend buckets
// learning records by the time's stored zone (server-local), so using
// toISOString() here — which is UTC — would point a westward user at
// tomorrow and they'd see an empty Day Detail.
function localTodayYYYYMMDD(): string {
  const d = new Date();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}
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

      <Box pb={20}>
        <QuizResultsGroupedList
          items={allResults}
          isEtymology={isEtymologyQuiz}
          onOverride={handleOverride}
          onUndo={handleUndo}
          onSkip={handleSkip}
          onResume={handleResume}
        />
      </Box>

      <Box
        position="sticky"
        bottom={0}
        bg="white"
        _dark={{ bg: "gray.900", borderTopColor: "gray.700" }}
        borderTopWidth="1px"
        borderTopColor="gray.200"
        mx={-4}
        px={4}
        py={3}
      >
        <Button w="full" colorPalette="blue" onClick={handleBackToStart}>
          Back to Start
        </Button>
        {incorrectCount > 0 && (
          <Box mt={2} textAlign="center">
            <Link
              href={`/history/${localTodayYYYYMMDD()}`}
              data-testid="review-wrong-link"
            >
              <Text fontSize="sm" color="blue.500">
                Review what you got wrong today →
              </Text>
            </Link>
          </Box>
        )}
      </Box>
    </Box>
  );
}
