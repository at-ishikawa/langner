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

function getTypeBadgeColors(type: string): { bg: string; color: string } {
  switch (type.toLowerCase()) {
    case "root":
      return { bg: "#dbeafe", color: "#2563eb" };
    case "prefix":
      return { bg: "#fef3c7", color: "#92400e" };
    case "suffix":
      return { bg: "#dcfce7", color: "#166534" };
    default:
      return { bg: "#f3f4f6", color: "#666" };
  }
}

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
  const [submittedAnswer, setSubmittedAnswer] = useState("");
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
    setSubmittedAnswer("");
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
    setSubmittedAnswer(userAnswer);
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
    <Box p={4} maxW="sm" mx="auto">
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
          {/* Origins card */}
          <Box p={4} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
            <Text fontWeight="medium" mb={3} fontSize="sm" textAlign="center">
              What word is made from these origins?
            </Text>
            <VStack align="stretch" gap={2}>
              {card.originParts.map((p, i) => (
                <Box key={i}>
                  {i > 0 && (
                    <Text textAlign="center" fontSize="lg" color="fg.muted" my={1}>+</Text>
                  )}
                  <Box
                    p={2}
                    borderRadius="md"
                    bg="blue.50"
                    _dark={{ bg: "blue.900/20" }}
                    display="flex"
                    alignItems="center"
                    gap={2}
                    flexWrap="wrap"
                  >
                    <Text fontWeight="semibold" color="#2563eb" fontSize="lg">{p.origin}</Text>
                    <Text fontSize="sm" color="fg.muted">= {p.meaning}</Text>
                    <Box ml="auto" px={2} py={0.5} borderRadius="full" bg="#f3f4f6">
                      <Text fontSize="xs" color="#666">{p.language}</Text>
                    </Box>
                  </Box>
                </Box>
              ))}
            </VStack>
          </Box>

          <Text fontSize="xl" textAlign="center" color="fg.muted">=</Text>

          <Box>
            <Text fontWeight="medium" mb={1} fontSize="sm">Your answer</Text>
            <Input
              ref={inputRef}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="type the word..."
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
          {loading ? (
            <Box textAlign="center" py={8}>
              <Spinner size="lg" mb={4} />
              <Text>Checking your answer...</Text>
            </Box>
          ) : feedback ? (
            <>
              {/* Correct/Incorrect banner */}
              <Box
                p={3}
                borderRadius="md"
                bg={displayCorrect ? "green.100" : "red.100"}
                color={displayCorrect ? "green.800" : "red.800"}
                _dark={{
                  bg: displayCorrect ? "green.900" : "red.900",
                  color: displayCorrect ? "green.200" : "red.200",
                }}
                textAlign="center"
              >
                <Text fontWeight="bold" fontSize="md">
                  {displayCorrect ? "\u2713 Correct" : "\u2717 Incorrect"}
                  {overridden && (
                    <Text as="span" fontWeight="normal" fontStyle="italic"> (overridden)</Text>
                  )}
                </Text>
              </Box>

              {/* Word card */}
              <Box p={4} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                <Text fontSize="xl" fontWeight="bold">{card.expression}</Text>
                <Text fontSize="sm" color="fg.muted">{card.meaning}</Text>
              </Box>

              {/* Your answer */}
              {submittedAnswer && (
                <Box>
                  <Text fontWeight="medium" fontSize="sm" mb={1}>Your answer</Text>
                  <Box
                    p={3}
                    borderWidth="1.5px"
                    borderRadius="lg"
                    borderColor={displayCorrect ? "#16a34a" : "#dc2626"}
                    bg="white"
                    _dark={{ bg: "gray.800" }}
                    display="flex"
                    justifyContent="space-between"
                    alignItems="center"
                  >
                    <Text
                      textDecoration={displayCorrect ? "none" : "line-through"}
                      color={displayCorrect ? undefined : "#dc2626"}
                    >
                      {submittedAnswer}
                    </Text>
                    <Text
                      fontWeight="medium"
                      color={displayCorrect ? "#16a34a" : "#dc2626"}
                    >
                      {displayCorrect ? "\u2713" : "\u2717"}
                    </Text>
                  </Box>
                </Box>
              )}

              {/* Correct answer */}
              {!displayCorrect && feedback.correctExpression && (
                <Box>
                  <Text fontWeight="medium" fontSize="sm" mb={1}>Correct answer</Text>
                  <Heading size="lg">{feedback.correctExpression}</Heading>
                </Box>
              )}

              {/* Full breakdown with badges */}
              <Box>
                <Text fontWeight="medium" fontSize="sm" mb={1}>
                  {displayCorrect ? "Full breakdown" : "Correct breakdown"}
                </Text>
                <Box p={3} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                  <VStack align="stretch" gap={2}>
                    {card.originParts.map((p, i) => {
                      const typeBadge = getTypeBadgeColors(p.type);
                      return (
                        <Box key={i} display="flex" gap={2} alignItems="center" flexWrap="wrap">
                          <Text color="#2563eb" fontWeight="medium" fontSize="sm">{p.origin}</Text>
                          <Text fontSize="sm" color="fg.muted">= {p.meaning}</Text>
                          <Box px={2} py={0.5} borderRadius="full" bg="#f3f4f6">
                            <Text fontSize="xs" color="#666">{p.language}</Text>
                          </Box>
                          {p.type && (
                            <Box px={2} py={0.5} borderRadius="full" bg={typeBadge.bg}>
                              <Text fontSize="xs" color={typeBadge.color}>{p.type}</Text>
                            </Box>
                          )}
                        </Box>
                      );
                    })}
                  </VStack>
                </Box>
              </Box>

              {/* Related words */}
              {feedback.relatedDefinitions.length > 0 && (
                <Box>
                  <Text fontWeight="medium" fontSize="sm" mb={1}>Related words</Text>
                  <Box p={3} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                    <VStack align="stretch" gap={1}>
                      {feedback.relatedDefinitions.map((d, i) => (
                        <Text key={i} fontSize="sm">
                          <Text as="span" fontWeight="medium" color="#2563eb">{d.expression}</Text>
                          {" - "}{d.meaning}
                        </Text>
                      ))}
                    </VStack>
                  </Box>
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
                  setAnswer(submittedAnswer);
                  setTimeout(() => inputRef.current?.focus(), 50);
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
