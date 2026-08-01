"use client";

import { Box, Button, Text } from "@chakra-ui/react";
import type { GrammarResultState } from "@/store/grammarStore";
import type { ResultItem } from "@/components/QuizResultCard";
import { grammarResultToItem } from "@/lib/grammarResultItems";
import { wordDiff, type DiffToken } from "@/lib/wordDiff";

// GrammarFeedbackCard replaces the shared vocabulary card for grammar results.
// The vocabulary card shows a short word + meaning; grammar answers are full
// sentences that differ from the correction by only a word or two, so three
// unlabeled near-identical lines were impossible to tell apart. This card
// labels each line (Mistake / You wrote / Suggested / Why) and diff-highlights
// exactly the words that differ, so the reader sees what still needs fixing.

const STATUS = {
  correct: { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200", glyph: "✓", label: "Correct", border: "green.200", darkBorder: "green.700" },
  incorrect: { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200", glyph: "✗", label: "Incorrect", border: "red.200", darkBorder: "red.700" },
  skipped: { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300", glyph: "—", label: "Excluded", border: "gray.200", darkBorder: "gray.600" },
} as const;

function statusOf(r: GrammarResultState): keyof typeof STATUS {
  if (r.isSkipped) return "skipped";
  return r.correct ? "correct" : "incorrect";
}

// Render diffed tokens; changed words get a tinted background so they stand out.
function DiffLine({ tokens, tone }: { tokens: DiffToken[]; tone: "user" | "correct" }) {
  const changedBg = tone === "user" ? "red.100" : "green.100";
  const changedDarkBg = tone === "user" ? "red.900" : "green.900";
  const changedColor = tone === "user" ? "red.700" : "green.700";
  const changedDarkColor = tone === "user" ? "red.200" : "green.200";
  return (
    <Text fontSize="sm" wordBreak="break-word">
      {tokens.map((t, i) => (
        <Text as="span" key={i}>
          {i > 0 ? " " : ""}
          {t.changed ? (
            <Text
              as="span"
              px={1}
              borderRadius="sm"
              fontWeight="bold"
              bg={changedBg}
              color={changedColor}
              _dark={{ bg: changedDarkBg, color: changedDarkColor }}
            >
              {t.text}
            </Text>
          ) : (
            t.text
          )}
        </Text>
      ))}
    </Text>
  );
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
  // Diff the user's answer against the suggested correction so each side
  // highlights the words the other is missing.
  const diff = wordDiff(result.answer, result.correctAnswer);

  return (
    <Box
      borderWidth="1px"
      borderColor={s.border}
      _dark={{ borderColor: s.darkBorder }}
      borderRadius="md"
      p={3}
      opacity={result.isSkipped ? 0.75 : 1}
    >
      {/* Header: status + category */}
      <Box display="flex" alignItems="center" gap={2} mb={2}>
        <Box
          bg={s.bg}
          _dark={{ bg: s.darkBg }}
          px={2}
          py={0.5}
          borderRadius="full"
          display="inline-flex"
          alignItems="center"
          gap={1}
          flexShrink={0}
        >
          <Text as="span" fontSize="xs" fontWeight="bold" color={s.color} _dark={{ color: s.darkColor }}>
            {s.glyph}
          </Text>
          <Text as="span" fontSize="xs" fontWeight="medium" color={s.color} _dark={{ color: s.darkColor }}>
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

      {/* Mistake (original), muted — also the anchor for the wrong span. */}
      <Box display="flex" gap={2} mb={1}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          Mistake
        </Text>
        <Text fontSize="sm" color="fg.muted" textDecoration="line-through" wordBreak="break-word">
          {result.incorrect}
        </Text>
      </Box>

      {/* You wrote — changed words vs the correction highlighted. */}
      <Box display="flex" gap={2} mb={1}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          You wrote
        </Text>
        {result.answer.trim() ? (
          <DiffLine tokens={diff.left} tone="user" />
        ) : (
          <Text fontSize="sm" color="fg.muted" fontStyle="italic">
            (skipped)
          </Text>
        )}
      </Box>

      {/* Suggested — the words you still needed highlighted. */}
      <Box display="flex" gap={2} mb={2}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          Suggested
        </Text>
        <DiffLine tokens={diff.right} tone="correct" />
      </Box>

      {/* Why — the note on the mistake. */}
      {result.reason && (
        <Text fontSize="sm" color="fg.muted" fontStyle="italic" mb={3}>
          {result.reason}
        </Text>
      )}

      {/* Footer actions — mirror QuizResultCard so override/skip reuse the same
          RPCs. */}
      <Box display="flex" flexWrap="wrap" gap={2} alignItems="center">
        {!item.isOverridden && !item.isSkipped && item.noteId && item.learnedAt && (
          <Button
            size="sm"
            variant="outline"
            colorPalette={item.correct ? "red" : "blue"}
            onClick={() => onOverride(item)}
          >
            {item.correct ? "Mark as Incorrect" : "Mark as Correct"}
          </Button>
        )}
        {item.isOverridden && item.noteId && item.learnedAt && (
          <Button size="sm" variant="ghost" colorPalette="blue" onClick={() => onUndo(item)}>
            Undo override
          </Button>
        )}
        {item.isSkipped
          ? item.noteId && (
              <Button size="sm" variant="outline" colorPalette="blue" onClick={() => onResume(item)}>
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
