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
import { useQuizStore, type ReverseFlashcard } from "@/store/quizStore";

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
    <Box
      p={4}
      maxW="md"
      mx="auto"
      display="flex"
      flexDirection="column"
      minH="100dvh"
    >
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

      <Box flex="1" overflowY="auto" mb={4}>
        {phase === "answering" ? (
          <VStack align="stretch" gap={4}>
            <Text fontWeight="medium" fontSize="sm" color="purple.600" _dark={{ color: "purple.300" }}>
              Meaning
            </Text>
            <Heading size="xl" textAlign="center" color="purple.700" _dark={{ color: "purple.300" }}>
              {card.meaning}
            </Heading>

            {card.contexts.length > 0 && (
              <>
                <Text fontWeight="medium" fontSize="sm" color="purple.600" _dark={{ color: "purple.300" }}>
                  Context (fill in the blank)
                </Text>
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
              </>
            )}
          </VStack>
        ) : (
          <VStack align="stretch" gap={4}>
            <Heading size="xl" textAlign="center" color="purple.700" _dark={{ color: "purple.300" }}>
              {card.meaning}
            </Heading>

            {loading ? (
              <Box textAlign="center" py={4}>
                <Text mb={2}>Your answer: {submittedAnswer}</Text>
                <Spinner size="lg" />
              </Box>
            ) : feedback ? (
              <VStack align="stretch" gap={3}>
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

                <Text
                  textDecoration={feedback.correct ? "none" : "line-through"}
                >
                  Your answer: {submittedAnswer}
                </Text>

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
                    <Text fontWeight="bold" mt={2}>
                      Context:
                    </Text>
                    {feedback.contexts.map((ctx, i) => (
                      <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>
                        {i + 1}. {ctx}
                      </Text>
                    ))}
                  </Box>
                )}
              </VStack>
            ) : error ? (
              <Text color="red.500">{error}</Text>
            ) : null}
          </VStack>
        )}
      </Box>

      <Box>
        {phase === "answering" ? (
          <Box display="flex" gap={2}>
            <Input
              ref={inputRef}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type the word"
              flex="1"
              autoFocus
              borderColor="purple.500"
            />
            <Button
              colorPalette="purple"
              onClick={handleSubmit}
              disabled={!answer.trim()}
            >
              Submit
            </Button>
          </Box>
        ) : (
          <Button
            w="full"
            colorPalette="purple"
            onClick={handleNext}
            onKeyDown={handleKeyDown}
            disabled={loading}
          >
            {currentIndex + 1 >= total ? "See Results" : "Next"}
          </Button>
        )}
      </Box>
    </Box>
  );
}
