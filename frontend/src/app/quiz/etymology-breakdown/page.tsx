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
    nextReviewDate?: string;
    learnedAt?: string;
    noteId?: bigint;
  } | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
  const startTimeRef = useRef(Date.now());

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
        answer: rows.map((r) => `${r.origin}=${r.meaning}`).join(", "),
        correct: res.correct,
        reason: res.reason,
        originGrades: fb.originGrades,
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
          <Box p={4} borderWidth="1px" borderRadius="md" textAlign="center">
            <Heading size="xl">{card.expression}</Heading>
            <Text color="fg.muted" mt={1}>{card.meaning}</Text>
          </Box>

          <Text fontWeight="medium">Break down the origins:</Text>

          {rows.map((row, i) => (
            <Box key={i} display="flex" gap={2} alignItems="center">
              <Input
                placeholder="Origin"
                value={row.origin}
                onChange={(e) => updateRow(i, "origin", e.target.value)}
                flex={1}
              />
              <Text color="fg.muted">=</Text>
              <Input
                placeholder="Meaning"
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
          ))}

          <Text
            color="#2563eb"
            fontSize="sm"
            cursor="pointer"
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
          <Box p={4} borderWidth="1px" borderRadius="md" textAlign="center">
            <Heading size="xl">{card.expression}</Heading>
            <Text color="fg.muted" mt={1}>{card.meaning}</Text>
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

              {feedback.reason && (
                <Box>
                  <Text fontWeight="bold">Reason</Text>
                  <Text>{feedback.reason}</Text>
                </Box>
              )}

              {feedback.originGrades.length > 0 && (
                <Box>
                  <Text fontWeight="bold" mb={1}>Your answers:</Text>
                  <VStack align="stretch" gap={1}>
                    {feedback.originGrades.map((g, i) => (
                      <Box
                        key={i}
                        p={2}
                        borderWidth="1px"
                        borderRadius="sm"
                        borderColor={g.originCorrect && g.meaningCorrect ? "green.200" : "red.200"}
                      >
                        <Box display="flex" gap={2} alignItems="center">
                          <Text
                            textDecoration={g.originCorrect ? "none" : "line-through"}
                            color={g.originCorrect ? "green.600" : "red.600"}
                          >
                            {g.userOrigin}
                          </Text>
                          <Text color="fg.muted">=</Text>
                          <Text
                            textDecoration={g.meaningCorrect ? "none" : "line-through"}
                            color={g.meaningCorrect ? "green.600" : "red.600"}
                          >
                            {g.userMeaning}
                          </Text>
                        </Box>
                        {g.correctOrigin && !(g.originCorrect && g.meaningCorrect) && (
                          <Text fontSize="sm" color="fg.muted" mt={1}>
                            Correct: {g.correctOrigin.origin} = {g.correctOrigin.meaning}
                          </Text>
                        )}
                      </Box>
                    ))}
                  </VStack>
                </Box>
              )}

              <Box>
                <Text fontWeight="bold" mb={1}>Full breakdown:</Text>
                <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
                  {card.originParts.map((p, i) => (
                    <Box key={i} display="flex" alignItems="center" gap={1}>
                      {i > 0 && <Text color="fg.muted">+</Text>}
                      <Text color="#2563eb" fontWeight="medium">{p.origin}</Text>
                      <Text fontSize="sm" color="fg.muted">({p.meaning})</Text>
                    </Box>
                  ))}
                </Box>
              </Box>

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
                      quizType: ProtoQuizType.ETYMOLOGY_BREAKDOWN,
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
                    storeSkipResult(currentIndex, "etymology-breakdown");
                  } catch { /* silently fail */ }
                } : undefined}
                onChangeReviewDate={feedback.noteId && feedback.learnedAt ? async (newDate) => {
                  try {
                    await quizClient.overrideAnswer({
                      noteId: feedback.noteId!,
                      quizType: ProtoQuizType.ETYMOLOGY_BREAKDOWN,
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
