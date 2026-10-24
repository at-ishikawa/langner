"use client";

import { Box, Button, Text } from "@chakra-ui/react";
import type { GrammarResultState } from "@/store/grammarStore";
import type { ResultItem } from "@/components/QuizResultCard";
import { grammarResultToItem } from "@/lib/grammarResultItems";
import { GrammarCorrectionBody } from "@/components/GrammarCorrectionBody";

// GrammarFeedbackCard replaces the shared vocabulary card for grammar results.
// The vocabulary card shows a short word + meaning; grammar answers are full
// sentences that differ from the correction by only a word or two, so three
// unlabeled near-identical lines were impossible to tell apart. This card
// adds the status header + override/skip footer around GrammarCorrectionBody,
// which does the labelled-lines + diff rendering (shared with the Relearn
// Quiz's grammar card).

const STATUS = {
  correct: { bg: "green.subtle", color: "green.fg", glyph: "✓", label: "Correct", border: "green.emphasized" },
  incorrect: { bg: "red.subtle", color: "red.fg", glyph: "✗", label: "Incorrect", border: "red.emphasized" },
  skipped: { bg: "gray.subtle", color: "gray.fg", glyph: "—", label: "Excluded", border: "border" },
} as const;

function statusOf(r: GrammarResultState): keyof typeof STATUS {
  if (r.isSkipped) return "skipped";
  return r.correct ? "correct" : "incorrect";
}

interface GrammarFeedbackCardProps {
  result: GrammarResultState;
  index: number;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function GrammarFeedbackCard({
  result,
  index,
  onOverride,
  onUndo,
  onSkip,
  onResume,
}: GrammarFeedbackCardProps) {
  const s = STATUS[statusOf(result)];
  const item = grammarResultToItem(result, index);

  return (
    <Box
      borderWidth="1px"
      borderColor={s.border}
      borderRadius="md"
      p={3}
      opacity={result.isSkipped ? 0.75 : 1}
    >
      {/* Header: status + category */}
      <Box display="flex" alignItems="center" gap={2} mb={2}>
        <Box
          bg={s.bg}
          px={2}
          py={0.5}
          borderRadius="full"
          display="inline-flex"
          alignItems="center"
          gap={1}
          flexShrink={0}
        >
          <Text as="span" fontSize="xs" fontWeight="bold" color={s.color}>
            {s.glyph}
          </Text>
          <Text as="span" fontSize="xs" fontWeight="medium" color={s.color}>
            {s.label}
          </Text>
        </Box>
        {result.isOverridden && (
          <Text as="span" fontSize="xs" color="fg.muted" fontStyle="italic">
            (overridden)
          </Text>
        )}
        {result.category && (
          <Text fontSize="xs" color="fg.muted" ml="auto">
            {result.category}
          </Text>
        )}
      </Box>

      <GrammarCorrectionBody
        incorrect={result.incorrect}
        answer={result.answer}
        correctAnswer={result.correctAnswer}
        correct={result.correct}
        isSkipped={result.isSkipped}
        assessment={result.assessment}
        grammarNote={result.reason}
      />

      {/* Footer actions — mirror QuizResultCard so override/skip reuse the same
          RPCs. */}
      <Box display="flex" flexWrap="wrap" gap={2} alignItems="center">
        {!item.isOverridden && !item.isSkipped && item.noteId && item.learnedAt && (
          <Button
            size="sm"
            variant="outline"
            colorPalette={item.correct ? "red" : "brand"}
            onClick={() => onOverride(item)}
          >
            {item.correct ? "Mark as incorrect" : "Mark as correct"}
          </Button>
        )}
        {item.isOverridden && item.noteId && item.learnedAt && (
          <Button size="sm" variant="ghost" colorPalette="brand" onClick={() => onUndo(item)}>
            Undo override
          </Button>
        )}
        {item.isSkipped
          ? item.noteId && (
              <Button size="sm" variant="outline" colorPalette="brand" onClick={() => onResume(item)}>
                Resume
              </Button>
            )
          : !item.isOverridden && item.noteId && (
              <Button size="sm" variant="outline" colorPalette="gray" onClick={() => onSkip(item)}>
                Exclude
              </Button>
            )}
      </Box>
    </Box>
  );
}
