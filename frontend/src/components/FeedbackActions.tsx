"use client";

import { useMemo, useState } from "react";
import { Box, Button, Input, Text, VStack } from "@chakra-ui/react";
import { formatReviewDate } from "@/lib/formatReviewDate";

interface FeedbackActionsProps {
  /** Whether the current answer was correct (possibly after override). */
  isCorrect: boolean;
  /** noteId for the word, needed for override/skip RPCs. undefined disables those actions. */
  noteId?: bigint;
  /** Next review date in YYYY-MM-DD format. */
  nextReviewDate?: string;
  /** Whether the result has been overridden. */
  isOverridden: boolean;
  /** Whether the word has been skipped. */
  isSkipped: boolean;
  /** Label for the primary navigation button. */
  nextLabel: string;
  /** Called when the user clicks Next / See Results. */
  onNext: () => void;
  /** Called when the user overrides the result. */
  onOverride?: () => void;
  /** Called when the user skips the word. */
  onSkip?: () => void;
  /** Called when the user changes the review date. */
  onChangeReviewDate?: (newDate: string) => void;
}

export function FeedbackActions({
  isCorrect,
  noteId,
  nextReviewDate,
  isOverridden,
  isSkipped,
  nextLabel,
  onNext,
  onOverride,
  onSkip,
  onChangeReviewDate,
}: FeedbackActionsProps) {
  const [showDatePicker, setShowDatePicker] = useState(false);
  const [customDate, setCustomDate] = useState("");
  const [originalDate, setOriginalDate] = useState<string | null>(null);

  const canOverrideOrSkip = noteId !== undefined && !isOverridden && !isSkipped;

  const tomorrowStr = useMemo(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return d.toISOString().split("T")[0];
  }, []);

  return (
    <VStack align="stretch" gap={3}>
      {/* Next review date box */}
      {nextReviewDate && (
        <Box
          bg="blue.50"
          _dark={{ bg: "blue.900/20", borderColor: "blue.700" }}
          borderWidth="1px"
          borderColor="blue.200"
          borderRadius="md"
          p={3}
        >
          {showDatePicker ? (
            <VStack align="stretch" gap={2}>
              <Text fontSize="sm" fontWeight="medium">
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
                  size="sm"
                  variant="ghost"
                  onClick={() => setShowDatePicker(false)}
                >
                  Cancel
                </Button>
                <Button
                  size="sm"
                  colorPalette="blue"
                  onClick={() => {
                    if (customDate && onChangeReviewDate) {
                      if (!originalDate) {
                        setOriginalDate(nextReviewDate);
                      }
                      onChangeReviewDate(customDate);
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
              <Text fontSize="sm" fontWeight="medium">
                Next review: {formatReviewDate(nextReviewDate)}
              </Text>
              {originalDate && originalDate !== nextReviewDate && (
                <Text fontSize="xs" color="fg.muted">
                  (changed from {formatReviewDate(originalDate)})
                </Text>
              )}
              {onChangeReviewDate && (
                <Text
                  fontSize="xs"
                  color="blue.600"
                  _dark={{ color: "blue.300" }}
                  cursor="pointer"
                  onClick={() => {
                    setCustomDate(nextReviewDate);
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

      {/* Primary navigation button */}
      <Button w="full" colorPalette="blue" onClick={onNext}>
        {nextLabel}
      </Button>

      {/* Override button */}
      {canOverrideOrSkip && onOverride ? (
        <Button
          w="full"
          variant="outline"
          colorPalette={isCorrect ? "red" : "blue"}
          onClick={onOverride}
        >
          {isCorrect ? "Mark as Incorrect" : "Mark as Correct"}
        </Button>
      ) : null}

      {/* Exclude button or Excluded label */}
      {isSkipped ? (
        <Text fontSize="sm" color="fg.muted" fontStyle="italic">
          Excluded from quizzes
        </Text>
      ) : canOverrideOrSkip && onSkip ? (
        <Button
          w="full"
          variant="outline"
          colorPalette="gray"
          onClick={onSkip}
        >
          Exclude from Quizzes
        </Button>
      ) : null}
    </VStack>
  );
}
