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
  /** Called when the user skips the word, with an optional custom date. */
  onSkip?: (customDate?: string) => void;
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
}: FeedbackActionsProps) {
  const [showDatePicker, setShowDatePicker] = useState(false);
  const [customDate, setCustomDate] = useState("");

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
          <Text fontSize="sm" fontWeight="medium">
            Next review: {formatReviewDate(nextReviewDate)}
          </Text>
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
          onClick={onOverride}
        >
          {isCorrect ? "Mark as Incorrect" : "Mark as Correct"}
        </Button>
      ) : null}

      {/* Skip button or Skipped label */}
      {isSkipped ? (
        <Text fontSize="sm" color="fg.muted" fontStyle="italic">
          Skipped
        </Text>
      ) : canOverrideOrSkip && onSkip ? (
        <>
          {showDatePicker ? (
            <VStack align="stretch" gap={2}>
              <Text fontSize="sm" fontWeight="medium">
                Skip until:
              </Text>
              <Input
                type="date"
                size="sm"
                value={customDate}
                min={tomorrowStr}
                onChange={(e) => setCustomDate(e.target.value)}
              />
              <Box display="flex" gap={2}>
                <Button
                  flex="1"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    onSkip(customDate || undefined);
                    setShowDatePicker(false);
                  }}
                >
                  Confirm Skip
                </Button>
                <Button
                  flex="1"
                  size="sm"
                  variant="ghost"
                  onClick={() => setShowDatePicker(false)}
                >
                  Cancel
                </Button>
              </Box>
            </VStack>
          ) : (
            <Button
              w="full"
              variant="outline"
              colorPalette="gray"
              onClick={() => setShowDatePicker(true)}
            >
              Skip
            </Button>
          )}
        </>
      ) : noteId === undefined && !isOverridden && !isSkipped ? (
        // Freeform quiz: noteId is not available from the SubmitFreeformAnswerResponse proto,
        // so override/skip are disabled. The backend SubmitFreeformAnswer handler knows the
        // noteId but does not return it in the response. A proto change would be needed to
        // enable these actions for freeform quizzes.
        null
      ) : null}
    </VStack>
  );
}
