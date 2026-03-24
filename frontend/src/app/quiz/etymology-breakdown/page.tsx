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

interface OriginRow {
  origin: string;
  meaning: string;
}

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

export default function EtymologyBreakdownPage() {
  const router = useRouter();
  const etymologyCards = useQuizStore((s) => s.etymologyCards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyResult);
  const storeSkipResult = useQuizStore((s) => s.skipResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [rows, setRows] = useState<OriginRow[]>([{ origin: "", meaning: "" }]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{
    correct: boolean;
    reason: string;
    originGrades: Array<{
      userOrigin: string;
      userMeaning: string;
      originCorrect: boolean;
      meaningCorrect: boolean;
      correctOrigin?: { origin: string; meaning: string };
    }>;
    relatedDefinitions: Array<{ expression: string; meaning: string; notebookName: string }>;
    learnedAt?: string;
    noteId?: bigint;
    images?: string[];
  } | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
  const startTimeRef = useRef(Date.now());
  const firstInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (etymologyCards.length === 0 || quizType !== "etymology-breakdown") {
      router.push("/");
    }
  }, [etymologyCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setRows([{ origin: "", meaning: "" }]);
    setFeedback(null);
    setOverridden(false);
    setSkipped(false);
    setDisplayCorrect(false);
    setTimeout(() => firstInputRef.current?.focus(), 50);
  }, [currentIndex]);

  if (etymologyCards.length === 0) return null;

  const card = etymologyCards[currentIndex];
  const total = etymologyCards.length;
  const progress = ((currentIndex + 1) / total) * 100;

  const addRow = () => {
    setRows([...rows, { origin: "", meaning: "" }]);
  };

  const updateRow = (index: number, field: keyof OriginRow, value: string) => {
    setRows(rows.map((r, i) => (i === index ? { ...r, [field]: value } : r)));
  };

  const removeRow = (index: number) => {
    if (rows.length <= 1) return;
    setRows(rows.filter((_, i) => i !== index));
  };

  const canSubmit = rows.some((r) => r.origin.trim() || r.meaning.trim());

  const handleSubmit = async () => {
    if (!canSubmit) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    setPhase("feedback");
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitEtymologyBreakdownAnswer({
        cardId: card.cardId,
        answers: rows
          .filter((r) => r.origin.trim() || r.meaning.trim())
          .map((r) => ({ origin: r.origin.trim(), meaning: r.meaning.trim() })),
        responseTimeMs: BigInt(responseTimeMs),
      });

      const fb = {
        correct: res.correct,
        reason: res.reason,
        originGrades: (res.originGrades ?? []).map((g) => ({
          userOrigin: g.userOrigin,
          userMeaning: g.userMeaning,
          originCorrect: g.originCorrect,
          meaningCorrect: g.meaningCorrect,
          correctOrigin: g.correctOrigin
            ? { origin: g.correctOrigin.origin, meaning: g.correctOrigin.meaning }
            : undefined,
        })),
        relatedDefinitions: (res.relatedDefinitions ?? []).map((d) => ({
          expression: d.expression,
          meaning: d.meaning,
          notebookName: d.notebookName,
        })),
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
        answer: rows.map((r) => `${r.origin}=${r.meaning}`).join(", "),
        correct: res.correct,
        reason: res.reason,
        originGrades: fb.originGrades,
        relatedDefinitions: fb.relatedDefinitions,
        originParts: card.originParts,
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
    if (e.key === "Enter" && phase === "feedback" && !loading) {
      handleNext();
    }
  };

  const relatedExpressions = feedback?.relatedDefinitions
    ? getRelatedExpressions(feedback.relatedDefinitions)
    : [];

  return (
    <Box p={4} maxW="sm" mx="auto" onKeyDown={handleKeyDown}>
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
          <Box p={4} borderWidth="1px" borderRadius="lg" textAlign="center" bg="white" _dark={{ bg: "gray.800" }}>
            <Heading size="xl">{card.expression}</Heading>
            <Text color="fg.muted" mt={1}>{card.meaning}</Text>
          </Box>

          <Box>
            <Text fontWeight="medium" mb={1}>What are the origins of this word?</Text>
            <Text fontSize="sm" color="fg.muted">Type each origin and its meaning. Add more rows as needed.</Text>
          </Box>

          {rows.map((row, i) => (
            <Box key={i}>
              <Text fontSize="xs" color="fg.muted" mb={1}>Origin {i + 1}</Text>
              <Box display="flex" gap={2} alignItems="center">
                <Input
                  ref={i === 0 ? firstInputRef : undefined}
                  placeholder="origin..."
                  value={row.origin}
                  onChange={(e) => updateRow(i, "origin", e.target.value)}
                  flex={1}
                />
                <Text color="fg.muted">=</Text>
                <Input
                  placeholder="meaning..."
                  value={row.meaning}
                  onChange={(e) => updateRow(i, "meaning", e.target.value)}
                  flex={1}
                />
                {rows.length > 1 && (
                  <Text
                    color="red.500"
                    cursor="pointer"
                    fontSize="sm"
                    onClick={() => removeRow(i)}
                  >
                    x
                  </Text>
                )}
              </Box>
            </Box>
          ))}

          <Text
            color="blue.600"
            _dark={{ color: "blue.300" }}
            fontSize="sm"
            cursor="pointer"
            fontWeight="medium"
            onClick={addRow}
          >
            + Add origin
          </Text>

          <Button
            colorPalette="blue"
            onClick={handleSubmit}
            disabled={!canSubmit}
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

              {/* Your answer section */}
              {feedback.originGrades.length > 0 && (
                <Box>
                  <Text fontWeight="medium" fontSize="sm" mb={1}>Your answer</Text>
                  <Box
                    p={3}
                    borderWidth="1.5px"
                    borderRadius="lg"
                    borderColor={displayCorrect ? "green.600" : "red.600"}
                    bg="white"
                    _dark={{ bg: "gray.800" }}
                  >
                    <VStack align="stretch" gap={2}>
                      {feedback.originGrades.map((g, i) => {
                        const bothCorrect = g.originCorrect && g.meaningCorrect;
                        return (
                          <Box key={i} display="flex" justifyContent="space-between" alignItems="center">
                            <Text
                              fontSize="sm"
                              textDecoration={!bothCorrect ? "line-through" : "none"}
                              color={!bothCorrect ? "red.600" : undefined}
                            >
                              {g.userOrigin} = {g.userMeaning}
                            </Text>
                            <Text
                              fontSize="sm"
                              fontWeight="medium"
                              color={bothCorrect ? "green.600" : "red.600"}
                            >
                              {bothCorrect ? "\u2713" : "\u2717"}
                            </Text>
                          </Box>
                        );
                      })}
                    </VStack>
                  </Box>
                </Box>
              )}

              {/* Full breakdown / Correct breakdown */}
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
              {relatedExpressions.length > 0 && (
                <Box>
                  <Text fontWeight="medium" fontSize="sm" mb={1}>Related words</Text>
                  <Box p={3} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                    <Text fontSize="sm" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">
                      {relatedExpressions.join(", ")}
                    </Text>
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
                isOverridden={overridden}
                isSkipped={skipped}
                nextLabel={currentIndex + 1 >= total ? "See Results" : "Next"}
                onNext={handleNext}
                onOverride={feedback.noteId ? async () => {
                  try {
                    const res = await quizClient.overrideAnswer({
                      noteId: feedback.noteId!,
                      quizType: ProtoQuizType.ETYMOLOGY_BREAKDOWN,
                      learnedAt: feedback.learnedAt!,
                      markCorrect: !displayCorrect,
                    });
                    setOverridden(true);
                    setDisplayCorrect(!displayCorrect);
                  } catch { /* silently fail */ }
                } : undefined}
                onSkip={feedback.noteId ? async () => {
                  try {
                    await quizClient.skipWord({ noteId: feedback.noteId! });
                    setSkipped(true);
                    storeSkipResult(currentIndex, "etymology-breakdown");
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

/** Get unique related word expressions for display */
function getRelatedExpressions(
  relatedDefs: Array<{ expression: string; meaning: string; notebookName: string }>,
): string[] {
  return relatedDefs.map((d) => d.expression);
}
