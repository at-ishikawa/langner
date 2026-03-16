"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Flex,
  Heading,
  Input,
  Progress,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore, type OriginalValues } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

type QuizPhase = "answering" | "feedback";

interface FeedbackData {
  correct: boolean;
  meaning: string;
  reason: string;
  nextReviewDate: string;
  learnedAt: string;
}

export default function QuizCardPage() {
  const router = useRouter();
  const flashcards = useQuizStore((s) => s.flashcards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [submittedAnswer, setSubmittedAnswer] = useState("");
  const [feedback, setFeedback] = useState<FeedbackData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  // Override/skip local state for current card (used for banner display)
  const [isOverridden, setIsOverridden] = useState(false);
  const [isSkipped, setIsSkipped] = useState(false);

  useEffect(() => {
    if (flashcards.length === 0 || quizType !== "standard") {
      router.push("/");
    }
  }, [flashcards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setSubmittedAnswer("");
    setFeedback(null);
    setIsOverridden(false);
    setIsSkipped(false);
    setTimeout(() => inputRef.current?.focus(), 50);
  }, [currentIndex]);

  if (flashcards.length === 0) {
    return null;
  }

  const card = flashcards[currentIndex];
  const total = flashcards.length;
  const progress = ((currentIndex + 1) / total) * 100;

  const handleSubmit = async () => {
    if (!answer.trim()) return;

    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    setSubmittedAnswer(userAnswer);
    setAnswer("");
    setPhase("feedback");
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitAnswer({
        noteId: card.noteId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback(res);
      storeSubmitResult({
        noteId: card.noteId,
        entry: card.entry,
        answer: userAnswer,
        correct: res.correct,
        meaning: res.meaning,
        reason: res.reason,
        contexts: card.examples.map((ex) => ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`),
        wordDetail: res.wordDetail,
        nextReviewDate: res.nextReviewDate,
        learnedAt: res.learnedAt,
      });
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleSkip = async () => {
    const responseTimeMs = Date.now() - startTimeRef.current;
    setSubmittedAnswer("");
    setAnswer("");
    setPhase("feedback");
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitAnswer({
        noteId: card.noteId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback(res);
      storeSubmitResult({
        noteId: card.noteId,
        entry: card.entry,
        answer: "(skipped)",
        correct: false,
        meaning: res.meaning,
        reason: res.reason,
        contexts: card.examples.map((ex) => ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`),
        wordDetail: res.wordDetail,
        nextReviewDate: res.nextReviewDate,
        learnedAt: res.learnedAt,
      });
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    if (currentIndex + 1 >= total) {
      router.push("/quiz/complete");
    } else {
      nextCard();
    }
  };

  const handleOverrideResult = (newCorrect: boolean, newNextReviewDate: string, originalValues: OriginalValues) => {
    if (!feedback) return;
    useQuizStore.getState().overrideResult(currentIndex, "standard", newNextReviewDate, originalValues);
    setFeedback({ ...feedback, correct: newCorrect });
    setIsOverridden(true);
  };

  const handleUndoResult = (correct: boolean, nextReviewDate: string) => {
    if (!feedback) return;
    useQuizStore.getState().undoOverrideResult(currentIndex, "standard", correct, nextReviewDate);
    setFeedback({ ...feedback, correct });
    setIsOverridden(false);
  };

  const handleSkipResult = () => {
    useQuizStore.getState().skipResult(currentIndex, "standard");
    setIsSkipped(true);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      if (phase === "answering") {
        handleSubmit();
      } else if (phase === "feedback" && !loading) {
        handleNext();
      }
    }
  };

  return (
    <Box p={4} maxW="md" mx="auto">
      <Box mb={4}>
        <Text fontSize="sm" mb={1}>
          {currentIndex + 1} / {total}
        </Text>
        <Progress.Root value={progress} size="sm">
          <Progress.Track>
            <Progress.Range />
          </Progress.Track>
        </Progress.Root>
      </Box>

      {phase === "answering" ? (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center">
            {card.entry}
          </Heading>

          {card.examples.length > 0 && (
            <VStack align="stretch" gap={2}>
              {card.examples.map((ex, i) => (
                <Text key={i} fontSize="md" color="fg.muted">
                  {ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`}
                </Text>
              ))}
            </VStack>
          )}

          <Box>
            <Text fontWeight="medium" mb={1}>
              Meaning
            </Text>
            <Input
              ref={inputRef}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type your answer"
              size="lg"
            />
          </Box>

          <Button
            colorPalette="blue"
            onClick={handleSubmit}
            disabled={!answer.trim()}
            size="lg"
          >
            Submit
          </Button>

          <Button
            variant="outline"
            onClick={handleSkip}
            size="lg"
          >
            Don&apos;t Know
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center">
            {card.entry}
          </Heading>

          {loading ? (
            <Box textAlign="center" py={8}>
              <Spinner size="lg" mb={4} />
              <Text>Checking your answer...</Text>
            </Box>
          ) : feedback ? (
            <>
              <Box
                p={3}
                borderRadius="md"
                bg={feedback.correct ? "green.100" : "red.100"}
                color={feedback.correct ? "green.800" : "red.800"}
                _dark={{
                  bg: feedback.correct ? "green.900" : "red.900",
                  color: feedback.correct ? "green.200" : "red.200",
                }}
              >
                <Flex alignItems="center" gap={2}>
                  <Text fontWeight="bold">
                    {feedback.correct ? "\u2713 Correct" : "\u2717 Incorrect"}
                  </Text>
                  {isOverridden && (
                    <Text fontSize="sm" fontStyle="italic">(overridden)</Text>
                  )}
                  {isSkipped && (
                    <Text fontSize="sm" fontStyle="italic">(skipped)</Text>
                  )}
                </Flex>
              </Box>

              {submittedAnswer ? (
                <Text textDecoration={feedback.correct ? "none" : "line-through"}>
                  Your answer: {submittedAnswer}
                </Text>
              ) : (
                <Text color="gray.500" _dark={{ color: "gray.400" }} fontStyle="italic">
                  Skipped
                </Text>
              )}

              <Box>
                <Text fontWeight="bold">Meaning</Text>
                <Text>{feedback.meaning}</Text>
              </Box>

              {feedback.reason && (
                <Box>
                  <Text fontWeight="bold">Reason</Text>
                  <Text>{feedback.reason}</Text>
                </Box>
              )}

              {card.examples.length > 0 && (
                <Box>
                  <Text fontWeight="bold">Examples</Text>
                  <VStack align="stretch" gap={1} mt={1}>
                    {card.examples.map((ex, i) => (
                      <Text key={i} fontSize="sm" color="fg.muted" fontStyle="italic">
                        {ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`}
                      </Text>
                    ))}
                  </VStack>
                </Box>
              )}

              <FeedbackActions
                key={currentIndex}
                noteId={card.noteId}
                quizType="standard"
                learnedAt={feedback.learnedAt}
                correct={feedback.correct}
                nextReviewDate={feedback.nextReviewDate}
                onOverride={handleOverrideResult}
                onUndo={handleUndoResult}
                onSkip={handleSkipResult}
              />

              <Button
                w="full"
                colorPalette="blue"
                onClick={handleNext}
              >
                {currentIndex + 1 >= total ? "See Results" : "Next"}
              </Button>
            </>
          ) : error ? (
            <>
              <Text color="red.500">{error}</Text>
              <Button
                w="full"
                colorPalette="blue"
                variant="outline"
                onClick={() => {
                  setPhase("answering");
                  setError(null);
                  setAnswer(submittedAnswer);
                  setTimeout(() => inputRef.current?.focus(), 50);
                }}
              >
                Retry
              </Button>
              <Button
                w="full"
                colorPalette="blue"
                onClick={handleNext}
              >
                {currentIndex + 1 >= total ? "See Results" : "Skip"}
              </Button>
            </>
          ) : null}
        </VStack>
      )}
    </Box>
  );
}
