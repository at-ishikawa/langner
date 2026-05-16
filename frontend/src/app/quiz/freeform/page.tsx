"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Input,
  Spinner,
  Text,
  Textarea,
  VStack,
} from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { BatchFeedback } from "@/components/BatchFeedback";
import { freeformResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

export default function FreeformQuizPage() {
  const router = useRouter();
  const quizType = useQuizStore((s) => s.quizType);
  const wordCount = useQuizStore((s) => s.wordCount);
  const storeSubmitResult = useQuizStore((s) => s.submitFreeformResult);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const freeformExpressions = useQuizStore((s) => s.freeformExpressions);
  const freeformNextReviewDates = useQuizStore((s) => s.freeformNextReviewDates);
  const recordFreeformAnswered = useQuizStore((s) => s.recordFreeformAnswered);
  const reset = useQuizStore((s) => s.reset);

  const { handleOverride, handleUndo, handleSkip, handleResume } =
    useQuizResultActions("freeform");

  const [word, setWord] = useState("");
  const [meaning, setMeaning] = useState("");
  const [loading, setLoading] = useState(false);
  // showFeedback flips true after a successful submit and false after
  // the user clicks Continue. We read the actual result from
  // freeformResults[last] so per-result override/undo/skip from
  // useQuizResultActions stays the single source of truth — same shape
  // the complete page uses.
  const [showFeedback, setShowFeedback] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef(Date.now());
  const wordInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (quizType !== "freeform") {
      router.push("/");
    }
    wordInputRef.current?.focus();
  }, [quizType, router]);

  // Returns: null (no input), false (not found), true (found & due), string (found but not due - the date)
  const wordStatus: null | boolean | string = useMemo(() => {
    const trimmed = word.trim();
    if (!trimmed || freeformExpressions.length === 0) return null;
    const lower = trimmed.toLowerCase();
    const found = freeformExpressions.some((e) => e.toLowerCase() === lower);
    if (!found) return false;
    const nextReview = freeformNextReviewDates[lower];
    if (nextReview) return nextReview;
    return true;
  }, [word, freeformExpressions, freeformNextReviewDates]);

  const handleSubmit = async () => {
    if (!word.trim() || !meaning.trim()) return;

    const responseTimeMs = Date.now() - startTimeRef.current;
    setLoading(true);
    setShowFeedback(false);
    setError(null);

    try {
      const res = await quizClient.submitFreeformAnswer({
        word: word.trim(),
        meaning: meaning.trim(),
        responseTimeMs: BigInt(responseTimeMs),
      });

      storeSubmitResult({
        word: res.word,
        answer: meaning.trim(),
        correct: res.correct,
        meaning: res.meaning,
        reason: res.reason,
        notebookName: res.notebookName,
        contexts: res.context ? [res.context] : [],
        wordDetail: res.wordDetail,
        learnedAt: res.learnedAt || undefined,
        noteId: res.noteId || undefined,
        images: (res.images ?? []).length > 0 ? res.images : undefined,
      });
      setShowFeedback(true);
      // Mark this word as answered for the current session so the user
      // cannot re-submit the same word until its next review date.
      recordFreeformAnswered(word.trim(), res.nextReviewDate);
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    setWord("");
    setMeaning("");
    setShowFeedback(false);
    setError(null);
    startTimeRef.current = Date.now();
    wordInputRef.current?.focus();
  };

  // Latest result rendered as a one-element batch. Mirrors how
  // standard/reverse quizzes feed BatchFeedback so per-result actions
  // (override/undo/skip/resume) live in one component across all modes.
  const latestIndex = freeformResults.length - 1;
  const batchItems = useMemo(
    () =>
      showFeedback && latestIndex >= 0
        ? [freeformResultToItem(freeformResults[latestIndex], latestIndex)]
        : [],
    [showFeedback, freeformResults, latestIndex],
  );

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && e.shiftKey) {
      handleSubmit();
    }
  };

  return (
    <Box p={4} maxW="md" mx="auto" minH="100dvh">
      <Heading size="lg" mb={4}>
        Freeform Quiz
      </Heading>

      <Text mb={4} color="gray.600" _dark={{ color: "gray.400" }}>
        Type any word you&apos;re learning and its meaning
      </Text>

      {loading ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>Checking your answer...</Text>
        </Box>
      ) : error ? (
        <VStack align="stretch" gap={4}>
          <Text color="red.500">{error}</Text>
          <Button
            w="full"
            colorPalette="blue"
            variant="outline"
            onClick={() => {
              setError(null);
              wordInputRef.current?.focus();
            }}
          >
            Retry
          </Button>
        </VStack>
      ) : showFeedback && batchItems.length > 0 ? (
        <VStack align="stretch" gap={4}>
          <BatchFeedback
            items={batchItems}
            isEtymology={false}
            isFinal={false}
            onContinue={handleNext}
            onSeeResults={() => router.push("/quiz/complete")}
            onOverride={handleOverride}
            onUndo={handleUndo}
            onSkip={handleSkip}
            onResume={handleResume}
          />

          <Button
            variant="ghost"
            onClick={() => {
              reset();
              router.push("/");
            }}
          >
            Back to Start
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Box>
            <Text fontWeight="medium" mb={1}>
              Word
            </Text>
            <Input
              ref={wordInputRef}
              value={word}
              onChange={(e) => setWord(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="e.g., hit the hay"
              size="lg"
            />
            {wordStatus === true && (
              <Text fontSize="sm" color="green.500" mt={1}>
                Found in notebooks
              </Text>
            )}
            {wordStatus === false && (
              <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} mt={1}>
                Word not found in notebooks
              </Text>
            )}
            {typeof wordStatus === "string" && (
              <Text fontSize="sm" color="orange.500" _dark={{ color: "orange.300" }} mt={1}>
                Not due until {wordStatus}
              </Text>
            )}
          </Box>

          <Box>
            <Text fontWeight="medium" mb={1}>
              Meaning
            </Text>
            <Textarea
              value={meaning}
              onChange={(e) => setMeaning(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="e.g., to go to bed; to sleep"
              size="lg"
              rows={2}
            />
          </Box>

          {error && <Text color="red.500">{error}</Text>}

          <Button
            colorPalette="blue"
            onClick={handleSubmit}
            disabled={!word.trim() || !meaning.trim() || wordStatus === false || typeof wordStatus === "string"}
            size="lg"
          >
            Check Answer
          </Button>

          {freeformResults.length > 0 && (
            <Button
              colorPalette="green"
              variant="outline"
              onClick={() => router.push("/quiz/complete")}
            >
              See Results
            </Button>
          )}

          <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} textAlign="center">
            {wordCount} words available in your notebooks
          </Text>
        </VStack>
      )}
    </Box>
  );
}
