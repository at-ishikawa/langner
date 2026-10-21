"use client";

import { Box, Button, Text } from "@chakra-ui/react";
import type { WordDetail } from "@/store/quizStore";
import { WordDetailView } from "./WordDetailView";
import { OriginBreakdown, type OriginPartDisplay } from "./OriginBreakdown";
import { PronounceButton } from "./PronounceButton";

export type { OriginPartDisplay };

export interface ResultItem {
  index: number;
  key: string;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  noteId?: bigint;
  /** senseId is the stable source-entry identity echoed from the Submit
   * response. Threaded back into OverrideAnswer/UndoOverrideAnswer so the
   * override targets the exact sense the card came from. */
  senseId?: string;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalCorrect: boolean;
  /** originBreakdown lists the word's etymology origins with their meanings.
   * Shown for any vocabulary word that carries etymology data — the origin of
   * the word is surfaced in feedback like every other vocabulary detail. */
  originBreakdown?: OriginPartDisplay[];
  userAnswer?: string;
  images?: string[];
  reason?: string;
  pronunciation?: string;
  partOfSpeech?: string;
  /** Full word detail (origin prose, synonyms, antonyms, memo). Origin parts
   * are rendered via originBreakdown — the wordDetail passed to
   * WordDetailView has its originParts stripped to avoid duplication. */
  wordDetail?: WordDetail;
}

interface StatusChipProps {
  kind: "correct" | "incorrect" | "skipped";
}

function StatusChip({ kind }: StatusChipProps) {
  const styles = {
    correct: { bg: "green.subtle", color: "green.fg", glyph: "✓", label: "Correct" },
    incorrect: { bg: "red.subtle", color: "red.fg", glyph: "✗", label: "Incorrect" },
    skipped: { bg: "gray.subtle", color: "gray.fg", glyph: "—", label: "Excluded" },
  }[kind];

  return (
    <Box
      bg={styles.bg}
      color={styles.color}
      px={2}
      py={0.5}
      borderRadius="full"
      display="inline-flex"
      alignItems="center"
      gap={1}
      flexShrink={0}
    >
      <Text as="span" fontSize="xs" fontWeight="bold">
        {styles.glyph}
      </Text>
      <Text as="span" fontSize="xs" fontWeight="medium">
        {styles.label}
      </Text>
    </Box>
  );
}

interface QuizResultCardProps {
  item: ResultItem;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function QuizResultCard({
  item,
  onOverride,
  onUndo,
  onSkip,
  onResume,
}: QuizResultCardProps) {
  const statusKind: "correct" | "incorrect" | "skipped" = item.isSkipped
    ? "skipped"
    : item.correct
      ? "correct"
      : "incorrect";

  const borderColor = item.isSkipped
    ? "border"
    : item.correct
      ? "green.emphasized"
      : "red.emphasized";

  // "Your answer" chip styling
  const answerChipBg = item.correct ? "green.subtle" : "red.subtle";
  const answerChipBorder = item.correct ? "green.emphasized" : "red.emphasized";
  const answerIcon = item.correct ? "✓" : "✗";
  const answerIconColor = item.correct ? "green.fg" : "red.fg";

  return (
    <Box
      borderWidth="1px"
      borderColor={borderColor}
      borderRadius="md"
      p={3}
      opacity={item.isSkipped ? 0.7 : 1}
    >
      {/* Header: status chip (left), entry, pron/POS (right) */}
      <Box display="flex" alignItems="center" gap={2} mb={2}>
        <StatusChip kind={statusKind} />
        <Text fontWeight="bold" flex="1" minW={0}>
          {item.entry}
          {item.isOverridden && (
            <Text as="span" fontSize="xs" color="fg.muted" fontStyle="italic" fontWeight="normal">
              {" "}(overridden)
            </Text>
          )}
        </Text>
        {/* The entry (headword) is always revealed on this result card, so
            playing its pronunciation leaks nothing. */}
        <PronounceButton text={item.entry} />

        {(item.pronunciation || item.partOfSpeech) && (
          <Text fontSize="xs" color="fg.muted" flexShrink={0}>
            {[
              item.pronunciation && `/${item.pronunciation}/`,
              item.partOfSpeech,
            ].filter(Boolean).join(" · ")}
          </Text>
        )}
      </Box>

      {/* Meaning (primary). Reason is appended with em-dash in italic muted. */}
      <Text fontSize="sm" mb={2}>
        {item.meaning}
        {item.reason && (
          <Text as="span" color="fg.muted" fontStyle="italic">
            {" — "}
            {item.reason}
          </Text>
        )}
      </Text>

      {/* Your-answer chip */}
      {item.userAnswer && (
        <Box
          display="inline-flex"
          alignItems="center"
          gap={2}
          px={2}
          py={1}
          mb={2}
          borderWidth="1px"
          borderColor={answerChipBorder}
          borderRadius="md"
          bg={answerChipBg}
          maxW="full"
        >
          <Text as="span" fontSize="xs" fontWeight="bold" color={answerIconColor}>
            {answerIcon}
          </Text>
          <Text fontSize="sm" color="fg.muted">
            <Text as="span" fontSize="xs">your answer · </Text>
            <Text as="span" color="fg">&ldquo;{item.userAnswer}&rdquo;</Text>
          </Text>
        </Box>
      )}

      {/* Context: italic with a left-accent border */}
      {item.contexts && item.contexts.length > 0 && (
        <Box
          borderLeftWidth="3px"
          borderLeftColor="border"
          pl={2}
          mb={2}
        >
          {item.contexts.map((ctx, i) => (
            <Text key={i} fontSize="sm" fontStyle="italic" color="fg.muted">
              {ctx}
            </Text>
          ))}
        </Box>
      )}

      {/* Images */}
      {item.images && item.images.length > 0 && (
        <Box display="flex" gap={2} mb={2} flexWrap="wrap">
          {item.images.map((src, i) => (
            <img key={i} src={src} alt="" style={{ maxHeight: "150px", borderRadius: "4px" }} />
          ))}
        </Box>
      )}

      {/* Origin breakdown — shown for any vocabulary word that carries etymology
          data, so the origin of the word is surfaced in feedback. */}
      {item.originBreakdown && item.originBreakdown.length > 0 && (
        <Box mb={2}>
          <Text fontSize="xs" color="fg.muted" mb={1}>Etymology</Text>
          <OriginBreakdown parts={item.originBreakdown} />
        </Box>
      )}

      {/* Extra word details (origin prose, synonyms, antonyms, memo). Origin
          parts are stripped so WordDetailView doesn't render the etymology
          section a second time. */}
      {item.wordDetail && (
        <Box mb={3}>
          <WordDetailView wordDetail={{ ...item.wordDetail, originParts: undefined }} />
        </Box>
      )}

      {/* Footer: Mark as correct/Incorrect, Undo when overridden, Exclude/Resume. */}
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
          <Button
            size="sm"
            variant="ghost"
            colorPalette="brand"
            onClick={() => onUndo(item)}
          >
            Undo override
          </Button>
        )}

        {item.isSkipped
          ? item.noteId && (
              <Button
                size="sm"
                variant="outline"
                colorPalette="brand"
                onClick={() => onResume(item)}
              >
                Resume
              </Button>
            )
          : !item.isOverridden && item.noteId && (
              <Button
                size="sm"
                variant="outline"
                colorPalette="gray"
                onClick={() => onSkip(item)}
              >
                Exclude
              </Button>
            )}
      </Box>
    </Box>
  );
}
