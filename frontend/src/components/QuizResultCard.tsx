"use client";

import { Box, Button, Text, VStack } from "@chakra-ui/react";
import type { WordDetail } from "@/store/quizStore";
import { WordDetailView } from "./WordDetailView";

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
  /** senseId is the stable source-entry identity echoed from the Submit
   * response. Threaded back into OverrideAnswer/UndoOverrideAnswer so the
   * override targets the exact sense the card came from. */
  senseId?: string;
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
  /** Full word detail (origin prose, synonyms, antonyms, memo). Origin parts
   * are rendered via originBreakdown — the wordDetail passed to
   * WordDetailView has its originParts stripped to avoid duplication. */
  wordDetail?: WordDetail;
  /** etymologyForms are the origin's principal parts (e.g. "facio",
   * "facere", "feci", "factum"). Rendered joined under the origin header
   * on the etymology feedback card. */
  etymologyForms?: string[];
  /** etymologyEnglishForms are the English combining-form spellings the
   * origin surfaces as inside English words (e.g. "fac", "fic", "fect").
   * Study context, distinct from etymologyForms. Rendered as chips on the
   * etymology feedback card. */
  etymologyEnglishForms?: string[];
  /** etymologyNote is the origin's free-text pedagogical hint, shown under
   * the origin header on the etymology feedback card. */
  etymologyNote?: string;
  /** etymologyWords are the per-word results for an etymology-origin card:
   * each derived family word, whether the typed meaning was correct, the
   * correct meaning, and the reason. Rendered as a list on the feedback
   * card so the learner sees every word's outcome even though the origin
   * is graded as one aggregate result. Correct/excluded are overridable
   * per word via onOverrideWord/onExcludeWord — the override flips inline
   * data on the origin's ONE stored record, never a second record per word
   * (learning-history invariants L1/L4). */
  etymologyWords?: {
    expression: string;
    correct: boolean;
    correctMeaning: string;
    reason: string;
    userAnswer: string;
    originalCorrect?: boolean;
    isExcluded?: boolean;
    /** skipped is true when the learner tapped "Don't Know" for this word;
     * it is neither correct nor incorrect and didn't affect sibling grading
     * or the origin's own aggregate result. */
    skipped?: boolean;
    /** pronunciation, examples, and literal are per-word study context shown
     * on the feedback screen alongside the graded meaning. */
    pronunciation?: string;
    examples?: string[];
    literal?: string;
  }[];
}

/** EtymologyWordItem is the shape QuizResultCard.etymologyWords carries per
 * derived family word — pulled out so onOverrideWord/onExcludeWord can share
 * one parameter type. */
export type EtymologyWordItem = NonNullable<ResultItem["etymologyWords"]>[number];

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
  /** onOverrideWord/onExcludeWord flip ONE derived family word's
   * correct/excluded flag within the origin's existing record — only
   * rendered when isEtymology and item.etymologyWords are set. */
  onOverrideWord?: (item: ResultItem, word: EtymologyWordItem) => void;
  onExcludeWord?: (item: ResultItem, word: EtymologyWordItem) => void;
}

export function QuizResultCard({
  item,
  isEtymology,
  onOverride,
  onUndo,
  onSkip,
  onResume,
  onOverrideWord,
  onExcludeWord,
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

      {/* Extra word details (origin prose, synonyms, antonyms, memo). Origin
          parts are stripped so WordDetailView doesn't render the etymology
          section a second time. */}
      {item.wordDetail && (
        <Box mb={3}>
          <WordDetailView wordDetail={{ ...item.wordDetail, originParts: undefined }} />
        </Box>
      )}

      {/* Origin principal parts (e.g. "facio, facere, feci, factum").
          Etymology-only context echoed from the card. */}
      {isEtymology && item.etymologyForms && item.etymologyForms.length > 0 && (
        <Text fontSize="xs" color="fg.muted" mb={2} fontStyle="italic">
          {item.etymologyForms.join(", ")}
        </Text>
      )}

      {/* English combining forms (e.g. "fac", "fic", "fect") as chips —
          study context distinct from the source-language principal parts. */}
      {isEtymology && item.etymologyEnglishForms && item.etymologyEnglishForms.length > 0 && (
        <Box display="flex" flexWrap="wrap" gap={1.5} mb={2}>
          {item.etymologyEnglishForms.map((ef) => (
            <Box key={ef} px={2} py={0.5} borderRadius="md" bg="teal.100" _dark={{ bg: "teal.900" }}>
              <Text fontSize="xs" fontFamily="mono" color="teal.700" _dark={{ color: "teal.200" }}>{ef}</Text>
            </Box>
          ))}
        </Box>
      )}

      {/* Origin note: free-text pedagogical hint about the root. */}
      {isEtymology && item.etymologyNote && (
        <Text fontSize="xs" color="fg.muted" mb={2} fontStyle="italic">
          {item.etymologyNote}
        </Text>
      )}

      {/* Per-word results for an etymology-origin card. The origin is graded
          as one aggregate result, but every derived family word's outcome is
          shown so the learner can see which words they missed. Mark as
          Correct/Incorrect and Exclude here flip that word's flags inline on
          the origin's ONE stored record (invariants L1/L4) — they never
          create a second record for the word. */}
      {isEtymology && item.etymologyWords && item.etymologyWords.length > 0 && (
        <Box mb={2}>
          <Text fontSize="xs" color="fg.muted" mb={1}>Words</Text>
          <VStack align="stretch" gap={1}>
            {item.etymologyWords.map((w, i) => {
              const wordOverridden = w.originalCorrect !== undefined && w.originalCorrect !== w.correct;
              const canOverrideWord = Boolean(onOverrideWord && item.noteId && item.learnedAt);
              const canExcludeWord = Boolean(onExcludeWord && item.noteId && item.learnedAt);
              const borderColor = w.skipped ? "gray.200" : w.correct ? "green.200" : "red.200";
              const darkBorderColor = w.skipped ? "gray.600" : w.correct ? "green.800" : "red.800";
              const glyphColor = w.skipped ? "gray.500" : w.correct ? "green.600" : "red.600";
              const darkGlyphColor = w.skipped ? "gray.400" : w.correct ? "green.300" : "red.300";
              const glyph = w.skipped ? "—" : w.correct ? "✓" : "✗";
              return (
                <Box
                  key={i}
                  borderWidth="1px"
                  borderColor={borderColor}
                  _dark={{ borderColor: darkBorderColor }}
                  borderRadius="md"
                  px={2}
                  py={1}
                  opacity={w.isExcluded || w.skipped ? 0.7 : 1}
                >
                  <Box display="flex" alignItems="center" gap={2}>
                    <Text as="span" fontSize="xs" fontWeight="bold" color={glyphColor} _dark={{ color: darkGlyphColor }}>
                      {glyph}
                    </Text>
                    <Text as="span" fontSize="sm" fontWeight="medium" flex="1" minW={0}>
                      {w.expression}
                      {w.skipped && (
                        <Text as="span" fontSize="xs" color="fg.muted" fontStyle="italic" fontWeight="normal">
                          {" "}(skipped)
                        </Text>
                      )}
                      {wordOverridden && (
                        <Text as="span" fontSize="xs" color="fg.muted" fontStyle="italic" fontWeight="normal">
                          {" "}(overridden)
                        </Text>
                      )}
                      {w.isExcluded && (
                        <Text as="span" fontSize="xs" color="fg.muted" fontStyle="italic" fontWeight="normal">
                          {" "}(excluded)
                        </Text>
                      )}
                    </Text>
                  </Box>
                  {w.pronunciation && (
                    <Text fontSize="xs" color="fg.muted">/{w.pronunciation}/</Text>
                  )}
                  <Text fontSize="sm" color="fg.muted">
                    {w.correctMeaning}
                    {w.reason && (
                      <Text as="span" fontStyle="italic">
                        {" — "}
                        {w.reason}
                      </Text>
                    )}
                  </Text>
                  {/* Literal gloss (e.g. `de "down" + facere = "made down"`)
                      revealed only here on feedback, not while answering. */}
                  {w.literal && (
                    <Text fontSize="xs" color="fg.muted" fontStyle="italic">{w.literal}</Text>
                  )}
                  {/* Example sentence(s) — also feedback-only to avoid leaking
                      the meaning on the answering screen. */}
                  {w.examples && w.examples.length > 0 && (
                    <VStack align="stretch" gap={0.5} mt={0.5}>
                      {w.examples.map((ex, ei) => (
                        <Text key={ei} fontSize="xs" color="fg.muted">
                          &ldquo;{ex}&rdquo;
                        </Text>
                      ))}
                    </VStack>
                  )}
                  {w.userAnswer && (
                    <Text fontSize="xs" color="fg.muted">
                      your answer · &ldquo;{w.userAnswer}&rdquo;
                    </Text>
                  )}
                  {(canOverrideWord || canExcludeWord) && (
                    <Box display="flex" flexWrap="wrap" gap={2} mt={1}>
                      {canOverrideWord && (
                        <Button
                          size="xs"
                          variant="outline"
                          colorPalette={w.correct ? "red" : "blue"}
                          aria-label={`Mark ${w.expression} as ${w.correct ? "incorrect" : "correct"}`}
                          onClick={() => onOverrideWord?.(item, w)}
                        >
                          {w.correct ? "Mark as Incorrect" : "Mark as Correct"}
                        </Button>
                      )}
                      {canExcludeWord && (
                        <Button
                          size="xs"
                          variant="outline"
                          colorPalette="gray"
                          aria-label={`${w.isExcluded ? "Include" : "Exclude"} ${w.expression}`}
                          onClick={() => onExcludeWord?.(item, w)}
                        >
                          {w.isExcluded ? "Include" : "Exclude"}
                        </Button>
                      )}
                    </Box>
                  )}
                </Box>
              );
            })}
          </VStack>
        </Box>
      )}

      {/* Footer: small buttons left-aligned, + Undo link when overridden.
          Origin-level Mark as Correct/Incorrect and Exclude/Resume are
          meaningless for a multi-word etymology-origin card — the per-word
          actions above are the correct granularity — so this footer is
          suppressed entirely for isEtymology. Unaffected for every other
          quiz mode, where a single origin-level action is still correct. */}
      {!isEtymology && (
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
      )}
    </Box>
  );
}
