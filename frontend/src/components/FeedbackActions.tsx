"use client";

import { Box, Button, Text, VStack } from "@chakra-ui/react";

interface FeedbackActionsProps {
  /** Whether the current answer is displayed as correct (possibly after override). */
  isCorrect: boolean;
  /** noteId for the word, needed for override/skip RPCs. undefined disables those actions. */
  noteId?: bigint;
  /** Whether the result has been overridden. */
  isOverridden: boolean;
  /** Whether the word has been skipped. */
  isSkipped: boolean;
  /**
   * Whether to render the Exclude-from-quizzes control (and the "Excluded"
   * label). Defaults to true. Relearn passes false: Relearn re-drills missed
   * items and persists nothing, so excluding a word is not offered there —
   * exclusion is done only from the normal quizzes.
   */
  showExclude?: boolean;
  /** Label for the primary navigation button. */
  nextLabel: string;
  /** Called when the user clicks the primary Continue button. */
  onNext: () => void;
  /** Called when the user overrides the result. */
  onOverride?: () => void;
  /** Called when the user undoes an override. */
  onUndo?: () => void;
  /** Called when the user skips the word. */
  onSkip?: () => void;
  /** Called when the user wants to see results early (before finishing all cards). */
  onSeeResults?: () => void;
  /** Page-specific content rendered between the banner and action buttons. */
  children?: React.ReactNode;
}

export function FeedbackActions({
  isCorrect,
  noteId,
  isOverridden,
  isSkipped,
  showExclude = true,
  nextLabel,
  onNext,
  onOverride,
  onUndo,
  onSkip,
  onSeeResults,
  children,
}: FeedbackActionsProps) {
  const canOverrideOrSkip = noteId !== undefined && !isOverridden && !isSkipped;

  return (
    <VStack align="stretch" gap={3}>
      {/* Correct/Incorrect banner */}
      <Box
        p={3}
        borderRadius="md"
        bg={isCorrect ? "green.subtle" : "red.subtle"}
        color={isCorrect ? "green.fg" : "red.fg"}
        display="flex"
        justifyContent="space-between"
        alignItems="center"
      >
        <Text fontWeight="bold">
          {isCorrect ? "\u2713 Correct" : "\u2717 Incorrect"}
          {isOverridden && (
            <Text as="span" fontWeight="normal" fontStyle="italic"> (overridden)</Text>
          )}
        </Text>
        {isOverridden && onUndo && (
          <Text
            as="span"
            fontSize="sm"
            cursor="pointer"
            textDecoration="underline"
            onClick={onUndo}
          >
            Undo
          </Text>
        )}
      </Box>

      {/* Page-specific content between banner and buttons */}
      {children}

      {/* Primary navigation button */}
      <Button w="full" colorPalette="brand" onClick={onNext}>
        {nextLabel}
      </Button>

      {/* Override button */}
      {canOverrideOrSkip && onOverride ? (
        <Button
          w="full"
          variant="outline"
          colorPalette={isCorrect ? "red" : "brand"}
          onClick={onOverride}
        >
          {isCorrect ? "Mark as incorrect" : "Mark as correct"}
        </Button>
      ) : null}

      {/* Exclude button or Excluded label. Hidden entirely when showExclude is
          false (Relearn), where exclusion is not offered. */}
      {!showExclude ? null : isSkipped ? (
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
          Exclude from quizzes
        </Button>
      ) : null}

      {/* Stop the session early — quiet secondary so it never competes with the
          primary Continue button. */}
      {onSeeResults && (
        <Button
          w="full"
          variant="ghost"
          size="sm"
          color="fg.muted"
          onClick={onSeeResults}
        >
          Finish
        </Button>
      )}
    </VStack>
  );
}
