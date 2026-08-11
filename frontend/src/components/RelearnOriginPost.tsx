"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Box, Button, Input, Spinner, Text } from "@chakra-ui/react";
import {
  quizClient,
  QuizType,
  type RelearnCard,
  type SubmitRelearnAnswerResponse,
} from "@/lib/client";
import { OriginBreakdown } from "@/components/OriginBreakdown";
import RelearnContext from "@/components/RelearnContext";
import { responseTimeSince } from "@/lib/responseTime";

// RelearnOriginPost presents ONE etymology origin the way the etymology family
// card does: the origin (with its meaning) heads the screen, and every missed
// word that shares that origin is listed with an inline box to recall it. A
// family can MIX directions — each word is drilled in the direction it was
// missed (originDirection):
//
//   - recognition (STANDARD / unset) → show the WORD, type its MEANING.
//   - reverse (REVERSE)              → show the MEANING (+ masked contexts),
//                                       type the WORD.
//
// Each word grades individually through SubmitRelearnAnswer keyed by its own
// note_id — the backend picks the matching grader from the card's direction, so
// a reverse word is graded produce-the-word (not the meaning grader). Grouping
// is purely presentation, so the per-word grading path (and relearn's
// write-nothing guarantee) is unchanged.
//
// Relearn re-drills missed words and persists NOTHING (see
// .claude/rules/quiz-ui-invariants). There is no skip / "Don't know" and no
// Exclude control here — a word is one of two states:
//
//   - unanswered  → on "See answers" it is graded INCORRECT (a normal miss).
//                   SubmitRelearnAnswer persists nothing, so the word stays
//                   "misunderstood" and therefore DUE.
//   - answered    → correct / incorrect from the grader.
//
// On "Next", onComplete reports the words answered WRONG (unanswered→incorrect
// included) so the caller can re-queue just those as a smaller family screen and
// re-drill them this session until they are answered correctly — mirroring the
// single-card re-drill at the group level. Excluding a word from future quizzes
// is done only in the normal quizzes; a Relearn miss never writes any state.

interface GradedWord {
  answer: string;
  res: SubmitRelearnAnswerResponse;
}

export interface RelearnOriginPostProps {
  originText: string;
  originMeaning: string;
  type: string;
  language: string;
  englishForms: string[];
  words: RelearnCard[];
  // onComplete fires on "Next" with the words answered WRONG this pass
  // (unanswered→incorrect included) and how many were correct. The caller
  // re-queues the wrong words so they are re-drilled this session.
  onComplete: (wrongWords: RelearnCard[], correctCount: number) => void;
}

export function RelearnOriginPost({
  originText,
  originMeaning,
  type,
  language,
  englishForms,
  words,
  onComplete,
}: RelearnOriginPostProps) {
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, GradedWord>>({});
  const [grading, setGrading] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const inputRefs = useRef<Map<string, HTMLInputElement>>(new Map());
  const rowRefs = useRef<Map<string, HTMLElement>>(new Map());
  const focusTimeRef = useRef<Map<string, number>>(new Map());

  const orderedKeys = useMemo(() => words.map((w) => w.noteId.toString()), [words]);
  const wordByKey = useMemo(() => {
    const m = new Map<string, RelearnCard>();
    for (const w of words) m.set(w.noteId.toString(), w);
    return m;
  }, [words]);

  const originBadge = [type, language].filter(Boolean).join(" · ");

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
  const selectedWord = selectedKey ? wordByKey.get(selectedKey) : undefined;

  // Keep the selected row scrolled into view alongside the pinned feedback sheet,
  // so the word and its meaning/why are visible together — never buried below the
  // Next button on a long family.
  useEffect(() => {
    if (selectedKey) {
      rowRefs.current.get(selectedKey)?.scrollIntoView?.({ block: "center", behavior: "smooth" });
    }
  }, [selectedKey]);

  // grade one word. An empty value is the "unanswered → incorrect" case (the
  // backend grades an empty answer wrong). reveal opens the feedback sheet
  // automatically when the result is not correct.
  const grade = async (word: RelearnCard, value: string, reveal: boolean) => {
    const key = word.noteId.toString();
    const startedAt = focusTimeRef.current.get(key);
    const responseTimeMs = startedAt !== undefined ? responseTimeSince(startedAt) : BigInt(0);
    setGrading((g) => (g.includes(key) ? g : [...g, key]));
    try {
      const res = await quizClient.submitRelearnAnswer({
        noteId: word.noteId,
        answer: value,
        isSkipped: false,
        responseTimeMs,
      });
      setResults((r) => ({ ...r, [key]: { answer: value, res } }));
      setError(null);
      if (reveal && !res.correct) setSelectedKey(key);
    } catch {
      setError("A word couldn't be graded. Re-type it to retry.");
    } finally {
      setGrading((g) => g.filter((k) => k !== key));
    }
  };

  // Grade one word the moment its box is committed (Enter / blur), and jump the
  // cursor to the next unanswered box.
  const commitWord = (word: RelearnCard, indexInOrder: number) => {
    const key = word.noteId.toString();
    const value = (inputs[key] ?? "").trim();
    if (!value || isDone(key)) return;
    const nextKey = nextUnansweredAfter(indexInOrder);
    if (nextKey) setTimeout(() => inputRefs.current.get(nextKey)?.focus(), 0);
    void grade(word, value, true);
  };

  // Reveal any still-empty words by grading them INCORRECT (a normal miss —
  // never skipped), then open the first not-correct word.
  const revealAnswers = async () => {
    const remaining = words.filter((w) => !isDone(w.noteId.toString()));
    await Promise.all(remaining.map((w) => grade(w, "", false)));
    const firstToShow =
      orderedKeys.find((k) => results[k] && !results[k].res.correct) ??
      remaining[0]?.noteId.toString() ??
      orderedKeys[0];
    if (firstToShow) setSelectedKey(firstToShow);
  };

  const setInput = (key: string, value: string) =>
    setInputs((prev) => ({ ...prev, [key]: value }));

  return (
    <Box pb={selected ? 80 : 0} maxW="100%">
      <Box display="flex" justifyContent="space-between" mb={2}>
        <Text fontSize="xs" color="purple.500" _dark={{ color: "purple.300" }} fontWeight="medium">
          Etymology — recall each word
        </Text>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} aria-live="polite">
          {correctCount} / {orderedKeys.length} correct
          {grading.length > 0 ? " · grading…" : ""}
        </Text>
      </Box>

      {/* Origin header — the shared root, its meaning, and the family below it. */}
      <Box
        p={4}
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.700" }}
        borderWidth="1px"
        borderColor="gray.200"
        borderRadius="lg"
        mb={4}
        maxW="100%"
        data-testid="relearn-origin-post"
      >
        <OriginHeader originText={originText} originBadge={originBadge} originMeaning={originMeaning} englishForms={englishForms} />

        <Box display="flex" flexDirection="column" gap={2} mt={3}>
          {words.map((word, indexInOrder) => {
            const key = word.noteId.toString();
            const graded = results[key];
            const isGrading = grading.includes(key);

            if (graded) {
              const isSel = key === selectedKey;
              const c = graded.res.correct
                ? { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200", glyph: "✓" }
                : { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200", glyph: "✗" };
              return (
                <Box
                  key={key}
                  ref={(el: HTMLElement | null) => {
                    if (el) rowRefs.current.set(key, el);
                    else rowRefs.current.delete(key);
                  }}
                  role="button"
                  tabIndex={0}
                  aria-pressed={isSel}
                  aria-label={`${word.entry} — ${graded.res.correct ? "correct" : "incorrect"}`}
                  onClick={() => setSelectedKey(key)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setSelectedKey(key);
                    }
                  }}
                  cursor="pointer"
                  display="inline-flex"
                  alignItems="baseline"
                  gap={2}
                  px={2}
                  py={1}
                  borderRadius="md"
                  bg={c.bg}
                  color={c.color}
                  _dark={{ bg: c.darkBg, color: c.darkColor }}
                  boxShadow={isSel ? "0 0 0 2px var(--chakra-colors-blue-500)" : undefined}
                  maxW="100%"
                  overflowWrap="anywhere"
                >
                  <Text as="span" fontWeight="bold" aria-hidden="true">{c.glyph}</Text>
                  <Text as="span" fontWeight="medium">{word.entry}</Text>
                </Box>
              );
            }

            if (isGrading) {
              return (
                <Box key={key} display="inline-flex" alignItems="baseline" gap={2}>
                  <Text as="span" fontWeight="bold" color="blue.600" _dark={{ color: "blue.300" }}>
                    {word.entry}
                  </Text>
                  <Spinner size="xs" />
                </Box>
              );
            }

            const reverse = word.originDirection === QuizType.REVERSE;
            const input = (
              <Input
                ref={(el: HTMLInputElement | null) => {
                  if (el) inputRefs.current.set(key, el);
                  else inputRefs.current.delete(key);
                }}
                size="sm"
                display="inline-block"
                w="auto"
                minW="8rem"
                maxW="100%"
                aria-label={reverse ? `Word for "${word.meaning}"` : `Meaning for "${word.entry}"`}
                placeholder={reverse ? "the word" : "meaning"}
                value={inputs[key] ?? ""}
                onFocus={() => focusTimeRef.current.set(key, Date.now())}
                onChange={(e) => setInput(key, e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    commitWord(word, indexInOrder);
                  }
                }}
                onBlur={() => commitWord(word, indexInOrder)}
              />
            );

            // Reverse: show the MEANING (and masked contexts) as the prompt and
            // ask for the word. Recognition: show the WORD and ask its meaning.
            if (reverse) {
              return (
                <Box key={key} display="flex" flexDirection="column" gap={1} maxW="100%">
                  <Text fontWeight="semibold" color="gray.700" _dark={{ color: "gray.200" }} overflowWrap="anywhere">
                    {word.meaning}
                  </Text>
                  {word.contexts.map((c, i) => (
                    <Text key={i} fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} overflowWrap="anywhere">
                      {c.maskedContext || c.context}
                    </Text>
                  ))}
                  {input}
                </Box>
              );
            }

            return (
              <Box key={key} display="flex" flexDirection="column" gap={1} maxW="100%">
                <Box display="inline-flex" flexWrap="wrap" alignItems="baseline" gap={2} maxW="100%">
                  <Text as="span" fontWeight="bold" color="blue.600" _dark={{ color: "blue.300" }} overflowWrap="anywhere">
                    {word.entry}
                  </Text>
                  {input}
                </Box>
                {/* Full example as usage context while asking — the same hint the
                    single-card recognition screen shows. Reverse words mask the
                    word instead (the contexts branch above); recognition shows it
                    in full because the WORD, not the answer, is on screen. */}
                {word.examples.map((ex, i) => (
                  <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }} overflowWrap="anywhere">
                    {ex.speaker ? `${ex.speaker}: ` : ""}
                    {ex.text}
                  </Text>
                ))}
              </Box>
            );
          })}
        </Box>
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
          // A word committed just before Next is still grading; leaving Next
          // enabled lets the learner advance past it and discard the in-flight
          // grade (the reported bug). Disable Next until every grade has landed.
          disabled={grading.length > 0}
          onClick={() =>
            onComplete(
              words.filter((w) => results[w.noteId.toString()]?.res.correct !== true),
              correctCount,
            )
          }
          data-testid="relearn-origin-next"
        >
          {grading.length > 0 ? "Grading…" : "Next"}
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
            Tap any word above to review it.
          </Text>
        )}
      </Box>

      {/* Feedback for the word just graded (wrong answers) opens here
          automatically, PINNED to the bottom so it stays visible without
          scrolling past the Next button — the word, its meaning, the origin
          breakdown, the literal gloss, and what the learner typed. */}
      {selected && selectedWord && (
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
          data-testid="relearn-origin-feedback"
        >
          <Box display="flex" alignItems="center" justifyContent="space-between" mb={2}>
            <Text
              fontSize="xs"
              fontWeight="bold"
              color={selected.res.correct ? "green.600" : "red.600"}
              _dark={{ color: selected.res.correct ? "green.300" : "red.300" }}
            >
              {selected.res.correct ? "✓ Correct" : "✗ Incorrect"}
            </Text>
            <Button size="xs" variant="ghost" onClick={() => setSelectedKey(null)} aria-label="Close details">
              ✕
            </Button>
          </Box>

          <Text fontWeight="bold">{selectedWord.entry}</Text>
          <Text fontSize="sm" mb={1}>
            <Text as="span" fontWeight="semibold">Meaning: </Text>
            <Text as="span" data-testid="relearn-answer">
              {selected.res.meaning || selectedWord.meaning}
            </Text>
          </Text>
          {selected.answer.trim() && (
            <Text fontSize="sm" color={selected.res.correct ? "gray.500" : "red.600"} _dark={{ color: selected.res.correct ? "gray.400" : "red.300" }}>
              <Text as="span" fontWeight="semibold">Your answer: </Text>
              {selected.answer}
            </Text>
          )}
          {selected.res.reason && (
            <Text fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }} mt={1}>
              {selected.res.reason}
            </Text>
          )}
          {selected.res.wordDetail?.originParts && selected.res.wordDetail.originParts.length > 0 && (
            <Box mt={2}>
              <Text fontSize="xs" color="fg.muted" mb={1}>Origin</Text>
              <OriginBreakdown
                parts={selected.res.wordDetail.originParts.map((p) => ({
                  origin: p.origin,
                  meaning: p.meaning,
                  language: p.language,
                  type: p.type,
                }))}
              />
            </Box>
          )}
          {selected.res.literal && (
            <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} fontStyle="italic" mt={1}>
              {selected.res.literal}
            </Text>
          )}
          {/* Where-it-appears example scenes for the graded word, mirroring the
              single-card Relearn screen. RelearnContext returns null on empty
              scenes, so a word without examples renders nothing extra. */}
          <RelearnContext
            entry={selectedWord.entry}
            scenes={selected.res.contextScenes ?? []}
            exampleWords={[]}
          />
        </Box>
      )}
    </Box>
  );
}

// OriginHeader renders the origin header: the source-language root, an optional
// type/language badge, the origin's English gloss, and — when present — its
// English combining-form spellings as small chips (study context for learning
// the derived vocabulary; e.g. the Latin root "liber" → lib, liv).
function OriginHeader({
  originText,
  originBadge,
  originMeaning,
  englishForms,
}: {
  originText: string;
  originBadge: string;
  originMeaning: string;
  englishForms: string[];
}) {
  return (
    <Box>
      <Text fontSize="lg" fontWeight="bold" overflowWrap="anywhere">{originText}</Text>
      {originBadge && (
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>{originBadge}</Text>
      )}
      {originMeaning && (
        <Text fontSize="sm" color="gray.700" _dark={{ color: "gray.200" }}>{originMeaning}</Text>
      )}
      {englishForms.length > 0 && (
        <Box display="flex" flexWrap="wrap" gap={1} mt={2} data-testid="relearn-origin-english-forms">
          {englishForms.map((form) => (
            <Text
              key={form}
              as="span"
              fontSize="xs"
              fontWeight="medium"
              px={2}
              py={0.5}
              borderRadius="md"
              bg="purple.100"
              color="purple.700"
              _dark={{ bg: "purple.900", color: "purple.200" }}
              overflowWrap="anywhere"
            >
              {form}
            </Text>
          ))}
        </Box>
      )}
    </Box>
  );
}
