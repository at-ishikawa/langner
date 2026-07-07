"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Input, Spinner, Text } from "@chakra-ui/react";
import { quizClient, QuizType, type SubmitRelearnAnswerResponse } from "@/lib/client";
import { useRelearnStore } from "@/store/relearnStore";
import RelearnContext from "@/components/RelearnContext";

// sourceLabel names, for context only, which quiz originally produced the wrong
// answer that pooled this word. It never changes how the word is asked.
function sourceLabel(source: QuizType): string {
  switch (source) {
    case QuizType.REVERSE:
      return "missed in Reverse";
    case QuizType.FREEFORM:
      return "missed in Freeform";
    case QuizType.ETYMOLOGY_STANDARD:
    case QuizType.ETYMOLOGY_REVERSE:
    case QuizType.ETYMOLOGY_FREEFORM:
      return "missed in Etymology";
    default:
      return "missed in Notebook";
  }
}

type Phase = "answering" | "feedback";

export default function RelearnSessionPage() {
  const router = useRouter();
  const queue = useRelearnStore((s) => s.queue);
  const totalAnswers = useRelearnStore((s) => s.totalAnswers);
  const resolveFront = useRelearnStore((s) => s.resolveFront);

  const current = queue[0];
  const [phase, setPhase] = useState<Phase>("answering");
  const [answer, setAnswer] = useState("");
  const [feedback, setFeedback] = useState<SubmitRelearnAnswerResponse | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startRef = useRef<number>(Date.now());

  // Leaving the queue empty ends the session. A direct visit with no answers
  // yet bounces back to the start screen instead of a hollow complete page.
  useEffect(() => {
    if (queue.length === 0) {
      router.push(totalAnswers > 0 ? "/quiz/relearn/complete" : "/quiz/relearn");
    }
  }, [queue.length, totalAnswers, router]);

  // Reset the per-card timer whenever a new card reaches the front.
  useEffect(() => {
    startRef.current = Date.now();
  }, [current?.noteId]);

  if (!current) {
    return null;
  }

  const submit = async (isSkipped: boolean) => {
    setSubmitting(true);
    setError(null);
    setPhase("feedback");
    try {
      const res = await quizClient.submitRelearnAnswer({
        noteId: current.noteId,
        answer: isSkipped ? "" : answer,
        isSkipped,
        responseTimeMs: BigInt(Date.now() - startRef.current),
      });
      setFeedback(res);
    } catch {
      setError("Grading failed. Please try again.");
      setPhase("answering");
    } finally {
      setSubmitting(false);
    }
  };

  const handleNext = () => {
    const correct = feedback?.correct ?? false;
    setAnswer("");
    setFeedback(null);
    setPhase("answering");
    resolveFront(correct);
  };

  return (
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh" p={4}>
      <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mb={3} aria-live="polite">
        {queue.length} {queue.length === 1 ? "word" : "words"} left
      </Text>

      <Box textAlign="center" mb={4}>
        <Heading size="lg" data-testid="relearn-entry">{current.entry}</Heading>
        <Text fontSize="xs" color="gray.400" mt={1} aria-label={`originally ${sourceLabel(current.sourceQuizType)}`}>
          {sourceLabel(current.sourceQuizType)}
        </Text>
      </Box>

      {phase === "answering" ? (
        <Box display="flex" flexDirection="column" gap={3}>
          <Text fontSize="sm" fontWeight="medium">Your meaning:</Text>
          <Input
            autoFocus
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && answer.trim()) void submit(false);
            }}
            placeholder="Type the meaning"
          />
          {error && (
            <Text color="red.500" fontSize="sm" role="alert">{error}</Text>
          )}
          <Box display="flex" gap={2}>
            <Button variant="outline" flex={1} onClick={() => void submit(true)} aria-label="Skip and see the answer">
              Skip
            </Button>
            <Button colorPalette="purple" flex={1} onClick={() => void submit(false)} disabled={!answer.trim()}>
              Submit
            </Button>
          </Box>
        </Box>
      ) : (
        <Box display="flex" flexDirection="column" gap={3}>
          {submitting || !feedback ? (
            <Box textAlign="center" py={6}>
              <Spinner />
            </Box>
          ) : (
            <>
              <Box
                bg={feedback.correct ? "green.50" : "red.50"}
                color={feedback.correct ? "green.700" : "red.700"}
                _dark={{
                  bg: feedback.correct ? "green.900" : "red.900",
                  color: feedback.correct ? "green.200" : "red.200",
                }}
                borderRadius="md"
                p={3}
                fontWeight="semibold"
              >
                {feedback.correct ? "✓ Correct" : "✗ Incorrect"}
              </Box>
              <Box>
                <Text fontWeight="bold">{current.entry}</Text>
                <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }} data-testid="relearn-meaning">{feedback.meaning}</Text>
              </Box>
              {feedback.reason && (
                <Text fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
                  {feedback.reason}
                </Text>
              )}
              <RelearnContext
                entry={current.entry}
                scenes={feedback.contextScenes ?? []}
                exampleWords={feedback.exampleWords ?? []}
                graphContext={feedback.graphContext}
              />
              <Button colorPalette="purple" w="full" mt={2} onClick={handleNext}>
                Next
              </Button>
            </>
          )}
        </Box>
      )}
    </Box>
  );
}
