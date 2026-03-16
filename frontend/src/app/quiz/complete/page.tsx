"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Flex, Heading, Input, Text, VStack } from "@chakra-ui/react";
import {
  useQuizStore,
  WordDetail,
  type QuizType,
  type OriginalValues,
} from "@/store/quizStore";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";

interface ResultItem {
  key: string;
  index: number;
  noteId?: bigint;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  wordDetail?: WordDetail;
  nextReviewDate?: string;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalValues?: OriginalValues;
  quizType: QuizType;
}

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const reset = useQuizStore((s) => s.reset);

  const allResults = useMemo((): ResultItem[] => {
    if (results.length > 0) {
      return results.map((r, i) => ({
        key: r.noteId.toString(),
        index: i,
        noteId: r.noteId,
        entry: r.entry,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        wordDetail: r.wordDetail,
        nextReviewDate: r.nextReviewDate,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalValues: r.originalValues,
        quizType: "standard" as QuizType,
      }));
    }
    if (reverseResults.length > 0) {
      return reverseResults.map((r, i) => ({
        key: r.noteId.toString(),
        index: i,
        noteId: r.noteId,
        entry: r.expression,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        wordDetail: r.wordDetail,
        nextReviewDate: r.nextReviewDate,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalValues: r.originalValues,
        quizType: "reverse" as QuizType,
      }));
    }
    if (freeformResults.length > 0) {
      return freeformResults.map((r, i) => ({
        key: `freeform-${i}`,
        index: i,
        entry: r.word,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        wordDetail: r.wordDetail,
        nextReviewDate: r.nextReviewDate,
        learnedAt: r.learnedAt,
        isOverridden: r.isOverridden,
        isSkipped: r.isSkipped,
        originalValues: r.originalValues,
        quizType: "freeform" as QuizType,
      }));
    }
    return [];
  }, [results, reverseResults, freeformResults]);

  useEffect(() => {
    if (allResults.length === 0) {
      router.push("/");
    }
  }, [allResults, router]);

  if (allResults.length === 0) {
    return null;
  }

  const correctResults = allResults.filter((r) => r.correct);
  const incorrectResults = allResults.filter((r) => !r.correct);

  const handleBackToStart = () => {
    reset();
    router.push("/");
  };

  return (
    <Box p={4} maxW="md" mx="auto">
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

      {correctResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.600" _dark={{ color: "green.300" }} mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correctResults.map((r) => (
              <ResultCard key={r.key} result={r} />
            ))}
          </VStack>
        </Box>
      )}

      {incorrectResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.600" _dark={{ color: "red.300" }} mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrectResults.map((r) => (
              <ResultCard key={r.key} result={r} />
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

function ResultCard({ result }: { result: ResultItem }) {
  const r = result;
  const store = useQuizStore.getState;
  const [overrideLoading, setOverrideLoading] = useState(false);
  const [skipLoading, setSkipLoading] = useState(false);
  const [localOverridden, setLocalOverridden] = useState(r.isOverridden ?? false);
  const [localSkipped, setLocalSkipped] = useState(r.isSkipped ?? false);
  const [localCorrect, setLocalCorrect] = useState(r.correct);
  const [localNextReviewDate, setLocalNextReviewDate] = useState(r.nextReviewDate ?? "");
  const [showDatePicker, setShowDatePicker] = useState(false);
  const [dateLoading, setDateLoading] = useState(false);

  const protoQuizType = r.quizType === "standard"
    ? ProtoQuizType.STANDARD
    : r.quizType === "reverse"
      ? ProtoQuizType.REVERSE
      : ProtoQuizType.FREEFORM;

  const hasNoteId = r.noteId !== undefined;

  const handleOverride = async () => {
    if (!hasNoteId || !r.learnedAt) return;
    setOverrideLoading(true);
    try {
      const res = await quizClient.overrideAnswer({
        noteId: r.noteId!,
        quizType: protoQuizType,
        learnedAt: r.learnedAt,
        markCorrect: !localCorrect,
      });
      store().overrideResult(r.index, r.quizType, res.nextReviewDate, {
        quality: res.originalQuality,
        status: res.originalStatus,
        intervalDays: res.originalIntervalDays,
        easinessFactor: res.originalEasinessFactor,
      });
      setLocalCorrect(!localCorrect);
      setLocalNextReviewDate(res.nextReviewDate);
      setLocalOverridden(true);
    } catch {
      // silently fail
    } finally {
      setOverrideLoading(false);
    }
  };

  const handleUndoOverride = async () => {
    if (!hasNoteId || !r.learnedAt) return;
    // Read the latest original values from the store
    let originalValues: OriginalValues | undefined;
    if (r.quizType === "standard") {
      originalValues = store().results[r.index]?.originalValues;
    } else if (r.quizType === "reverse") {
      originalValues = store().reverseResults[r.index]?.originalValues;
    } else {
      originalValues = store().freeformResults[r.index]?.originalValues;
    }
    if (!originalValues) return;
    setOverrideLoading(true);
    try {
      const res = await quizClient.undoOverrideAnswer({
        noteId: r.noteId!,
        quizType: protoQuizType,
        learnedAt: r.learnedAt,
        originalQuality: originalValues.quality,
        originalStatus: originalValues.status,
        originalIntervalDays: originalValues.intervalDays,
        originalEasinessFactor: originalValues.easinessFactor,
      });
      store().undoOverrideResult(r.index, r.quizType, res.correct, res.nextReviewDate);
      setLocalCorrect(res.correct);
      setLocalNextReviewDate(res.nextReviewDate);
      setLocalOverridden(false);
    } catch {
      // silently fail
    } finally {
      setOverrideLoading(false);
    }
  };

  const handleSkipWord = async () => {
    if (!hasNoteId) return;
    setSkipLoading(true);
    try {
      await quizClient.skipWord({ noteId: r.noteId! });
      store().skipResult(r.index, r.quizType);
      setLocalSkipped(true);
    } catch {
      // silently fail
    } finally {
      setSkipLoading(false);
    }
  };

  const handleDateChange = async (newDate: string) => {
    if (!hasNoteId || !r.learnedAt || !newDate) return;
    setDateLoading(true);
    try {
      const res = await quizClient.overrideAnswer({
        noteId: r.noteId!,
        quizType: protoQuizType,
        learnedAt: r.learnedAt,
        nextReviewDate: newDate,
      });
      setLocalNextReviewDate(res.nextReviewDate);
      store().updateResultReviewDate(r.index, r.quizType, res.nextReviewDate);
      setShowDatePicker(false);
    } catch {
      // silently fail
    } finally {
      setDateLoading(false);
    }
  };

  return (
    <Box p={2} borderWidth="1px" borderRadius="md">
      <Flex alignItems="center" gap={2}>
        <Text fontWeight="bold" flex="1">{r.entry}</Text>
        {localOverridden && (
          <Text fontSize="xs" fontStyle="italic" color="fg.muted">(overridden)</Text>
        )}
        {localSkipped && (
          <Text fontSize="xs" fontStyle="italic" color="fg.muted">(skipped)</Text>
        )}
      </Flex>
      <Text fontSize="sm">{r.meaning}</Text>
      {r.contexts?.map((ctx, i) => (
        <Text key={i} fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
          {ctx}
        </Text>
      ))}
      {r.wordDetail && (
        <Box mt={1} fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }}>
          {r.wordDetail.partOfSpeech && (
            <Text><Text as="span" fontWeight="bold">Part of speech:</Text> {r.wordDetail.partOfSpeech}</Text>
          )}
          {r.wordDetail.pronunciation && (
            <Text><Text as="span" fontWeight="bold">Pronunciation:</Text> {r.wordDetail.pronunciation}</Text>
          )}
          {r.wordDetail.origin && (
            <Text><Text as="span" fontWeight="bold">Origin:</Text> {r.wordDetail.origin}</Text>
          )}
          {r.wordDetail.synonyms && r.wordDetail.synonyms.length > 0 && (
            <Text><Text as="span" fontWeight="bold">Synonyms:</Text> {r.wordDetail.synonyms.join(", ")}</Text>
          )}
          {r.wordDetail.antonyms && r.wordDetail.antonyms.length > 0 && (
            <Text><Text as="span" fontWeight="bold">Antonyms:</Text> {r.wordDetail.antonyms.join(", ")}</Text>
          )}
          {r.wordDetail.memo && (
            <Text><Text as="span" fontWeight="bold">Memo:</Text> {r.wordDetail.memo}</Text>
          )}
        </Box>
      )}

      {localNextReviewDate && (
        <Box mt={2}>
          <Flex alignItems="center" gap={2}>
            <Text fontSize="xs" color="fg.muted">
              Next review: {localNextReviewDate}
            </Text>
            {hasNoteId && r.learnedAt && !showDatePicker && (
              <Text
                fontSize="xs"
                color="blue.500"
                cursor="pointer"
                onClick={() => setShowDatePicker(true)}
              >
                Change
              </Text>
            )}
          </Flex>
          {showDatePicker && (
            <Flex mt={1} gap={2} alignItems="center">
              <Input
                type="date"
                size="sm"
                defaultValue={localNextReviewDate}
                disabled={dateLoading}
                onChange={(e) => {
                  if (e.target.value) handleDateChange(e.target.value);
                }}
              />
              <Text
                fontSize="xs"
                color="gray.500"
                cursor="pointer"
                onClick={() => setShowDatePicker(false)}
              >
                Cancel
              </Text>
            </Flex>
          )}
        </Box>
      )}

      {hasNoteId && r.learnedAt && (
        <Flex mt={2} gap={2} alignItems="center" flexWrap="wrap">
          {!localOverridden && !localSkipped && (
            <Button
              variant="outline"
              colorPalette={localCorrect ? "red" : "blue"}
              onClick={handleOverride}
              disabled={overrideLoading}
              size="xs"
            >
              {localCorrect ? "Mark as Incorrect" : "Mark as Correct"}
            </Button>
          )}

          {localOverridden && (
            <Text
              fontSize="xs"
              color="blue.500"
              cursor="pointer"
              onClick={handleUndoOverride}
            >
              Undo
            </Text>
          )}

          {!localSkipped && (
            <Button
              variant="outline"
              colorPalette="gray"
              onClick={handleSkipWord}
              disabled={skipLoading}
              size="xs"
            >
              Skip Word
            </Button>
          )}
        </Flex>
      )}
    </Box>
  );
}
