"use client";

import { Box, Button, Text } from "@chakra-ui/react";
import type { WordDetail } from "@/store/quizStore";
import { WordDetailView } from "./WordDetailView";
import { OriginBreakdown, type OriginPartDisplay } from "./OriginBreakdown";

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
    correct: {
      bg: "green.100",
      darkBg: "green.900",
      color: "green.700",
      darkColor: "green.200",
      glyph: "✓",
      label: "Correct",
    },
    incorrect: {
      bg: "red.100",
      darkBg: "red.900",
      color: "red.700",
      darkColor: "red.200",
      glyph: "✗",
      label: "Incorrect",
    },
    skipped: {
      bg: "gray.100",
      darkBg: "gray.700",
      color: "gray.600",
      darkColor: "gray.300",
      glyph: "—",
      label: "Excluded",
    },
  }[kind];

  return (
    <Box
      bg={styles.bg}
      _dark={{ bg: styles.darkBg }}
      color={styles.color}
      px={2}
      py={0.5}
      borderRadius="full"
      display="inline-flex"
      alignItems="center"
      gap={1}
      flexShrink={0}
    >
      <Text as="span" fontSize="xs" fontWeight="bold" _dark={{ color: styles.darkColor }}>
        {styles.glyph}
      </Text>
      <Text as="span" fontSize="xs" fontWeight="medium" _dark={{ color: styles.darkColor }}>
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
    ? "gray.200"
    : item.correct
      ? "green.200"
      : "red.200";

  // "Your answer" chip styling
  const answerChipBg = item.correct ? "green.50" : "red.50";
  const answerChipDarkBg = item.correct ? "green.950" : "red.950";
  const answerChipBorder = item.correct ? "green.300" : "red.300";
  const answerChipDarkBorder = item.correct ? "green.700" : "red.700";
  const answerIcon = item.correct ? "✓" : "✗";
  const answerIconColor = item.correct ? "green.600" : "red.600";
  const answerIconDarkColor = item.correct ? "green.300" : "red.300";

  return (
    <Box
      borderWidth="1px"
      borderColor={borderColor}
      borderRadius="md"
      p={3}
      opacity={item.isSkipped ? 0.7 : 1}
      _dark={{ borderColor }}
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
          _dark={{ bg: answerChipDarkBg, borderColor: answerChipDarkBorder }}
          maxW="full"
        >
          <Text as="span" fontSize="xs" fontWeight="bold" color={answerIconColor} _dark={{ color: answerIconDarkColor }}>
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
          borderLeftColor="gray.300"
          _dark={{ borderLeftColor: "gray.600" }}
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

      {/* Footer: Mark as Correct/Incorrect, Undo when overridden, Exclude/Resume. */}
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
          <Button
            size="sm"
            variant="ghost"
            colorPalette="blue"
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
                colorPalette="blue"
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
