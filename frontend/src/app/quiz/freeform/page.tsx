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
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

export default function FreeformQuizPage() {
  const router = useRouter();
  const quizType = useQuizStore((s) => s.quizType);
  const wordCount = useQuizStore((s) => s.wordCount);
  const storeSubmitResult = useQuizStore((s) => s.submitFreeformResult);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const freeformExpressions = useQuizStore((s) => s.freeformExpressions);
  const freeformNextReviewDates = useQuizStore((s) => s.freeformNextReviewDates);
  const reset = useQuizStore((s) => s.reset);

  const [word, setWord] = useState("");
  const [meaning, setMeaning] = useState("");
  const [loading, setLoading] = useState(false);
  const [feedback, setFeedback] = useState<{
    correct: boolean;
    word: string;
    meaning: string;
    reason: string;
    notebookName: string;
    context?: string;
    learnedAt?: string;
    noteId?: bigint;
    images?: string[];
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
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
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitFreeformAnswer({
        word: word.trim(),
        meaning: meaning.trim(),
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: res.correct,
        word: res.word,
        meaning: res.meaning,
        reason: res.reason,
        notebookName: res.notebookName,
        context: res.context,
        learnedAt: res.learnedAt || undefined,
        noteId: res.noteId || undefined,
        images: res.images.length > 0 ? res.images : undefined,
      });
      setDisplayCorrect(res.correct);
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
        images: res.images.length > 0 ? res.images : undefined,
      });
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    setWord("");
    setMeaning("");
    setFeedback(null);
    setError(null);
    setOverridden(false);
    setSkipped(false);
    startTimeRef.current = Date.now();
    wordInputRef.current?.focus();
  };

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
      ) : feedback ? (
        <VStack align="stretch" gap={4}>
          <Box
            p={4}
            borderRadius="md"
            bg={displayCorrect ? "green.100" : "red.100"}
            _dark={{ bg: displayCorrect ? "green.900" : "red.900" }}
          >
            <Text fontWeight="bold" fontSize="lg">
              {displayCorrect ? "\u2713 Correct!" : "\u2717 Incorrect"}
            </Text>
          </Box>

          <Box>
            <Text fontWeight="bold">Word</Text>
            <Text fontSize="xl">{feedback.word}</Text>
          </Box>

          <Box>
            <Text fontWeight="bold">Correct meaning</Text>
            <Text fontStyle="italic">{feedback.meaning}</Text>
          </Box>

          {feedback.reason && (
            <Box>
              <Text fontWeight="bold">Reason</Text>
              <Text>{feedback.reason}</Text>
            </Box>
          )}

          {feedback.notebookName && (
            <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
              Found in: {feedback.notebookName}
            </Text>
          )}

          {feedback.context && (
            <Box>
              <Text fontWeight="bold">Context</Text>
              <Text fontStyle="italic">{feedback.context}</Text>
            </Box>
          )}

          {feedback.images && feedback.images.length > 0 && (
            <Box display="flex" gap={2} flexWrap="wrap">
              {feedback.images.map((src, i) => (
                <img key={i} src={src} alt="" style={{ maxHeight: "150px", borderRadius: "4px" }} />
              ))}
            </Box>
          )}

          <FeedbackActions
            isCorrect={displayCorrect}
            noteId={feedback.noteId}
            isOverridden={overridden}
            isSkipped={skipped}
            nextLabel="Next Word"
            onNext={handleNext}
            onOverride={async () => {
              if (!feedback.noteId || !feedback.learnedAt) return;
              try {
                const res = await quizClient.overrideAnswer({
                  noteId: feedback.noteId,
                  quizType: ProtoQuizType.FREEFORM,
                  learnedAt: feedback.learnedAt,
                  markCorrect: !displayCorrect,
                });
                setOverridden(true);
                setDisplayCorrect(!displayCorrect);
              } catch { /* silently fail */ }
            }}
            onSkip={async () => {
              if (!feedback.noteId) return;
              try {
                await quizClient.skipWord({ noteId: feedback.noteId });
                setSkipped(true);
              } catch { /* silently fail */ }
            }}
          />

          {freeformResults.length > 0 && (
            <Button
              colorPalette="green"
              variant="outline"
              onClick={() => router.push("/quiz/complete")}
            >
              See Results
            </Button>
          )}

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
