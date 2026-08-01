"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Box,
  Button,
  Heading,
  Input,
  Progress,
  Spinner,
  Text,
} from "@chakra-ui/react";
import { quizClient, type GrammarPostCard, type GrammarBlank } from "@/lib/client";
import { useGrammarStore, type GrammarResultState } from "@/store/grammarStore";
import { QuizResultCard } from "@/components/QuizResultCard";
import { grammarResultToItem } from "@/lib/grammarResultItems";
import { useGrammarResultActions } from "@/lib/useGrammarResultActions";
import { responseTimeSince } from "@/lib/responseTime";

// A post is rendered as an ordered list of plain-text runs and blank tokens so
// each mistake is corrected in place. Blanks are located by their incorrect
// span; duplicates map to successive occurrences in blank order. Any blank whose
// span can't be placed is appended at the end so the rendered set always equals
// the graded set.
type Segment =
  | { type: "text"; text: string }
  | { type: "blank"; blank: GrammarBlank };

function segmentPost(postText: string, blanks: GrammarBlank[]): Segment[] {
  const cursors = new Map<string, number>();
  const placed: { blank: GrammarBlank; pos: number }[] = [];
  const unplaced: GrammarBlank[] = [];
  for (const blank of blanks) {
    const from = cursors.get(blank.incorrect) ?? 0;
    const pos = blank.incorrect ? postText.indexOf(blank.incorrect, from) : -1;
    if (pos >= 0) {
      cursors.set(blank.incorrect, pos + blank.incorrect.length);
      placed.push({ blank, pos });
    } else {
      unplaced.push(blank);
    }
  }
  placed.sort((a, b) => a.pos - b.pos);

  const segments: Segment[] = [];
  let cursor = 0;
  for (const { blank, pos } of placed) {
    if (pos < cursor) {
      unplaced.push(blank);
      continue;
    }
    if (pos > cursor) segments.push({ type: "text", text: postText.slice(cursor, pos) });
    segments.push({ type: "blank", blank });
    cursor = pos + blank.incorrect.length;
  }
  if (cursor < postText.length) segments.push({ type: "text", text: postText.slice(cursor) });
  for (const blank of unplaced) segments.push({ type: "blank", blank });
  return segments;
}

const PILL_STYLES = {
  correct: { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200", glyph: "✓", label: "correct" },
  incorrect: { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200", glyph: "✗", label: "incorrect" },
  skipped: { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300", glyph: "–", label: "excluded" },
} as const;

function resultPillStatus(r: GrammarResultState): "correct" | "incorrect" | "skipped" {
  if (r.isSkipped) return "skipped";
  return r.correct ? "correct" : "incorrect";
}

function toResult(
  postIndex: number,
  answer: string,
  r: { noteId: bigint; senseId: string; correct: boolean; correctAnswer: string; incorrect: string; reason: string; category: string; nextReviewDate: string; learnedAt: string },
): GrammarResultState {
  return {
    postIndex,
    noteId: r.noteId,
    senseId: r.senseId,
    incorrect: r.incorrect,
    answer,
    correct: r.correct,
    correctAnswer: r.correctAnswer,
    reason: r.reason,
    category: r.category,
    nextReviewDate: r.nextReviewDate,
    learnedAt: r.learnedAt,
  };
}

export default function GrammarQuizPage() {
  const router = useRouter();
  const posts = useGrammarStore((s) => s.posts);
  const currentPostIndex = useGrammarStore((s) => s.currentPostIndex);
  const inputs = useGrammarStore((s) => s.inputs);
  const results = useGrammarStore((s) => s.results);
  const gradingKeys = useGrammarStore((s) => s.gradingKeys);
  const reviewedKeys = useGrammarStore((s) => s.reviewedKeys);
  const selectedKey = useGrammarStore((s) => s.selectedKey);
  const setInput = useGrammarStore((s) => s.setInput);
  const addGrading = useGrammarStore((s) => s.addGrading);
  const removeGrading = useGrammarStore((s) => s.removeGrading);
  const recordResults = useGrammarStore((s) => s.recordResults);
  const selectBlank = useGrammarStore((s) => s.selectBlank);
  const markReviewed = useGrammarStore((s) => s.markReviewed);
  const nextPost = useGrammarStore((s) => s.nextPost);

  const actions = useGrammarResultActions();

  const [error, setError] = useState<string | null>(null);
  const inputRefs = useRef<Map<string, HTMLInputElement>>(new Map());
  const pillRefs = useRef<Map<string, HTMLElement>>(new Map());
  const focusTimeRef = useRef<Map<string, number>>(new Map());

  const post: GrammarPostCard | undefined = posts[currentPostIndex];
  const isLastPost = currentPostIndex + 1 >= posts.length;

  useEffect(() => {
    if (posts.length === 0) router.replace("/quiz?tab=grammar");
  }, [posts.length, router]);

  const segments = useMemo(
    () => (post ? segmentPost(post.postText, post.blanks) : []),
    [post],
  );

  const orderedKeys = useMemo(
    () =>
      segments
        .filter((s): s is { type: "blank"; blank: GrammarBlank } => s.type === "blank")
        .map((s) => s.blank.noteId.toString()),
    [segments],
  );

  const postResults = useMemo(() => {
    const map = new Map<string, { result: GrammarResultState; globalIndex: number }>();
    results.forEach((r, globalIndex) => {
      if (r.postIndex === currentPostIndex) map.set(r.noteId.toString(), { result: r, globalIndex });
    });
    return map;
  }, [results, currentPostIndex]);

  // Autofocus the first box when a post opens.
  useEffect(() => {
    if (orderedKeys.length > 0) {
      const first = orderedKeys[0];
      setTimeout(() => inputRefs.current.get(first)?.focus(), 50);
    }
  }, [orderedKeys]);

  // Keep the selected pill in view alongside the detail sheet.
  useEffect(() => {
    if (selectedKey) {
      pillRefs.current.get(selectedKey)?.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  }, [selectedKey]);

  if (posts.length === 0 || !post) {
    return (
      <Box maxW="sm" mx="auto" p={4} textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  const isDone = (key: string) => postResults.has(key) || gradingKeys.includes(key);
  const nextUnansweredAfter = (afterIndex: number): string | null => {
    for (let i = afterIndex + 1; i < orderedKeys.length; i++) {
      if (!isDone(orderedKeys[i])) return orderedKeys[i];
    }
    for (let i = 0; i <= afterIndex; i++) {
      if (!isDone(orderedKeys[i])) return orderedKeys[i];
    }
    return null;
  };

  const progress = ((currentPostIndex + 1) / posts.length) * 100;
  const totalBlanks = orderedKeys.length;
  const gradedCount = orderedKeys.filter((k) => postResults.has(k)).length;
  const gradingCount = gradingKeys.length;
  const remainingCount = totalBlanks - gradedCount - gradingCount;
  const correctCount = orderedKeys.filter((k) => postResults.get(k)?.result.correct).length;
  const unreviewedWrong = orderedKeys.filter((k) => {
    const r = postResults.get(k)?.result;
    return r && !r.correct && !r.isSkipped && !reviewedKeys.includes(k);
  }).length;
  const selected = selectedKey ? postResults.get(selectedKey) : undefined;
  const gradedKeysInOrder = orderedKeys.filter((k) => postResults.has(k));
  const selectedGradedPos = selectedKey ? gradedKeysInOrder.indexOf(selectedKey) : -1;

  // Grade one blank the moment its box is committed (Enter / blur), and jump the
  // cursor to the next unanswered box. Deterministic answers come back instantly
  // from the server; only answers that differ from the reference take a beat.
  const commitBlank = (blank: GrammarBlank, indexInOrder: number) => {
    const key = blank.noteId.toString();
    const value = (inputs[key] ?? "").trim();
    if (!value || isDone(key)) return;

    const nextKey = nextUnansweredAfter(indexInOrder);
    if (nextKey) setTimeout(() => inputRefs.current.get(nextKey)?.focus(), 0);

    const startedAt = focusTimeRef.current.get(key);
    const responseTimeMs = startedAt !== undefined ? responseTimeSince(startedAt) : BigInt(0);
    addGrading(key);
    void (async () => {
      for (let attempt = 0; attempt < 2; attempt++) {
        try {
          const res = await quizClient.submitGrammarPost({
            answers: [{ noteId: blank.noteId, answer: value, responseTimeMs, isSkipped: false }],
          });
          recordResults(res.results.map((r) => toResult(currentPostIndex, value, r)));
          setError(null);
          break;
        } catch {
          if (attempt === 1) setError("A correction couldn't be graded. Re-type it to retry.");
        }
      }
      removeGrading(key);
    })();
  };

  const focusNextUnanswered = () => {
    const key = nextUnansweredAfter(-1);
    if (key) inputRefs.current.get(key)?.focus();
  };

  // "Next post" is always available: grade any still-empty blanks as skipped
  // (instant, no model call) so their spaced-repetition schedule advances, then
  // move on.
  const handleNextPost = async () => {
    const remaining = post.blanks.filter((b) => {
      const k = b.noteId.toString();
      return !postResults.has(k) && !gradingKeys.includes(k);
    });
    if (remaining.length > 0) {
      try {
        const res = await quizClient.submitGrammarPost({
          answers: remaining.map((b) => ({ noteId: b.noteId, answer: "", responseTimeMs: BigInt(0), isSkipped: true })),
        });
        recordResults(res.results.map((r) => toResult(currentPostIndex, "", r)));
      } catch {
        /* saved server-side on retry next time; don't block navigation */
      }
    }
    if (isLastPost) router.push("/quiz/grammar/complete");
    else nextPost();
  };

  return (
    <Box maxW="sm" mx="auto" p={4} pb={selected ? 80 : 4} minH="100vh">
      <Box mb={2}>
        <Link href="/quiz?tab=grammar">
          <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="xs">
            &lt; Quiz
          </Text>
        </Link>
      </Box>

      <Box mb={4}>
        <Box display="flex" justifyContent="space-between" mb={1}>
          <Text fontSize="sm">
            Post {currentPostIndex + 1} / {posts.length}
          </Text>
          <Text fontSize="sm" aria-live="polite">
            {correctCount} / {totalBlanks} correct
            {gradingCount > 0 ? " · grading…" : ""}
          </Text>
        </Box>
        <Progress.Root value={progress} size="sm">
          <Progress.Track>
            <Progress.Range />
          </Progress.Track>
        </Progress.Root>
      </Box>

      {post.title && (
        <Heading size="sm" mb={3}>
          {post.title}
        </Heading>
      )}

      <Text fontSize="xs" color="fg.muted" mb={2}>
        Type each fix and press Enter. Tap a graded word for details.
      </Text>

      {/* The full post, mistakes fixed in place. */}
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
      >
        {(() => {
          return segments.map((seg, i) => {
            if (seg.type === "text") return <span key={i}>{seg.text}</span>;
            const key = seg.blank.noteId.toString();
            const indexInOrder = orderedKeys.indexOf(key);
            const graded = postResults.get(key)?.result;
            const grading = gradingKeys.includes(key);

            // Graded → tappable pill.
            if (graded) {
              const c = PILL_STYLES[resultPillStatus(graded)];
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
                  aria-label={`${seg.blank.incorrect || "correction"} — ${c.label}`}
                  onClick={() => {
                    selectBlank(key);
                    markReviewed(key);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      selectBlank(key);
                      markReviewed(key);
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
                  <Text as="span" aria-hidden="true">{c.glyph} </Text>
                  {seg.blank.incorrect}
                </Text>
              );
            }

            // Grading in flight → the wrong span with a small spinner in place.
            if (grading) {
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

            // Not yet graded → the wrong span stays visible with a textbox.
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
          });
        })()}
      </Box>

      <Button colorPalette="blue" w="full" size="lg" onClick={handleNextPost}>
        {isLastPost ? "See results" : "Next post"}
      </Button>
      <Box mt={1} textAlign="center" minH="1.25rem">
        {remainingCount > 0 ? (
          <Text
            as="button"
            fontSize="xs"
            color="blue.600"
            _dark={{ color: "blue.300" }}
            onClick={focusNextUnanswered}
          >
            ↓ Next unanswered ({remainingCount})
          </Text>
        ) : unreviewedWrong > 0 ? (
          <Text fontSize="xs" color="orange.600" _dark={{ color: "orange.300" }}>
            {unreviewedWrong} wrong {unreviewedWrong === 1 ? "correction" : "corrections"} not reviewed yet.
          </Text>
        ) : null}
      </Box>
      {error && (
        <Text fontSize="xs" color="red.500" mt={1} textAlign="center" aria-live="assertive">
          {error}
        </Text>
      )}

      {/* Detail sheet: opens when a graded word is tapped. */}
      {selected && (
        <Box
          position="fixed"
          bottom={0}
          left={0}
          right={0}
          maxW="sm"
          mx="auto"
          maxH="60vh"
          overflowY="auto"
          bg="white"
          _dark={{ bg: "gray.900", borderTopColor: "gray.700" }}
          borderTopWidth="1px"
          borderTopColor="gray.200"
          borderTopRadius="xl"
          p={3}
          boxShadow="0 -4px 12px rgba(0,0,0,0.08)"
        >
          <Box display="flex" alignItems="center" justifyContent="space-between" mb={2}>
            <Box display="flex" alignItems="center" gap={1}>
              <Button
                size="xs"
                variant="ghost"
                disabled={selectedGradedPos <= 0}
                onClick={() => selectBlank(gradedKeysInOrder[selectedGradedPos - 1])}
                aria-label="Previous graded word"
              >
                ‹
              </Button>
              <Button
                size="xs"
                variant="ghost"
                disabled={selectedGradedPos < 0 || selectedGradedPos >= gradedKeysInOrder.length - 1}
                onClick={() => selectBlank(gradedKeysInOrder[selectedGradedPos + 1])}
                aria-label="Next graded word"
              >
                ›
              </Button>
            </Box>
            <Text fontSize="xs" color="fg.muted">
              {selectedGradedPos + 1} / {gradedKeysInOrder.length}
              {selected.result.category ? ` · ${selected.result.category}` : ""}
            </Text>
            <Button
              size="xs"
              variant="ghost"
              onClick={() => selectBlank(null)}
              aria-label="Close details"
            >
              ✕
            </Button>
          </Box>
          <QuizResultCard
            item={grammarResultToItem(selected.result, selected.globalIndex)}
            isEtymology={false}
            onOverride={actions.handleOverride}
            onUndo={actions.handleUndo}
            onSkip={actions.handleSkip}
            onResume={actions.handleResume}
          />
        </Box>
      )}
    </Box>
  );
}
