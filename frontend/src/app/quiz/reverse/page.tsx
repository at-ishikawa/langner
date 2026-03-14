"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Input,
  Progress,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

type QuizPhase = "answering" | "feedback";

interface FeedbackData {
  correct: boolean;
  expression: string;
  meaning: string;
  reason: string;
  contexts: string[];
}

export default function ReverseQuizPage() {
  const router = useRouter();
  const reverseFlashcards = useQuizStore((s) => s.reverseFlashcards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitReverseResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [submittedAnswer, setSubmittedAnswer] = useState("");
  const [feedback, setFeedback] = useState<FeedbackData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (reverseFlashcards.length === 0 || quizType !== "reverse") {
      router.push("/");
    }
  }, [reverseFlashcards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setSubmittedAnswer("");
    setFeedback(null);
    setTimeout(() => inputRef.current?.focus(), 100);
  }, [currentIndex]);

  if (reverseFlashcards.length === 0) {
    return null;
  }

  const card = reverseFlashcards[currentIndex];
  const total = reverseFlashcards.length;
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
      const res = await quizClient.submitReverseAnswer({
        noteId: card.noteId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: res.correct,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
      });
      storeSubmitResult({
        noteId: card.noteId,
        answer: userAnswer,
        correct: res.correct,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
        wordDetail: res.wordDetail,
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
      const res = await quizClient.submitReverseAnswer({
        noteId: card.noteId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: false,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
      });
      storeSubmitResult({
        noteId: card.noteId,
        answer: "(skipped)",
        correct: false,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
        wordDetail: res.wordDetail,
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
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {card.meaning}
          </Heading>

          {card.contexts.length > 0 && (
            <VStack align="stretch" gap={2}>
              {card.contexts.map((ctx, i) => (
                <Text
                  key={i}
                  fontSize="md"
                  color="gray.600"
                  _dark={{ color: "gray.400" }}
                  fontStyle="italic"
                >
                  {ctx.maskedContext}
                </Text>
              ))}
            </VStack>
          )}

          <Box>
            <Text fontWeight="medium" mb={1}>
              Word
            </Text>
            <Input
              ref={inputRef}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type the word"
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
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {card.meaning}
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
                <Text fontWeight="bold">
                  {feedback.correct ? "\u2713 Correct" : "\u2717 Incorrect"}
                </Text>
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
                <Text fontWeight="bold">Word</Text>
                <Text fontStyle="italic">{feedback.expression}</Text>
              </Box>

              {feedback.reason && (
                <Box>
                  <Text fontWeight="bold">Reason</Text>
                  <Text>{feedback.reason}</Text>
                </Box>
              )}

              {feedback.contexts.length > 0 && (
                <Box>
                  <Text fontWeight="bold">Context</Text>
                  <VStack align="stretch" gap={1} mt={1}>
                    {feedback.contexts.map((ctx, i) => (
                      <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }} fontStyle="italic">
                        {i + 1}. {ctx}
                      </Text>
                    ))}
                  </VStack>
                </Box>
              )}

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
