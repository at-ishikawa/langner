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
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

type QuizPhase = "answering" | "feedback";

export default function EtymologyAssemblyPage() {
  const router = useRouter();
  const etymologyCards = useQuizStore((s) => s.etymologyCards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyResult);
  const storeSkipResult = useQuizStore((s) => s.skipResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{
    correct: boolean;
    reason: string;
    correctExpression: string;
    relatedDefinitions: Array<{ expression: string; meaning: string; notebookName: string }>;
    nextReviewDate?: string;
    learnedAt?: string;
    noteId?: bigint;
  } | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (etymologyCards.length === 0 || quizType !== "etymology-assembly") {
      router.push("/");
    }
  }, [etymologyCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setFeedback(null);
    setOverridden(false);
    setSkipped(false);
    setDisplayCorrect(false);
    setTimeout(() => inputRef.current?.focus(), 50);
  }, [currentIndex]);

  if (etymologyCards.length === 0) return null;

  const card = etymologyCards[currentIndex];
  const total = etymologyCards.length;
  const progress = ((currentIndex + 1) / total) * 100;

  const handleSubmit = async () => {
    if (!answer.trim()) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    setAnswer("");
    setPhase("feedback");
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitEtymologyAssemblyAnswer({
        cardId: card.cardId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      const fb = {
        correct: res.correct,
        reason: res.reason,
        correctExpression: res.correctExpression,
        relatedDefinitions: (res.relatedDefinitions ?? []).map((d) => ({
          expression: d.expression,
          meaning: d.meaning,
          notebookName: d.notebookName,
        })),
        nextReviewDate: res.nextReviewDate || undefined,
        learnedAt: res.learnedAt || undefined,
        noteId: res.noteId ? BigInt(res.noteId) : undefined,
      };

      setFeedback(fb);
      setDisplayCorrect(res.correct);
      storeSubmitResult({
        noteId: fb.noteId,
        cardId: card.cardId,
        expression: card.expression,
        meaning: card.meaning,
        answer: userAnswer,
        correct: res.correct,
        reason: res.reason,
        originGrades: [],
        relatedDefinitions: fb.relatedDefinitions,
        originParts: card.originParts,
        nextReviewDate: fb.nextReviewDate,
        learnedAt: fb.learnedAt,
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
          <Box p={4} borderWidth="1px" borderRadius="md">
            <Text fontWeight="medium" mb={2}>Origins:</Text>
            <Box display="flex" gap={2} alignItems="center" flexWrap="wrap">
              {card.originParts.map((p, i) => (
                <Box key={i} display="flex" alignItems="center" gap={1}>
                  {i > 0 && <Text fontSize="lg" color="fg.muted">+</Text>}
                  <Box
                    p={2}
                    borderWidth="1px"
                    borderRadius="md"
                    bg="blue.50"
                    _dark={{ bg: "blue.900/20" }}
                  >
                    <Text fontWeight="semibold" color="#2563eb">{p.origin}</Text>
                    <Text fontSize="sm" color="fg.muted">{p.meaning}</Text>
                    {p.language && (
                      <Text fontSize="xs" color="fg.subtle">{p.language}</Text>
                    )}
                  </Box>
                </Box>
              ))}
            </Box>
          </Box>

          <Text fontSize="lg" textAlign="center" color="fg.muted">=</Text>

          <Box>
            <Text fontWeight="medium" mb={1}>Word</Text>
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
            position="sticky"
            bottom={4}
          >
            Submit
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Box p={4} borderWidth="1px" borderRadius="md">
            <Text fontWeight="medium" mb={2}>Origins:</Text>
            <Box display="flex" gap={2} alignItems="center" flexWrap="wrap">
              {card.originParts.map((p, i) => (
                <Box key={i} display="flex" alignItems="center" gap={1}>
                  {i > 0 && <Text fontSize="lg" color="fg.muted">+</Text>}
                  <Box p={2} borderWidth="1px" borderRadius="md">
                    <Text fontWeight="semibold" color="#2563eb">{p.origin}</Text>
                    <Text fontSize="sm" color="fg.muted">{p.meaning}</Text>
                  </Box>
                </Box>
              ))}
            </Box>
          </Box>

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
                bg={displayCorrect ? "green.100" : "red.100"}
                color={displayCorrect ? "green.800" : "red.800"}
                _dark={{
                  bg: displayCorrect ? "green.900" : "red.900",
                  color: displayCorrect ? "green.200" : "red.200",
                }}
              >
                <Text fontWeight="bold">
                  {displayCorrect ? "\u2713 Correct" : "\u2717 Incorrect"}
                  {overridden && (
                    <Text as="span" fontWeight="normal" fontStyle="italic"> (overridden)</Text>
                  )}
                </Text>
              </Box>

              <Box>
                <Text fontWeight="bold">Correct answer</Text>
                <Heading size="lg">{feedback.correctExpression}</Heading>
              </Box>

              {feedback.reason && (
                <Box>
                  <Text fontWeight="bold">Reason</Text>
                  <Text>{feedback.reason}</Text>
                </Box>
              )}

              {feedback.relatedDefinitions.length > 0 && (
                <Box>
                  <Text fontWeight="bold" mb={1}>Related words:</Text>
                  <VStack align="stretch" gap={1}>
                    {feedback.relatedDefinitions.map((d, i) => (
                      <Text key={i} fontSize="sm">
                        <Text as="span" fontWeight="medium">{d.expression}</Text>
                        {" - "}{d.meaning}
                      </Text>
                    ))}
                  </VStack>
                </Box>
              )}

              <FeedbackActions
                isCorrect={displayCorrect}
                noteId={feedback.noteId}
                nextReviewDate={feedback.nextReviewDate}
                isOverridden={overridden}
                isSkipped={skipped}
                nextLabel={currentIndex + 1 >= total ? "See Results" : "Next"}
                onNext={handleNext}
                onOverride={feedback.noteId ? async () => {
                  try {
                    const res = await quizClient.overrideAnswer({
                      noteId: feedback.noteId!,
                      quizType: ProtoQuizType.ETYMOLOGY_ASSEMBLY,
                      learnedAt: feedback.learnedAt!,
                      markCorrect: !displayCorrect,
                    });
                    setOverridden(true);
                    setDisplayCorrect(!displayCorrect);
                    setFeedback((prev) => prev ? { ...prev, nextReviewDate: res.nextReviewDate || undefined } : prev);
                  } catch { /* silently fail */ }
                } : undefined}
                onSkip={feedback.noteId ? async () => {
                  try {
                    await quizClient.skipWord({ noteId: feedback.noteId! });
                    setSkipped(true);
                    storeSkipResult(currentIndex, "etymology-assembly");
                  } catch { /* silently fail */ }
                } : undefined}
                onChangeReviewDate={feedback.noteId && feedback.learnedAt ? async (newDate) => {
                  try {
                    await quizClient.overrideAnswer({
                      noteId: feedback.noteId!,
                      quizType: ProtoQuizType.ETYMOLOGY_ASSEMBLY,
                      learnedAt: feedback.learnedAt!,
                      nextReviewDate: newDate,
                    });
                    setFeedback((prev) => prev ? { ...prev, nextReviewDate: newDate } : prev);
                  } catch { /* silently fail */ }
                } : undefined}
              />
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
                }}
              >
                Retry
              </Button>
              <Button w="full" colorPalette="blue" onClick={handleNext}>
                {currentIndex + 1 >= total ? "See Results" : "Skip"}
              </Button>
            </>
          ) : null}
        </VStack>
      )}
    </Box>
  );
}
