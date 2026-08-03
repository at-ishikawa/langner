"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Box, Button, Input, Spinner, Text } from "@chakra-ui/react";
import {
  quizClient,
  QuizType as ProtoQuizType,
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
//
// The model has NO skip / "Don't know" (see .claude/rules/quiz-ui-invariants):
//
//   - unanswered  → on "See answers" it is graded INCORRECT (a normal miss).
//                   SubmitRelearnAnswer persists nothing, so the correction
//                   stays "misunderstood" in the learning history and therefore
//                   stays DUE, returning in the next Relearn session.
//   - answered    → correct / incorrect from the grader.
//   - Excluded    → the learner deliberately removed this correction from all
//                   future quizzes. This is the ONLY thing that excludes: it
//                   calls SkipWord (SetSkippedAt) — the same RPC every other
//                   relearn / quiz card uses — never a normal or empty answer.
//
// A normal miss (incorrect) MUST NOT set the exclude marker; only the Exclude
// button does.

interface GradedBlank {
  answer: string;
  res: SubmitRelearnAnswerResponse;
}

export interface RelearnGrammarPostProps {
  content: string;
  blanks: RelearnCard[];
  onComplete: (correctCount: number, blankCount: number) => void;
}

function pillStatus(g: GradedBlank): "correct" | "incorrect" {
  return g.res.correct ? "correct" : "incorrect";
}

function pillLabel(g: GradedBlank): string {
  return g.res.correct ? "correct" : "incorrect";
}

export function RelearnGrammarPost({ content, blanks, onComplete }: RelearnGrammarPostProps) {
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, GradedBlank>>({});
  const [grading, setGrading] = useState<string[]>([]);
  // excluded holds the keys the learner deliberately removed from future
  // quizzes via SkipWord. An excluded blank is neither answered nor graded and
  // is dropped from the post's active blanks (denominator and remaining count).
  const [excluded, setExcluded] = useState<string[]>([]);
  const [excluding, setExcluding] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const inputRefs = useRef<Map<string, HTMLInputElement>>(new Map());
  const pillRefs = useRef<Map<string, HTMLElement>>(new Map());
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

  const isExcluded = (key: string) => excluded.includes(key);
  // A blank is "done" for navigation once it is graded, grading, or excluded —
  // an excluded blank is never focused or graded again.
  const isDone = (key: string) =>
    key in results || grading.includes(key) || isExcluded(key);
  const nextUnansweredAfter = (afterIndex: number): string | null => {
    for (let i = afterIndex + 1; i < orderedKeys.length; i++) {
      if (!isDone(orderedKeys[i])) return orderedKeys[i];
    }
    for (let i = 0; i <= afterIndex; i++) {
      if (!isDone(orderedKeys[i])) return orderedKeys[i];
    }
    return null;
  };

  // Excluded blanks leave the post entirely — they don't count toward the
  // score, the remaining tally, or the answers reported on completion.
  const activeKeys = orderedKeys.filter((k) => !isExcluded(k));
  const gradedCount = activeKeys.filter((k) => k in results).length;
  const correctCount = activeKeys.filter((k) => results[k]?.res.correct).length;
  const remainingCount = activeKeys.filter((k) => !isDone(k)).length;
  const selected = selectedKey ? results[selectedKey] : undefined;
  const selectedBlank = selectedKey ? blankByKey.get(selectedKey) : undefined;

  // Keep the selected pill scrolled into view alongside the pinned feedback
  // sheet, so the correction and its mistake/suggested/why are visible together
  // — never buried at the bottom of a long post below the Next button.
  useEffect(() => {
    if (selectedKey) {
      pillRefs.current.get(selectedKey)?.scrollIntoView?.({ block: "center", behavior: "smooth" });
    }
  }, [selectedKey]);

  // grade one blank. An empty value is the "unanswered → incorrect" case (the
  // backend grades an empty answer wrong deterministically). reveal opens the
  // feedback sheet automatically when the result is not correct.
  const grade = async (blank: RelearnCard, value: string, reveal: boolean) => {
    const key = blank.noteId.toString();
    const startedAt = focusTimeRef.current.get(key);
    const responseTimeMs = startedAt !== undefined ? responseTimeSince(startedAt) : BigInt(0);
    setGrading((g) => (g.includes(key) ? g : [...g, key]));
    try {
      const res = await quizClient.submitRelearnAnswer({
        noteId: blank.noteId,
        answer: value,
        isSkipped: false,
        responseTimeMs,
      });
      setResults((r) => ({ ...r, [key]: { answer: value, res } }));
      setError(null);
      if (reveal && !res.correct) setSelectedKey(key);
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
    void grade(blank, value, true);
  };

  // Exclude THIS correction from all future quizzes. This is the deliberate
  // exclude action — it calls SkipWord (SetSkippedAt) for the blank's
  // (notebook, senseID), the same RPC every other relearn / quiz card uses. It
  // never grades the blank incorrect; the correction simply leaves the pool.
  const excludeBlank = async (blank: RelearnCard) => {
    const key = blank.noteId.toString();
    if (isExcluded(key) || excluding.includes(key)) return;
    setExcluding((e) => [...e, key]);
    try {
      await quizClient.skipWord({
        noteId: blank.noteId,
        quizTypes: [ProtoQuizType.GRAMMAR],
      });
      setExcluded((ex) => (ex.includes(key) ? ex : [...ex, key]));
      setError(null);
      setSelectedKey((sel) => (sel === key ? null : sel));
    } catch {
      setError("Couldn't exclude that correction. Try again.");
    } finally {
      setExcluding((e) => e.filter((k) => k !== key));
    }
  };

  // Reveal any still-empty blanks by grading them INCORRECT (a normal miss —
  // never skipped, never excluded), then open the first not-correct blank so an
  // answer is visible right away. Excluded blanks are left untouched.
  const revealAnswers = async () => {
    const remaining = blanks.filter((b) => !isDone(b.noteId.toString()));
    await Promise.all(remaining.map((b) => grade(b, "", false)));
    const firstToShow =
      activeKeys.find((k) => results[k] && !results[k].res.correct) ??
      remaining[0]?.noteId.toString() ??
      activeKeys[0];
    if (firstToShow) setSelectedKey(firstToShow);
  };

  const setInput = (key: string, value: string) =>
    setInputs((prev) => ({ ...prev, [key]: value }));

  return (
    <Box pb={selected ? 80 : 0} maxW="100%">
      <Box display="flex" justifyContent="space-between" mb={2}>
        <Text fontSize="xs" color="purple.500" _dark={{ color: "purple.300" }} fontWeight="medium">
          Grammar — fix the mistakes
        </Text>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} aria-live="polite">
          {correctCount} / {activeKeys.length} correct
          {grading.length > 0 ? " · grading…" : ""}
        </Text>
      </Box>

      <Text fontSize="xs" color="fg.muted" mb={2}>
        Type each fix and press Enter. Tap “See answers” to reveal the rest —
        anything you didn’t answer is marked incorrect and stays due. Use
        “Exclude” to drop a correction from future quizzes. Tap a graded word for
        details.
      </Text>

      {/* The whole post, mistakes fixed in place — one screen, all due blanks.
          overflowWrap/wordBreak keep long prose and long unbroken tokens inside
          the viewport, so the post never scrolls horizontally. */}
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
        overflowWrap="anywhere"
        wordBreak="break-word"
        maxW="100%"
        data-testid="relearn-grammar-post"
      >
        {segments.map((seg, i) => {
          if (seg.type === "text") return <span key={i}>{seg.text}</span>;
          const key = seg.blank.noteId.toString();
          const indexInOrder = orderedKeys.indexOf(key);
          const graded = results[key];
          const isGrading = grading.includes(key);

          // Excluded → a muted, non-interactive marker; it no longer counts.
          if (isExcluded(key)) {
            return (
              <Text
                as="span"
                key={i}
                mx={0.5}
                color="gray.500"
                _dark={{ color: "gray.400" }}
                fontStyle="italic"
                aria-label={`${seg.blank.incorrect || "correction"} — excluded from quizzes`}
              >
                {seg.blank.incorrect} <Text as="span" fontSize="xs">(excluded)</Text>
              </Text>
            );
          }

          // Graded → tappable pill.
          if (graded) {
            const c = PILL_STYLES[pillStatus(graded)];
            const isSel = key === selectedKey;
            return (
              <Text
                as="span"
                key={i}
                ref={(el: HTMLElement | null) => {
                  if (el) pillRefs.current.set(key, el);
                  else pillRefs.current.delete(key);
                }}
                role="button"
                tabIndex={0}
                aria-pressed={isSel}
                aria-label={`${seg.blank.incorrect || "correction"} — ${pillLabel(graded)}`}
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
                overflowWrap="anywhere"
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
              <Text as="span" key={i}>
                <Text
                  as="span"
                  fontWeight="bold"
                  color="blue.600"
                  _dark={{ color: "blue.300" }}
                  textDecoration="line-through"
                  overflowWrap="anywhere"
                >
                  {seg.blank.incorrect}
                </Text>
                <Spinner size="xs" mx={1} verticalAlign="middle" />
              </Text>
            );
          }

          // Not yet graded → the wrong span with an inline textbox and a
          // per-blank "Exclude" (the deliberate remove-from-quizzes action).
          // inline-flex + flexWrap keeps the group together but lets it wrap
          // within the viewport instead of forcing a horizontal scroll.
          return (
            <Text
              as="span"
              key={i}
              display="inline-flex"
              flexWrap="wrap"
              alignItems="baseline"
              gap={1}
              mx={1}
              maxW="100%"
              verticalAlign="baseline"
            >
              <Text
                as="span"
                fontWeight="bold"
                color="blue.600"
                _dark={{ color: "blue.300" }}
                textDecoration="line-through"
                overflowWrap="anywhere"
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
                minW="5rem"
                maxW="100%"
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
              <Button
                size="xs"
                variant="ghost"
                colorPalette="gray"
                verticalAlign="baseline"
                loading={excluding.includes(key)}
                // onMouseDown so the click registers before the input's onBlur
                // commits a typed answer — Exclude must never grade the blank.
                onMouseDown={(e) => {
                  e.preventDefault();
                  void excludeBlank(seg.blank);
                }}
                aria-label={`Exclude "${seg.blank.incorrect}" from quizzes`}
              >
                Exclude
              </Button>
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
          onClick={() => onComplete(correctCount, activeKeys.length)}
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

      <Box mt={2} minH="1rem" textAlign="center">
        {remainingCount === 0 && gradedCount > 0 && (
          <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
            Tap any correction above to review it.
          </Text>
        )}
      </Box>

      {/* Feedback for the blank just graded (wrong answers) opens here
          automatically, and stays PINNED to the bottom of the viewport so it is
          visible without scrolling past the Next button on a long post — the
          live grammar quiz's own labelled body (Mistake / You wrote / Suggested
          / Why you missed it / Grammar note). Session-only: relearn persists
          nothing, so there is no override-to-history footer — but the blank can
          still be Excluded from here. */}
      {selected && selectedBlank && (
        <Box
          position="fixed"
          bottom={0}
          left={0}
          right={0}
          maxW="sm"
          mx="auto"
          maxH="60vh"
          overflowY="auto"
          overflowX="hidden"
          bg="white"
          _dark={{ bg: "gray.900", borderTopColor: "gray.700" }}
          borderTopWidth="1px"
          borderTopColor="gray.200"
          borderTopRadius="xl"
          p={3}
          boxShadow="0 -4px 12px rgba(0,0,0,0.08)"
          data-testid="relearn-grammar-feedback"
        >
          <Box display="flex" alignItems="center" justifyContent="space-between" mb={2}>
            <Text
              fontSize="xs"
              fontWeight="bold"
              color={selected.res.correct ? "green.600" : "red.600"}
              _dark={{ color: selected.res.correct ? "green.300" : "red.300" }}
            >
              {selected.res.correct ? "✓ Correct" : "✗ Incorrect"}
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
            assessment={selected.res.reason}
            grammarNote={selected.res.grammarNote}
            correctAnswerTestId="relearn-answer"
          />
          <Button
            mt={2}
            size="xs"
            w="full"
            variant="outline"
            colorPalette="gray"
            loading={selectedKey ? excluding.includes(selectedKey) : false}
            onClick={() => void excludeBlank(selectedBlank)}
            aria-label={`Exclude "${selectedBlank.incorrect}" from quizzes`}
          >
            Exclude from quizzes
          </Button>
        </Box>
      )}
    </Box>
  );
}
