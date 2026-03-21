"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Input, Text, VStack } from "@chakra-ui/react";
import { useQuizStore, type QuizType } from "@/store/quizStore";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { formatReviewDate } from "@/lib/formatReviewDate";

interface OriginPartDisplay {
  origin: string;
  meaning: string;
  language?: string;
  type?: string;
}

interface ResultItem {
  index: number;
  key: string;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  nextReviewDate?: string;
  noteId?: bigint;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalCorrect: boolean;
  originBreakdown?: OriginPartDisplay[];
  userAnswer?: string;
}

function getProtoQuizType(qt: QuizType): ProtoQuizType {
  if (qt === "reverse") return ProtoQuizType.REVERSE;
  if (qt === "freeform") return ProtoQuizType.FREEFORM;
  if (qt === "etymology-breakdown") return ProtoQuizType.ETYMOLOGY_BREAKDOWN;
  if (qt === "etymology-assembly") return ProtoQuizType.ETYMOLOGY_ASSEMBLY;
  if (qt === "etymology-freeform") return ProtoQuizType.ETYMOLOGY_BREAKDOWN;
  return ProtoQuizType.STANDARD;
}

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

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const etymologyResults = useQuizStore((s) => s.etymologyResults);
  const quizType = useQuizStore((s) => s.quizType);
  const reset = useQuizStore((s) => s.reset);
  const overrideResult = useQuizStore((s) => s.overrideResult);
  const undoOverrideResult = useQuizStore((s) => s.undoOverrideResult);
  const skipResult = useQuizStore((s) => s.skipResult);
  const resumeResult = useQuizStore((s) => s.resumeResult);
  const updateResultReviewDate = useQuizStore((s) => s.updateResultReviewDate);

  const isEtymologyQuiz = quizType === "etymology-breakdown" || quizType === "etymology-assembly" || quizType === "etymology-freeform";

  const allResults = useMemo((): ResultItem[] => {
    if (results.length > 0) {
      return results.map((r, i) => ({
        index: i,
        key: r.noteId.toString(),
        entry: r.entry,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        nextReviewDate: r.nextReviewDate,
        noteId: r.noteId,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
      }));
    }
    if (reverseResults.length > 0) {
      return reverseResults.map((r, i) => ({
        index: i,
        key: r.noteId.toString(),
        entry: r.expression,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        nextReviewDate: r.nextReviewDate,
        noteId: r.noteId,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
      }));
    }
    if (freeformResults.length > 0) {
      return freeformResults.map((r, i) => ({
        index: i,
        key: `freeform-${i}`,
        entry: r.word,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        nextReviewDate: r.nextReviewDate,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
      }));
    }
    if (etymologyResults.length > 0) {
      return etymologyResults.map((r, i) => ({
        index: i,
        key: r.noteId ? r.noteId.toString() : `ety-${i}`,
        entry: r.expression,
        meaning: r.meaning,
        correct: r.correct,
        nextReviewDate: r.nextReviewDate,
        noteId: r.noteId,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
        originBreakdown: r.originParts?.map((p) => ({
          origin: p.origin,
          meaning: p.meaning,
          language: p.language,
          type: p.type,
        })),
        userAnswer: r.answer,
      }));
    }
    return [];
  }, [results, reverseResults, freeformResults, etymologyResults]);

  useEffect(() => {
    if (allResults.length === 0) {
      router.push("/");
    }
  }, [allResults, router]);

  if (allResults.length === 0) {
    return null;
  }

  const correctResults = allResults.filter((r) => r.correct && !r.isSkipped);
  const incorrectResults = allResults.filter((r) => !r.correct && !r.isSkipped);
  const skippedResults = allResults.filter((r) => r.isSkipped);

  const protoQt = getProtoQuizType(quizType);

  const handleOverride = async (item: ResultItem) => {
    if (!item.noteId || !item.learnedAt) return;
    try {
      const res = await quizClient.overrideAnswer({
        noteId: item.noteId,
        quizType: protoQt,
        learnedAt: item.learnedAt,
        markCorrect: !item.correct,
      });
      overrideResult(item.index, quizType, res.nextReviewDate || item.nextReviewDate || "", {
        quality: res.originalQuality,
        status: res.originalStatus,
        intervalDays: res.originalIntervalDays,
        easinessFactor: res.originalEasinessFactor,
      });
    } catch { /* silently fail */ }
  };

  const handleUndo = async (item: ResultItem) => {
    if (!item.noteId || !item.learnedAt) return;
    const storeResults = results.length > 0 ? results : reverseResults.length > 0 ? reverseResults : freeformResults.length > 0 ? freeformResults : etymologyResults;
    const original = storeResults[item.index];
    if (!original?.originalValues) return;
    try {
      const res = await quizClient.undoOverrideAnswer({
        noteId: item.noteId,
        quizType: protoQt,
        learnedAt: item.learnedAt,
        originalQuality: original.originalValues.quality,
        originalStatus: original.originalValues.status,
        originalIntervalDays: original.originalValues.intervalDays,
        originalEasinessFactor: original.originalValues.easinessFactor,
      });
      undoOverrideResult(item.index, quizType, res.correct, res.nextReviewDate || item.nextReviewDate || "");
    } catch { /* silently fail */ }
  };

  const handleSkip = async (item: ResultItem) => {
    if (!item.noteId) return;
    try {
      await quizClient.skipWord({ noteId: item.noteId });
      skipResult(item.index, quizType);
    } catch { /* silently fail */ }
  };

  const handleResume = async (item: ResultItem) => {
    if (!item.noteId) return;
    try {
      await quizClient.resumeWord({ noteId: item.noteId });
      resumeResult(item.index, quizType);
    } catch { /* silently fail */ }
  };

  const handleChangeReviewDate = async (item: ResultItem, newDate: string) => {
    if (!item.noteId || !item.learnedAt) return;
    try {
      await quizClient.overrideAnswer({
        noteId: item.noteId,
        quizType: protoQt,
        learnedAt: item.learnedAt,
        nextReviewDate: newDate,
      });
      updateResultReviewDate(item.index, quizType, newDate);
    } catch { /* silently fail */ }
  };

  const handleBackToStart = () => {
    reset();
    router.push("/quiz");
  };

  return (
    <Box p={4} maxW="sm" mx="auto">
      <Heading size="lg" mb={4}>
        Session Complete
      </Heading>

      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Total: {allResults.length} words</Text>
        <Text color="green.600" _dark={{ color: "green.300" }} fontWeight="bold">
          Correct: {correctResults.length}
        </Text>
        <Text color="red.600" _dark={{ color: "red.300" }} fontWeight="bold">
          Incorrect: {incorrectResults.length}
        </Text>
      </VStack>

      {incorrectResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.600" _dark={{ color: "red.300" }} mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrectResults.map((r) => (
              <ResultCard
                key={r.key}
                item={r}
                isEtymology={isEtymologyQuiz}
                onOverride={handleOverride}
                onUndo={handleUndo}
                onSkip={handleSkip}
                onResume={handleResume}
                onChangeReviewDate={handleChangeReviewDate}
              />
            ))}
          </VStack>
        </Box>
      )}

      {correctResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.600" _dark={{ color: "green.300" }} mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correctResults.map((r) => (
              <ResultCard
                key={r.key}
                item={r}
                isEtymology={isEtymologyQuiz}
                onOverride={handleOverride}
                onUndo={handleUndo}
                onSkip={handleSkip}
                onResume={handleResume}
                onChangeReviewDate={handleChangeReviewDate}
              />
            ))}
          </VStack>
        </Box>
      )}

      {skippedResults.length > 0 && (
        <Box mb={6}>
          <Text fontWeight="bold" mb={2} color="gray.500">
            Excluded from Quizzes ({skippedResults.length})
          </Text>
          <VStack align="stretch" gap={2}>
            {skippedResults.map((r) => (
              <ResultCard
                key={r.key}
                item={r}
                isEtymology={isEtymologyQuiz}
                onOverride={handleOverride}
                onUndo={handleUndo}
                onSkip={handleSkip}
                onResume={handleResume}
                onChangeReviewDate={handleChangeReviewDate}
              />
            ))}
          </VStack>
        </Box>
      )}

      <Button w="full" colorPalette="blue" onClick={handleBackToStart}>
        Back to Start
      </Button>
    </Box>
  );
}

function ResultCard({
  item,
  isEtymology,
  onOverride,
  onUndo,
  onSkip,
  onResume,
  onChangeReviewDate,
}: {
  item: ResultItem;
  isEtymology: boolean;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
  onChangeReviewDate: (item: ResultItem, newDate: string) => void;
}) {
  const [showDatePicker, setShowDatePicker] = useState(false);
  const [customDate, setCustomDate] = useState("");

  const tomorrowStr = useMemo(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return d.toISOString().split("T")[0];
  }, []);

  const borderColor = item.isSkipped
    ? "gray.200"
    : item.correct
      ? "green.200"
      : "red.200";

  const topBarColor = item.isSkipped
    ? "#d1d5db"
    : item.correct
      ? "#16a34a"
      : "#dc2626";

  return (
    <Box
      borderWidth="1px"
      borderColor={borderColor}
      borderRadius="md"
      overflow="hidden"
      opacity={item.isSkipped ? 0.6 : 1}
    >
      {/* Color bar at top */}
      <Box h="4px" bg={topBarColor} />

      <Box p={2}>
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Text fontWeight="bold">{item.entry}</Text>
          {item.isSkipped && (
            <Box bg="gray.100" _dark={{ bg: "gray.700" }} px={2} py={0.5} borderRadius="sm">
              <Text fontSize="xs" color="fg.muted" fontStyle="italic">Excluded</Text>
            </Box>
          )}
        </Box>
        <Text fontSize="sm">{item.meaning}</Text>
        {item.contexts?.map((ctx, i) => (
          <Text key={i} fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
            {ctx}
          </Text>
        ))}

        {/* Etymology origin breakdown with badges */}
        {isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
          <Box mt={2}>
            {/* Show user answer for incorrect results */}
            {!item.correct && item.userAnswer && (
              <Text fontSize="xs" color="fg.muted" mb={1}>
                Your answer: {item.userAnswer}
              </Text>
            )}
            <Text fontSize="xs" color={item.correct ? "#16a34a" : "fg.muted"} mb={1}>
              {item.correct ? "Breakdown:" : "Correct:"}
            </Text>
            <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
              {item.originBreakdown.map((p, i) => {
                const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
                return (
                  <Box key={i} display="flex" alignItems="center" gap={1}>
                    {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                    <Text fontSize="xs" color="#2563eb" fontWeight="medium">{p.origin}</Text>
                    <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
                    {p.language && (
                      <Box px={1.5} py={0} borderRadius="full" bg="#f3f4f6">
                        <Text fontSize="2xs" color="#666">{p.language}</Text>
                      </Box>
                    )}
                    {typeBadge && p.type && (
                      <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg}>
                        <Text fontSize="2xs" color={typeBadge.color}>{p.type}</Text>
                      </Box>
                    )}
                  </Box>
                );
              })}
            </Box>
          </Box>
        )}

        {/* Non-etymology origin breakdown (backward compatible) */}
        {!isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
          <Box display="flex" gap={1} alignItems="center" flexWrap="wrap" mt={1}>
            {item.originBreakdown.map((p, i) => (
              <Box key={i} display="flex" alignItems="center" gap={1}>
                {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                <Text fontSize="xs" color="#2563eb" fontWeight="medium">{p.origin}</Text>
                <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
              </Box>
            ))}
          </Box>
        )}

        {/* Review date */}
        {item.nextReviewDate && (
          <Box
            mt={2}
            bg="blue.50"
            _dark={{ bg: "blue.900/20", borderColor: "blue.700" }}
            borderWidth="1px"
            borderColor="blue.200"
            borderRadius="md"
            p={2}
          >
            {showDatePicker ? (
              <VStack align="stretch" gap={2}>
                <Text fontSize="xs" fontWeight="medium">
                  Pick a new review date:
                </Text>
                <Input
                  type="date"
                  size="sm"
                  value={customDate}
                  min={tomorrowStr}
                  onChange={(e) => setCustomDate(e.target.value)}
                />
                <Box display="flex" gap={2} justifyContent="flex-end">
                  <Button
                    size="xs"
                    variant="ghost"
                    onClick={() => setShowDatePicker(false)}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="xs"
                    colorPalette="blue"
                    onClick={() => {
                      if (customDate) {
                        onChangeReviewDate(item, customDate);
                      }
                      setShowDatePicker(false);
                    }}
                  >
                    Save
                  </Button>
                </Box>
              </VStack>
            ) : (
              <>
                <Text fontSize="xs" fontWeight="medium">
                  Next review: {formatReviewDate(item.nextReviewDate)}
                </Text>
                {item.noteId && item.learnedAt && (
                  <Text
                    fontSize="xs"
                    color="blue.600"
                    _dark={{ color: "blue.300" }}
                    cursor="pointer"
                    onClick={() => {
                      setCustomDate(item.nextReviewDate!);
                      setShowDatePicker(true);
                    }}
                  >
                    Change
                  </Text>
                )}
              </>
            )}
          </Box>
        )}

        {/* Override button */}
        {!item.isOverridden && !item.isSkipped && item.noteId && item.learnedAt && (
          <Button
            w="full"
            mt={2}
            size="sm"
            variant="outline"
            colorPalette={item.correct ? "red" : "blue"}
            onClick={() => onOverride(item)}
          >
            {item.correct ? "Mark as Incorrect" : "Mark as Correct"}
          </Button>
        )}

        {item.isOverridden && (
          <Text mt={2} fontSize="xs" color="fg.muted" fontStyle="italic">
            {item.correct ? "Marked as correct" : "Marked as incorrect"} (overridden){" "}
            <Text
              as="span"
              color="blue.600"
              cursor="pointer"
              textDecoration="underline"
              onClick={() => onUndo(item)}
            >
              Undo
            </Text>
          </Text>
        )}

        {/* Skip button, Resume button, or nothing */}
        {item.isSkipped ? (
          item.noteId ? (
            <Button
              w="full"
              mt={2}
              size="sm"
              variant="outline"
              colorPalette="blue"
              onClick={() => onResume(item)}
            >
              Resume
            </Button>
          ) : null
        ) : !item.isOverridden && item.noteId ? (
          <Button
            w="full"
            mt={2}
            size="sm"
            variant="outline"
            colorPalette="gray"
            onClick={() => onSkip(item)}
          >
            Exclude from Quizzes
          </Button>
        ) : null}
      </Box>
    </Box>
  );
}
