"use client";

import { useMemo, useRef, useState } from "react";
import { Box, Button, Input, Spinner, Text } from "@chakra-ui/react";
import {
  quizClient,
  type RelearnCard,
  type SubmitRelearnAnswerResponse,
} from "@/lib/client";
import { segmentPost, PILL_STYLES } from "@/lib/grammarSegments";
import { GrammarCorrectionBody } from "@/components/GrammarCorrectionBody";
import { responseTimeSince } from "@/lib/responseTime";

// RelearnGrammarPost presents ONE journal post the way the live grammar quiz
// does: the whole entry is shown once, every due mistake has an inline textbox,
// and each box is graded the moment it is committed (Enter / blur) with its
// result shown in place — progressive per-blank feedback, not a single
// submit-then-reveal or one-blank-per-card. It reuses the live quiz's segmenting
// (segmentPost), status palette (PILL_STYLES), and feedback body
// (GrammarCorrectionBody) so a graded correction reads identically in both
// places.
//
// Each blank still grades individually through SubmitRelearnAnswer, keyed by its
// own note_id — grouping is purely presentation, so the per-correction grading
// path (and relearn's write-nothing guarantee) is unchanged.

interface GradedBlank {
  answer: string;
  isSkipped: boolean;
  res: SubmitRelearnAnswerResponse;
}

export interface RelearnGrammarPostProps {
  content: string;
  blanks: RelearnCard[];
  onComplete: (correctCount: number, blankCount: number) => void;
}

function pillStatus(g: GradedBlank): "correct" | "incorrect" | "skipped" {
  if (g.isSkipped) return "skipped";
  return g.res.correct ? "correct" : "incorrect";
}

export function RelearnGrammarPost({ content, blanks, onComplete }: RelearnGrammarPostProps) {
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, GradedBlank>>({});
  const [grading, setGrading] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const inputRefs = useRef<Map<string, HTMLInputElement>>(new Map());
  const focusTimeRef = useRef<Map<string, number>>(new Map());

  const segments = useMemo(() => segmentPost(content, blanks), [content, blanks]);
  const orderedKeys = useMemo(
    () =>
      segments
        .filter((s): s is { type: "blank"; blank: RelearnCard } => s.type === "blank")
        .map((s) => s.blank.noteId.toString()),
    [segments],
  );

  const blankByKey = useMemo(() => {
    const map = new Map<string, RelearnCard>();
    for (const b of blanks) map.set(b.noteId.toString(), b);
    return map;
  }, [blanks]);

  const isDone = (key: string) => key in results || grading.includes(key);
  const nextUnansweredAfter = (afterIndex: number): string | null => {
    for (let i = afterIndex + 1; i < orderedKeys.length; i++) {
      if (!isDone(orderedKeys[i])) return orderedKeys[i];
    }
    for (let i = 0; i <= afterIndex; i++) {
      if (!isDone(orderedKeys[i])) return orderedKeys[i];
    }
    return null;
  };

  const gradedCount = orderedKeys.filter((k) => k in results).length;
  const correctCount = orderedKeys.filter((k) => results[k]?.res.correct).length;
  const remainingCount = orderedKeys.filter((k) => !isDone(k)).length;
  const selected = selectedKey ? results[selectedKey] : undefined;
  const selectedBlank = selectedKey ? blankByKey.get(selectedKey) : undefined;

  const grade = async (blank: RelearnCard, value: string, isSkipped: boolean) => {
    const key = blank.noteId.toString();
    const startedAt = focusTimeRef.current.get(key);
    const responseTimeMs = startedAt !== undefined ? responseTimeSince(startedAt) : BigInt(0);
    setGrading((g) => (g.includes(key) ? g : [...g, key]));
    try {
      const res = await quizClient.submitRelearnAnswer({
        noteId: blank.noteId,
        answer: isSkipped ? "" : value,
        isSkipped,
        responseTimeMs,
      });
      setResults((r) => ({ ...r, [key]: { answer: value, isSkipped, res } }));
      setError(null);
    } catch {
      setError("A correction couldn't be graded. Re-type it to retry.");
    } finally {
      setGrading((g) => g.filter((k) => k !== key));
    }
  };

  // Grade one blank the moment its box is committed (Enter / blur), and jump the
  // cursor to the next unanswered box — the live grammar quiz's interaction.
  const commitBlank = (blank: RelearnCard, indexInOrder: number) => {
    const key = blank.noteId.toString();
    const value = (inputs[key] ?? "").trim();
    if (!value || isDone(key)) return;
    const nextKey = nextUnansweredAfter(indexInOrder);
    if (nextKey) setTimeout(() => inputRefs.current.get(nextKey)?.focus(), 0);
    void grade(blank, value, false);
  };

  // Reveal any still-empty blanks by grading them as skipped, then open the
  // first not-correct blank so an answer is visible right away.
  const revealAnswers = async () => {
    const remaining = blanks.filter((b) => !isDone(b.noteId.toString()));
    await Promise.all(remaining.map((b) => grade(b, "", true)));
    const firstToShow =
      orderedKeys.find((k) => results[k] && !results[k].res.correct) ?? orderedKeys[0];
    if (firstToShow) setSelectedKey(firstToShow);
  };

  const setInput = (key: string, value: string) =>
    setInputs((prev) => ({ ...prev, [key]: value }));

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" mb={2}>
        <Text fontSize="xs" color="purple.500" _dark={{ color: "purple.300" }} fontWeight="medium">
          Grammar — fix the mistakes
        </Text>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} aria-live="polite">
          {correctCount} / {orderedKeys.length} correct
          {grading.length > 0 ? " · grading…" : ""}
        </Text>
      </Box>

      <Text fontSize="xs" color="fg.muted" mb={2}>
        Type each fix and press Enter. Tap a graded word for details.
      </Text>

      {/* The whole post, mistakes fixed in place — one screen, all due blanks. */}
      <Box
        p={4}
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.700" }}
        borderWidth="1px"
        borderColor="gray.200"
        borderRadius="lg"
        mb={4}
        fontSize="sm"
        lineHeight="2.2"
        whiteSpace="pre-wrap"
        overflowX="hidden"
        wordBreak="break-word"
        data-testid="relearn-grammar-post"
      >
        {segments.map((seg, i) => {
          if (seg.type === "text") return <span key={i}>{seg.text}</span>;
          const key = seg.blank.noteId.toString();
          const indexInOrder = orderedKeys.indexOf(key);
          const graded = results[key];
          const isGrading = grading.includes(key);

          // Graded → tappable pill.
          if (graded) {
            const c = PILL_STYLES[pillStatus(graded)];
            const isSel = key === selectedKey;
            return (
              <Text
                as="span"
                key={i}
                role="button"
                tabIndex={0}
                aria-pressed={isSel}
                aria-label={`${seg.blank.incorrect || "correction"} — ${c.label}`}
                onClick={() => setSelectedKey(key)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setSelectedKey(key);
                  }
                }}
                cursor="pointer"
                mx={0.5}
                px={1.5}
                py={0.5}
                borderRadius="md"
                fontWeight="bold"
                bg={c.bg}
                color={c.color}
                _dark={{ bg: c.darkBg, color: c.darkColor }}
                boxShadow={isSel ? "0 0 0 2px var(--chakra-colors-blue-500)" : undefined}
              >
                <Text as="span" aria-hidden="true">
                  {c.glyph}{" "}
                </Text>
                {seg.blank.incorrect}
              </Text>
            );
          }

          // Grading in flight → the wrong span with a small spinner in place.
          if (isGrading) {
            return (
              <Text as="span" key={i} whiteSpace="nowrap">
                <Text
                  as="span"
                  fontWeight="bold"
                  color="blue.600"
                  _dark={{ color: "blue.300" }}
                  textDecoration="line-through"
                >
                  {seg.blank.incorrect}
                </Text>
                <Spinner size="xs" mx={1} verticalAlign="middle" />
              </Text>
            );
          }

          // Not yet graded → the wrong span with an inline textbox.
          return (
            <Text as="span" key={i}>
              <Text
                as="span"
                fontWeight="bold"
                color="blue.600"
                _dark={{ color: "blue.300" }}
                textDecoration="line-through"
              >
                {seg.blank.incorrect}
              </Text>
              <Input
                ref={(el: HTMLInputElement | null) => {
                  if (el) inputRefs.current.set(key, el);
                  else inputRefs.current.delete(key);
                }}
                size="sm"
                display="inline-block"
                w="auto"
                minW="6rem"
                maxW="100%"
                mx={1}
                verticalAlign="baseline"
                aria-label={`Correction for "${seg.blank.incorrect}"`}
                placeholder="fix"
                value={inputs[key] ?? ""}
                onFocus={() => focusTimeRef.current.set(key, Date.now())}
                onChange={(e) => setInput(key, e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    commitBlank(seg.blank, indexInOrder);
                  }
                }}
                onBlur={() => commitBlank(seg.blank, indexInOrder)}
              />
            </Text>
          );
        })}
      </Box>

      {remainingCount > 0 ? (
        <Button colorPalette="blue" w="full" size="lg" onClick={() => void revealAnswers()}>
          See answers ({remainingCount} left)
        </Button>
      ) : (
        <Button
          colorPalette="purple"
          w="full"
          size="lg"
          onClick={() => onComplete(correctCount, orderedKeys.length)}
          data-testid="relearn-grammar-next"
        >
          Next
        </Button>
      )}
      {error && (
        <Text fontSize="xs" color="red.500" mt={1} textAlign="center" role="alert">
          {error}
        </Text>
      )}

      {/* Detail: opens when a graded word is tapped — the live grammar quiz's
          own labelled feedback body (Mistake / You wrote / Suggested / Why you
          missed it / Grammar note). Session-only: relearn persists nothing, so
          there is no override-to-history footer. */}
      {selected && selectedBlank && (
        <Box
          mt={4}
          p={3}
          bg="white"
          _dark={{ bg: "gray.900", borderColor: "gray.700" }}
          borderWidth="1px"
          borderColor="gray.200"
          borderRadius="lg"
        >
          <Box display="flex" alignItems="center" justifyContent="space-between" mb={2}>
            <Text fontSize="xs" fontWeight="bold" color={selected.res.correct ? "green.600" : "red.600"} _dark={{ color: selected.res.correct ? "green.300" : "red.300" }}>
              {selected.isSkipped ? "– Excluded" : selected.res.correct ? "✓ Correct" : "✗ Incorrect"}
              {selected.res.category ? ` · ${selected.res.category}` : ""}
            </Text>
            <Button size="xs" variant="ghost" onClick={() => setSelectedKey(null)} aria-label="Close details">
              ✕
            </Button>
          </Box>
          <GrammarCorrectionBody
            incorrect={selectedBlank.incorrect}
            answer={selected.answer}
            correctAnswer={selected.res.correctAnswer}
            correct={selected.res.correct}
            isSkipped={selected.isSkipped}
            assessment={selected.res.reason}
            grammarNote={selected.res.grammarNote}
            correctAnswerTestId="relearn-answer"
          />
        </Box>
      )}

      <Box mt={2} minH="1rem" textAlign="center">
        {remainingCount === 0 && gradedCount > 0 && (
          <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
            Tap any correction above to review it.
          </Text>
        )}
      </Box>
    </Box>
  );
}
