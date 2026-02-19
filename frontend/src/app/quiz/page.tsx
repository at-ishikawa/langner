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
import { submitAnswer } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

type QuizPhase = "answering" | "feedback";

interface FeedbackData {
  correct: boolean;
  meaning: string;
  reason: string;
}

export default function QuizCardPage() {
  const router = useRouter();
  const flashcards = useQuizStore((s) => s.flashcards);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [submittedAnswer, setSubmittedAnswer] = useState("");
  const [feedback, setFeedback] = useState<FeedbackData | null>(null);
  const [loading, setLoading] = useState(false);
  const startTimeRef = useRef(Date.now());

  useEffect(() => {
    if (flashcards.length === 0) {
      router.push("/");
    }
  }, [flashcards, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
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

    try {
      const res = await submitAnswer({
        noteId: card.noteId.toString(),
        answer: userAnswer,
        responseTimeMs: responseTimeMs.toString(),
      });

      setFeedback(res);
      storeSubmitResult({
        noteId: card.noteId,
        entry: card.entry,
        answer: userAnswer,
        correct: res.correct,
        meaning: res.meaning,
        reason: res.reason,
      });
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    if (currentIndex + 1 >= total) {
      router.push("/quiz/complete");
    } else {
      nextCard();
      setPhase("answering");
      setFeedback(null);
      setSubmittedAnswer("");
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
      {/* Progress */}
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

      {/* Content area - scrollable */}
      <Box flex="1" overflowY="auto" mb={4}>
        {phase === "answering" ? (
          <VStack align="stretch" gap={4}>
            <Heading size="xl" textAlign="center">
              {card.entry}
            </Heading>
            {card.examples.length > 0 && (
              <VStack align="stretch" gap={2}>
                {card.examples.map((ex, i) => (
                  <Text key={i} fontSize="md" color="fg.muted">
                    {ex.speaker
                      ? `${ex.speaker}: "${ex.text}"`
                      : `"${ex.text}"`}
                  </Text>
                ))}
              </VStack>
            )}
          </VStack>
        ) : (
          <VStack align="stretch" gap={4}>
            <Heading size="xl" textAlign="center">
              {card.entry}
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
                >
                  <Text fontWeight="bold">
                    {feedback.correct ? "\u2713 Correct" : "\u2717 Incorrect"}
                  </Text>
                </Box>

                <Text
                  textDecoration={
                    feedback.correct ? "none" : "line-through"
                  }
                >
                  Your answer: {submittedAnswer}
                </Text>

                <Box>
                  <Text fontWeight="bold">Meaning</Text>
                  <Text>{feedback.meaning}</Text>
                </Box>

                <Box>
                  <Text fontWeight="bold">Reason</Text>
                  <Text>{feedback.reason}</Text>
                </Box>
              </VStack>
            ) : null}
          </VStack>
        )}
      </Box>

      {/* Input area - fixed at bottom */}
      <Box>
        {phase === "answering" ? (
          <Box display="flex" gap={2}>
            <Input
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type your answer"
              flex="1"
              autoFocus
            />
            <Button
              colorPalette="blue"
              onClick={handleSubmit}
              disabled={!answer.trim()}
            >
              Submit
            </Button>
          </Box>
        ) : (
          <Button
            w="full"
            colorPalette="blue"
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
