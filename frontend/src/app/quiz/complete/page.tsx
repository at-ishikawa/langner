"use client";

import { useEffect, useMemo, useRef } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Box, Button, Text, VStack } from "@chakra-ui/react";
import { PageShell } from "@/components/PageShell";

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
} from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const quizType = useQuizStore((s) => s.quizType);
  const reset = useQuizStore((s) => s.reset);

  const allResults = useMemo((): ResultItem[] => {
    if (results.length > 0) return results.map(standardResultToItem);
    if (reverseResults.length > 0) return reverseResults.map(reverseResultToItem);
    if (freeformResults.length > 0) return freeformResults.map(freeformResultToItem);
    return [];
  }, [results, reverseResults, freeformResults]);

  // When the page loads with no results (e.g. navigated here directly), bounce
  // home. But NOT when the user clicks "Back to Start": that handler calls
  // reset() — which empties allResults — and then pushes to /quiz. Without this
  // guard the empty-results effect races that push and can win, landing the user
  // on "/" instead of /quiz (flaky on slow deploys; a real wrong-destination bug).
  const leavingToQuiz = useRef(false);

  useEffect(() => {
    if (allResults.length === 0 && !leavingToQuiz.current) {
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
    leavingToQuiz.current = true;
    reset();
    router.push("/quiz");
  };

  return (
    <PageShell title="Session Complete" maxW="xl" stickyFooter>
      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Total: {allResults.length} words</Text>
        <Text color="green.fg" fontWeight="bold">
          Correct: {correctCount}
        </Text>
        <Text color="red.fg" fontWeight="bold">
          Incorrect: {incorrectCount}
        </Text>
      </VStack>

      <QuizResultsGroupedList
        items={allResults}
        onOverride={handleOverride}
        onUndo={handleUndo}
        onSkip={handleSkip}
        onResume={handleResume}
      />

      <Box
        position="sticky"
        bottom={0}
        bg="bg"
        borderTopWidth="1px"
        borderTopColor="border"
        mx={-4}
        px={4}
        py={3}
        mt={6}
      >
        <Button w="full" colorPalette="brand" onClick={handleBackToStart}>
          Back to Start
        </Button>
        {incorrectCount > 0 && (
          <Box mt={2} textAlign="center">
            <Link
              href={`/history/${localTodayYYYYMMDD()}`}
              data-testid="review-wrong-link"
            >
              <Text fontSize="sm" color="brand.fg">
                Review what you got wrong today →
              </Text>
            </Link>
          </Box>
        )}
      </Box>
    </PageShell>
  );
}
