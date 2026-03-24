"use client";

import { useState } from "react";
import { Button, Text, VStack } from "@chakra-ui/react";

interface FeedbackActionsProps {
  /** Whether the current answer was correct (possibly after override). */
  isCorrect: boolean;
  /** noteId for the word, needed for override/skip RPCs. undefined disables those actions. */
  noteId?: bigint;
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
}

export function FeedbackActions({
  isCorrect,
  noteId,
  isOverridden,
  isSkipped,
  nextLabel,
  onNext,
  onOverride,
  onSkip,
}: FeedbackActionsProps) {
  const canOverrideOrSkip = noteId !== undefined && !isOverridden && !isSkipped;

  return (
    <VStack align="stretch" gap={3}>

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
