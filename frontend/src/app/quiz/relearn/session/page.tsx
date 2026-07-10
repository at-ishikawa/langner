"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Spinner, Text } from "@chakra-ui/react";
import { quizClient, QuizType, type SubmitRelearnAnswerResponse } from "@/lib/client";
import { AnswerInput } from "@/components/AnswerInput";
import { useRelearnStore } from "@/store/relearnStore";
import RelearnContext from "@/components/RelearnContext";

// sourceLabel names which quiz produced the wrong answer that pooled this card —
// and, now that relearn mirrors that quiz, which format it is presented in.
function sourceLabel(source: QuizType): string {
  switch (source) {
    case QuizType.REVERSE:
      return "Reverse — recall the word";
    case QuizType.ETYMOLOGY_STANDARD:
      return "Etymology — recall the meaning";
    case QuizType.ETYMOLOGY_REVERSE:
      return "Etymology — recall the origin";
    default:
      return "Recognition — recall the meaning";
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

  // Each card mirrors the quiz type it was failed in. For the reverse formats
  // the learner produces the word/origin from the meaning; otherwise they
  // recall the meaning from the word/origin.
  const isReverse =
    current.sourceQuizType === QuizType.REVERSE ||
    current.sourceQuizType === QuizType.ETYMOLOGY_REVERSE;
  const isEtymology =
    current.sourceQuizType === QuizType.ETYMOLOGY_STANDARD ||
    current.sourceQuizType === QuizType.ETYMOLOGY_REVERSE;
  const promptText = isReverse ? current.meaning : current.entry;
  const answerLabel = isReverse ? (isEtymology ? "The origin" : "The word") : "Your meaning";
  const answerPlaceholder = isReverse
    ? isEtymology
      ? "Type the origin"
      : "Type the word"
    : "Type the meaning";
  const etymologyBadge = [current.type, current.language].filter(Boolean).join(" · ");

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

      {/* Prompt card — mirrors the source quiz's format. */}
      <Box bg="white" _dark={{ bg: "gray.800" }} borderRadius="lg" borderWidth="1px" borderColor="gray.200" p={5} mb={4}>
        <Text fontSize="xs" color="purple.500" _dark={{ color: "purple.300" }} fontWeight="medium" mb={2}>
          {sourceLabel(current.sourceQuizType)}
        </Text>
        <Heading size="lg" textAlign="center" data-testid="relearn-prompt">
          {promptText}
        </Heading>
        {isEtymology && etymologyBadge && (
          <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} textAlign="center" mt={1}>
            {etymologyBadge}
          </Text>
        )}

        {/* Hints: examples for recognition, masked contexts for reverse. */}
        {!isReverse && current.examples.length > 0 && (
          <Box mt={3} display="flex" flexDirection="column" gap={1}>
            {current.examples.map((ex, i) => (
              <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }}>
                {ex.speaker ? `${ex.speaker}: ` : ""}
                {ex.text}
              </Text>
            ))}
          </Box>
        )}
        {isReverse && !isEtymology && current.contexts.length > 0 && (
          <Box mt={3} display="flex" flexDirection="column" gap={1}>
            {current.contexts.map((c, i) => (
              <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }}>
                {c.maskedContext || c.context}
              </Text>
            ))}
          </Box>
        )}
      </Box>

      {phase === "answering" ? (
        <Box display="flex" flexDirection="column" gap={3}>
          <AnswerInput
            label={answerLabel}
            value={answer}
            onChange={setAnswer}
            onSubmit={() => void submit(false)}
            onSkip={() => void submit(true)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && answer.trim()) void submit(false);
            }}
            placeholder={answerPlaceholder}
          />
          {error && (
            <Text color="red.500" fontSize="sm" role="alert">
              {error}
            </Text>
          )}
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
              {/* Always show the word and its meaning, whichever side was asked. */}
              <Box>
                <Text fontWeight="bold" data-testid={isReverse ? "relearn-answer" : undefined}>
                  {current.entry}
                </Text>
                <Text
                  fontSize="sm"
                  color="gray.600"
                  _dark={{ color: "gray.300" }}
                  data-testid={isReverse ? undefined : "relearn-answer"}
                >
                  {feedback.meaning || current.meaning}
                </Text>
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
