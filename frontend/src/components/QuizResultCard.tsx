"use client";

import { Box, Button, Text } from "@chakra-ui/react";

export interface OriginPartDisplay {
  origin: string;
  meaning: string;
  language?: string;
  type?: string;
}

export interface ResultItem {
  index: number;
  key: string;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  noteId?: bigint;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalCorrect: boolean;
  originBreakdown?: OriginPartDisplay[];
  userAnswer?: string;
  images?: string[];
  reason?: string;
  pronunciation?: string;
  partOfSpeech?: string;
}

function getTypeBadgeColors(type: string): { bg: string; darkBg: string; color: string; darkColor: string } {
  switch (type.toLowerCase()) {
    case "root":
      return { bg: "blue.100", darkBg: "blue.900", color: "blue.600", darkColor: "blue.300" };
    case "prefix":
      return { bg: "yellow.100", darkBg: "yellow.900", color: "yellow.800", darkColor: "yellow.200" };
    case "suffix":
      return { bg: "green.100", darkBg: "green.900", color: "green.800", darkColor: "green.200" };
    default:
      return { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300" };
  }
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
      glyph: "\u2713",
      label: "Correct",
    },
    incorrect: {
      bg: "red.100",
      darkBg: "red.900",
      color: "red.700",
      darkColor: "red.200",
      glyph: "\u2717",
      label: "Incorrect",
    },
    skipped: {
      bg: "gray.100",
      darkBg: "gray.700",
      color: "gray.600",
      darkColor: "gray.300",
      glyph: "\u2014",
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
  isEtymology: boolean;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function QuizResultCard({
  item,
  isEtymology,
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
  const answerIcon = item.correct ? "\u2713" : "\u2717";
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

      {/* Your-answer chip (only for non-etymology; etymology renders in its own block below) */}
      {!isEtymology && item.userAnswer && (
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

      {/* Etymology origin breakdown with badges */}
      {isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
        <Box mb={2}>
          {!item.correct && item.userAnswer && (
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
          <Text fontSize="xs" color="fg.muted" mb={1}>
            {item.correct ? "Breakdown" : "Correct"}
          </Text>
          <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
            {item.originBreakdown.map((p, i) => {
              const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
              return (
                <Box key={i} display="flex" alignItems="center" gap={1}>
                  {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                  <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
                  <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
                  {p.language && (
                    <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                      <Text fontSize="2xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                    </Box>
                  )}
                  {typeBadge && p.type && (
                    <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                      <Text fontSize="2xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
                    </Box>
                  )}
                </Box>
              );
            })}
          </Box>
        </Box>
      )}

      {/* Non-etymology origin breakdown (shown for vocabulary quizzes when word has etymology data) */}
      {!isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
        <Box mb={2}>
          <Text fontSize="xs" color="fg.muted" mb={1}>Etymology</Text>
          <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
            {item.originBreakdown.map((p, i) => {
              const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
              return (
                <Box key={i} display="flex" alignItems="center" gap={1}>
                  {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                  <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
                  <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
                  {p.language && (
                    <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                      <Text fontSize="2xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                    </Box>
                  )}
                  {typeBadge && p.type && (
                    <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                      <Text fontSize="2xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
                    </Box>
                  )}
                </Box>
              );
            })}
          </Box>
        </Box>
      )}

      {/* Footer: small buttons left-aligned, + Undo link when overridden */}
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
