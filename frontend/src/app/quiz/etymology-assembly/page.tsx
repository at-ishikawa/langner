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

function getTypeBadgeColors(type: string): { bg: string; darkBg: string; color: string; darkColor: string } {
  switch (type.toLowerCase()) {
    case "root":
      return { bg: "blue.100", darkBg: "blue.900", color: "blue.600", darkColor: "blue.300" };
    case "prefix":
      return { bg: "yellow.100", darkBg: "yellow.900", color: "yellow.800", darkColor: "yellow.200" };
    case "suffix":
      return { bg: "green.100", darkBg: "green.900", color: "green.800", darkColor: "green.200" };
    default:
      return { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300" };
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
    images?: string[];
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
        images: res.images.length > 0 ? res.images : undefined,
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
        images: res.images.length > 0 ? res.images : undefined,
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
            <Text fontWeight="medium" mb={1} fontSize="sm" textAlign="center">
              What word is made from these origins?
            </Text>
            {card.meaning && (
              <Text fontSize="sm" color="fg.muted" textAlign="center" mb={3} fontStyle="italic">
                Hint: {card.meaning}
              </Text>
            )}
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
                    <Text fontWeight="semibold" color="blue.600" _dark={{ color: "blue.300" }} fontSize="lg">{p.origin}</Text>
                    <Text fontSize="sm" color="fg.muted">= {p.meaning}</Text>
                    <Box ml="auto" px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                      <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
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
                    borderColor={displayCorrect ? "green.600" : "red.600"}
                    bg="white"
                    _dark={{ bg: "gray.800" }}
                    display="flex"
                    justifyContent="space-between"
                    alignItems="center"
                  >
                    <Text
                      textDecoration={displayCorrect ? "none" : "line-through"}
                      color={displayCorrect ? undefined : "red.600"}
                    >
                      {submittedAnswer}
                    </Text>
                    <Text
                      fontWeight="medium"
                      color={displayCorrect ? "green.600" : "red.600"}
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
                          <Text color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium" fontSize="sm">{p.origin}</Text>
                          <Text fontSize="sm" color="fg.muted">= {p.meaning}</Text>
                          <Box px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                            <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                          </Box>
                          {p.type && (
                            <Box px={2} py={0.5} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                              <Text fontSize="xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
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
                          <Text as="span" fontWeight="medium" color="blue.600" _dark={{ color: "blue.300" }}>{d.expression}</Text>
                          {" - "}{d.meaning}
                        </Text>
                      ))}
                    </VStack>
                  </Box>
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
