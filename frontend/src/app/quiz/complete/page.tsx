"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { useQuizStore, type QuizType } from "@/store/quizStore";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";

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
  noteId?: bigint;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalCorrect: boolean;
  originBreakdown?: OriginPartDisplay[];
  userAnswer?: string;
  images?: string[];
}

function getProtoQuizType(qt: QuizType): ProtoQuizType {
  if (qt === "reverse") return ProtoQuizType.REVERSE;
  if (qt === "freeform") return ProtoQuizType.FREEFORM;
  if (qt === "etymology-standard") return ProtoQuizType.ETYMOLOGY_STANDARD;
  if (qt === "etymology-reverse") return ProtoQuizType.ETYMOLOGY_REVERSE;
  if (qt === "etymology-freeform") return ProtoQuizType.ETYMOLOGY_FREEFORM;
  return ProtoQuizType.STANDARD;
}

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

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const etymologyResults = useQuizStore((s) => s.etymologyOriginResults);
  const quizType = useQuizStore((s) => s.quizType);
  const reset = useQuizStore((s) => s.reset);
  const overrideResult = useQuizStore((s) => s.overrideResult);
  const undoOverrideResult = useQuizStore((s) => s.undoOverrideResult);
  const skipResult = useQuizStore((s) => s.skipResult);
  const resumeResult = useQuizStore((s) => s.resumeResult);
  const isEtymologyQuiz = quizType === "etymology-standard" || quizType === "etymology-reverse" || quizType === "etymology-freeform";

  const allResults = useMemo((): ResultItem[] => {
    if (results.length > 0) {
      return results.map((r, i) => ({
        index: i,
        key: r.noteId.toString(),
        entry: r.entry,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        noteId: r.noteId,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
        images: r.images,
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
        noteId: r.noteId,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
        images: r.images,
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
        images: r.images,
      }));
    }
    if (etymologyResults.length > 0) {
      return etymologyResults.map((r, i) => ({
        index: i,
        key: r.noteId ? r.noteId.toString() : `ety-${i}`,
        entry: r.origin,
        meaning: r.correctAnswer,
        correct: r.correct,
        noteId: r.noteId,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalCorrect: r.isOverridden ? !r.correct : r.correct,
        originBreakdown: [{
          origin: r.origin,
          meaning: r.correctAnswer,
          language: r.language,
          type: r.type,
        }],
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
      overrideResult(item.index, quizType, res.nextReviewDate || "", {
        quality: res.originalQuality,
        status: res.originalStatus,
        intervalDays: res.originalIntervalDays,
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
      });
      undoOverrideResult(item.index, quizType, res.correct, res.nextReviewDate || "");
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
}: {
  item: ResultItem;
  isEtymology: boolean;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}) {

  const borderColor = item.isSkipped
    ? "gray.200"
    : item.correct
      ? "green.200"
      : "red.200";

  const topBarColor = item.isSkipped
    ? "gray.300"
    : item.correct
      ? "green.600"
      : "red.600";

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

        {/* Images */}
        {item.images && item.images.length > 0 && (
          <Box display="flex" gap={2} mt={2} flexWrap="wrap">
            {item.images.map((src, i) => (
              <img key={i} src={src} alt="" style={{ maxHeight: "150px", borderRadius: "4px" }} />
            ))}
          </Box>
        )}

        {/* Etymology origin breakdown with badges */}
        {isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
          <Box mt={2}>
            {/* Show user answer for incorrect results */}
            {!item.correct && item.userAnswer && (
              <Text fontSize="xs" color="fg.muted" mb={1}>
                Your answer: {item.userAnswer}
              </Text>
            )}
            <Text fontSize="xs" color={item.correct ? "green.600" : "fg.muted"} mb={1}>
              {item.correct ? "Breakdown:" : "Correct:"}
            </Text>
            <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
              {item.originBreakdown.map((p, i) => {
                const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
                return (
                  <Box key={i} display="flex" alignItems="center" gap={1}>
                    {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                    <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
                    <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
                    {p.language && (
                      <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                        <Text fontSize="2xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                      </Box>
                    )}
                    {typeBadge && p.type && (
                      <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                        <Text fontSize="2xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
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
                <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
                <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
              </Box>
            ))}
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
