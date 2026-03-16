"use client";

import { useState } from "react";
import {
  Box,
  Button,
  Flex,
  Input,
  Text,
} from "@chakra-ui/react";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore, type OriginalValues } from "@/store/quizStore";

interface FeedbackActionsProps {
  noteId: bigint | undefined;
  quizType: string;
  learnedAt: string | undefined;
  correct: boolean;
  nextReviewDate: string | undefined;
  onOverride?: (newCorrect: boolean, newNextReviewDate: string, originalValues: OriginalValues) => void;
  onUndo?: (correct: boolean, nextReviewDate: string) => void;
  onSkip?: () => void;
}

function toProtoQuizType(quizType: string): ProtoQuizType {
  switch (quizType) {
    case "reverse":
      return ProtoQuizType.REVERSE;
    case "freeform":
      return ProtoQuizType.FREEFORM;
    default:
      return ProtoQuizType.STANDARD;
  }
}

export function FeedbackActions({
  noteId,
  quizType,
  learnedAt,
  correct,
  nextReviewDate,
  onOverride,
  onUndo,
  onSkip,
}: FeedbackActionsProps) {
  const [isOverridden, setIsOverridden] = useState(false);
  const [isSkipped, setIsSkipped] = useState(false);
  const [overrideLoading, setOverrideLoading] = useState(false);
  const [skipLoading, setSkipLoading] = useState(false);
  const [localNextReviewDate, setLocalNextReviewDate] = useState(nextReviewDate ?? "");
  const [showDatePicker, setShowDatePicker] = useState(false);
  const [dateLoading, setDateLoading] = useState(false);

  const protoQuizType = toProtoQuizType(quizType);

  const handleOverride = async () => {
    if (!noteId || !learnedAt || !onOverride) return;
    setOverrideLoading(true);
    try {
      const res = await quizClient.overrideAnswer({
        noteId,
        quizType: protoQuizType,
        learnedAt,
        markCorrect: !correct,
      });
      const originalValues: OriginalValues = {
        quality: res.originalQuality,
        status: res.originalStatus,
        intervalDays: res.originalIntervalDays,
        easinessFactor: res.originalEasinessFactor,
      };
      onOverride(!correct, res.nextReviewDate, originalValues);
      setLocalNextReviewDate(res.nextReviewDate);
      setIsOverridden(true);
    } catch {
      // silently fail
    } finally {
      setOverrideLoading(false);
    }
  };

  const handleUndoOverride = async () => {
    if (!noteId || !learnedAt || !onUndo) return;
    const store = useQuizStore.getState();
    const currentIndex = store.currentIndex;
    let originalValues: OriginalValues | undefined;
    if (quizType === "standard") {
      originalValues = store.results[currentIndex]?.originalValues;
    } else if (quizType === "reverse") {
      originalValues = store.reverseResults[currentIndex]?.originalValues;
    } else {
      originalValues = store.freeformResults[currentIndex]?.originalValues;
    }
    if (!originalValues) return;
    setOverrideLoading(true);
    try {
      const res = await quizClient.undoOverrideAnswer({
        noteId,
        quizType: protoQuizType,
        learnedAt,
        originalQuality: originalValues.quality,
        originalStatus: originalValues.status,
        originalIntervalDays: originalValues.intervalDays,
        originalEasinessFactor: originalValues.easinessFactor,
      });
      onUndo(res.correct, res.nextReviewDate);
      setLocalNextReviewDate(res.nextReviewDate);
      setIsOverridden(false);
    } catch {
      // silently fail
    } finally {
      setOverrideLoading(false);
    }
  };

  const handleSkipWord = async () => {
    if (!noteId || !onSkip) return;
    setSkipLoading(true);
    try {
      await quizClient.skipWord({ noteId });
      onSkip();
      setIsSkipped(true);
    } catch {
      // silently fail
    } finally {
      setSkipLoading(false);
    }
  };

  const handleDateChange = async (newDate: string) => {
    if (!noteId || !learnedAt || !newDate) return;
    setDateLoading(true);
    try {
      const res = await quizClient.overrideAnswer({
        noteId,
        quizType: protoQuizType,
        learnedAt,
        nextReviewDate: newDate,
      });
      setLocalNextReviewDate(res.nextReviewDate);
      const store = useQuizStore.getState();
      store.updateResultReviewDate(store.currentIndex, quizType as "standard" | "reverse" | "freeform", res.nextReviewDate);
      setShowDatePicker(false);
    } catch {
      // silently fail
    } finally {
      setDateLoading(false);
    }
  };

  return (
    <>
      {localNextReviewDate && (
        <Box>
          <Flex alignItems="center" gap={2}>
            <Text fontSize="sm" color="fg.muted">
              Next review: {localNextReviewDate}
            </Text>
            {!showDatePicker && noteId && (
              <Text
                fontSize="sm"
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
                fontSize="sm"
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

      {onOverride && !isOverridden && !isSkipped && (
        <Button
          variant="outline"
          colorPalette={correct ? "red" : "blue"}
          onClick={handleOverride}
          disabled={overrideLoading}
          size="sm"
        >
          {correct ? "Mark as Incorrect" : "Mark as Correct"}
        </Button>
      )}

      {onUndo && isOverridden && (
        <Flex alignItems="center" gap={2}>
          <Text
            fontSize="sm"
            color="blue.500"
            cursor="pointer"
            onClick={handleUndoOverride}
          >
            Undo
          </Text>
        </Flex>
      )}

      {onSkip && !isSkipped && (
        <Button
          variant="outline"
          colorPalette="gray"
          onClick={handleSkipWord}
          disabled={skipLoading}
          size="sm"
        >
          Skip Word
        </Button>
      )}
    </>
  );
}
