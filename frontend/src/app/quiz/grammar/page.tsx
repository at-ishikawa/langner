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

type Phase = "answering" | "grading" | "review";

// A post is rendered as an ordered list of plain-text runs and blank tokens so
// each mistake is corrected in place. Blanks are located by their incorrect
// span; duplicates map to successive occurrences in blank order. Any blank whose
// span can't be placed (empty/omission span, or fewer occurrences than blanks)
// is appended at the end so the rendered set always equals the submitted set —
// submit, review pills, and the score header never diverge.
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
      unplaced.push(blank); // overlapping span — fall back to a trailing token
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

function pillStatus(
  r: GrammarResultState | undefined,
): "correct" | "incorrect" | "skipped" {
  if (!r || r.isSkipped) return "skipped";
  return r.correct ? "correct" : "incorrect";
}

const PILL_STYLES = {
  correct: { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200", glyph: "✓", label: "correct" },
  incorrect: { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200", glyph: "✗", label: "incorrect" },
  skipped: { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300", glyph: "–", label: "excluded" },
} as const;

export default function GrammarQuizPage() {
  const router = useRouter();
  const posts = useGrammarStore((s) => s.posts);
  const currentPostIndex = useGrammarStore((s) => s.currentPostIndex);
  const inputs = useGrammarStore((s) => s.inputs);
  const results = useGrammarStore((s) => s.results);
  const submittedPostIndices = useGrammarStore((s) => s.submittedPostIndices);
  const reviewedKeys = useGrammarStore((s) => s.reviewedKeys);
  const selectedKey = useGrammarStore((s) => s.selectedKey);
  const setInput = useGrammarStore((s) => s.setInput);
  const recordPostResults = useGrammarStore((s) => s.recordPostResults);
  const selectBlank = useGrammarStore((s) => s.selectBlank);
  const markReviewed = useGrammarStore((s) => s.markReviewed);
  const nextPost = useGrammarStore((s) => s.nextPost);

  const actions = useGrammarResultActions();

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef<number>(0);
  const pillRefs = useRef<Map<string, HTMLElement>>(new Map());

  const post: GrammarPostCard | undefined = posts[currentPostIndex];
  const isLastPost = currentPostIndex + 1 >= posts.length;

  // Phase is derived from the store so navigating between posts needs no
  // effect-driven setState: a post already graded shows review, otherwise the
  // answering form (with a transient grading spinner while the RPC is inflight).
  const phase: Phase = submitting
    ? "grading"
    : submittedPostIndices.includes(currentPostIndex)
      ? "review"
      : "answering";

  // Direct navigation without a seeded session → back to the hub.
  useEffect(() => {
    if (posts.length === 0) router.replace("/quiz?tab=grammar");
  }, [posts.length, router]);

  // Restart the response timer whenever the post changes (ref write only — no
  // render, no cascading setState).
  useEffect(() => {
    startTimeRef.current = Date.now();
  }, [currentPostIndex]);

  // Whatever blank is showing in the sheet counts as reviewed (covers the
  // auto-selected first-wrong blank, taps, and stepper). markReviewed is
  // idempotent, so this can't loop.
  useEffect(() => {
    if (phase === "review" && selectedKey) markReviewed(selectedKey);
  }, [phase, selectedKey, markReviewed]);

  // Keep the selected pill in view so the highlighted word and the detail sheet
  // are always visible together, even in a long post.
  useEffect(() => {
    if (phase === "review" && selectedKey) {
      pillRefs.current.get(selectedKey)?.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  }, [phase, selectedKey]);

  const segments = useMemo(
    () => (post ? segmentPost(post.postText, post.blanks) : []),
    [post],
  );

  // Blanks in reading order — drives the review pills and the sheet stepper.
  const orderedKeys = useMemo(
    () =>
      segments
        .filter((s): s is { type: "blank"; blank: GrammarBlank } => s.type === "blank")
        .map((s) => s.blank.noteId.toString()),
    [segments],
  );

  // Current post's graded blanks, keyed by noteId, with their global index into
  // the accumulator (needed by the override/skip actions).
  const postResults = useMemo(() => {
    const map = new Map<string, { result: GrammarResultState; globalIndex: number }>();
    results.forEach((r, globalIndex) => {
      if (r.postIndex === currentPostIndex) map.set(r.noteId.toString(), { result: r, globalIndex });
    });
    return map;
  }, [results, currentPostIndex]);

  if (posts.length === 0 || !post) {
    return (
      <Box maxW="sm" mx="auto" p={4} textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  const progress = ((currentPostIndex + 1) / posts.length) * 100;
  const totalBlanks = post.blanks.length;
  const filledCount = post.blanks.filter(
    (b) => (inputs[b.noteId.toString()] ?? "").trim() !== "",
  ).length;
  const emptyCount = totalBlanks - filledCount;

  const handleSubmit = async () => {
    if (phase !== "answering") return;
    setSubmitting(true);
    setError(null);
    const responseTimeMs = responseTimeSince(startTimeRef.current);
    const answers = post.blanks.map((b) => {
      const answer = (inputs[b.noteId.toString()] ?? "").trim();
      return {
        noteId: b.noteId,
        answer,
        responseTimeMs,
        isSkipped: answer === "",
      };
    });
    try {
      const res = await quizClient.submitGrammarPost({ answers });
      const answerByNote = new Map(answers.map((a) => [a.noteId.toString(), a.answer]));
      const graded: GrammarResultState[] = res.results.map((r) => ({
        postIndex: currentPostIndex,
        noteId: r.noteId,
        senseId: r.senseId,
        incorrect: r.incorrect,
        answer: answerByNote.get(r.noteId.toString()) ?? "",
        correct: r.correct,
        correctAnswer: r.correctAnswer,
        reason: r.reason,
        category: r.category,
        nextReviewDate: r.nextReviewDate,
        learnedAt: r.learnedAt,
      }));
      recordPostResults(currentPostIndex, graded);
    } catch {
      setError("Failed to submit your corrections.");
    } finally {
      setSubmitting(false);
    }
  };

  const handleNextPost = () => {
    if (isLastPost) {
      router.push("/quiz/grammar/complete");
    } else {
      nextPost();
    }
  };

  const correctCount = orderedKeys.filter((k) => postResults.get(k)?.result.correct).length;
  const unreviewedWrong = orderedKeys.filter((k) => {
    const r = postResults.get(k)?.result;
    return r && !r.correct && !r.isSkipped && !reviewedKeys.includes(k);
  }).length;
  const selected = selectedKey ? postResults.get(selectedKey) : undefined;
  const selectedPos = selectedKey ? orderedKeys.indexOf(selectedKey) : -1;

  return (
    <Box maxW="sm" mx="auto" p={4} pb={phase === "review" ? 80 : 4} minH="100vh">
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
          {phase === "review" && (
            <Text fontSize="sm" aria-live="polite">
              {correctCount} / {orderedKeys.length} correct
            </Text>
          )}
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

      {phase === "review" ? (
        <Text fontSize="xs" color="fg.muted" mb={2}>
          Tap any highlighted word for details.
        </Text>
      ) : (
        totalBlanks > 0 && (
          <Text fontSize="xs" color="fg.muted" mb={2} aria-live="polite">
            {filledCount} of {totalBlanks} filled
          </Text>
        )
      )}

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
        lineHeight="2"
        whiteSpace="pre-wrap"
        overflowX="hidden"
        wordBreak="break-word"
      >
        {segments.map((seg, i) => {
          if (seg.type === "text") return <span key={i}>{seg.text}</span>;
          const key = seg.blank.noteId.toString();

          if (phase === "review") {
            const r = postResults.get(key)?.result;
            const status = pillStatus(r);
            const c = PILL_STYLES[status];
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
                onClick={() => selectBlank(key)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    selectBlank(key);
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

          // Answering: wrong span stays visible with a textbox beside it. No
          // nowrap — a long span + input wraps naturally instead of overflowing.
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
                size="sm"
                display="inline-block"
                w="auto"
                minW="6rem"
                maxW="100%"
                mx={1}
                verticalAlign="baseline"
                aria-label={`Correction for "${seg.blank.incorrect}"`}
                placeholder="fix"
                disabled={phase === "grading"}
                value={inputs[key] ?? ""}
                onChange={(e) => setInput(key, e.target.value)}
              />
            </Text>
          );
        })}
      </Box>

      {phase === "grading" ? (
        <Box textAlign="center" py={6}>
          <Spinner size="md" mb={2} />
          <Text fontSize="sm">Checking your corrections…</Text>
        </Box>
      ) : phase === "answering" ? (
        <>
          <Button colorPalette="blue" w="full" size="lg" onClick={handleSubmit}>
            Check corrections
          </Button>
          {emptyCount > 0 && (
            <Text fontSize="xs" color="fg.muted" mt={1} textAlign="center">
              {emptyCount} left blank will be counted as skipped.
            </Text>
          )}
          {error && (
            <Text color="red.500" mt={2} fontSize="sm">
              {error}
            </Text>
          )}
        </>
      ) : (
        <>
          <Button colorPalette="blue" w="full" size="lg" onClick={handleNextPost}>
            {isLastPost ? "See results" : "Next post"}
          </Button>
          {unreviewedWrong > 0 && (
            <Text fontSize="xs" color="orange.600" _dark={{ color: "orange.300" }} mt={1} textAlign="center">
              {unreviewedWrong} wrong {unreviewedWrong === 1 ? "correction" : "corrections"} not reviewed yet.
            </Text>
          )}
        </>
      )}

      {/* Review sheet: one blank at a time with details + actions. */}
      {phase === "review" && selected && (
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
            <Button
              size="xs"
              variant="ghost"
              disabled={selectedPos <= 0}
              onClick={() => selectBlank(orderedKeys[selectedPos - 1])}
              aria-label="Previous blank"
            >
              ‹
            </Button>
            <Text fontSize="xs" color="fg.muted">
              {selectedPos + 1} / {orderedKeys.length}
              {selected.result.category ? ` · ${selected.result.category}` : ""}
            </Text>
            <Button
              size="xs"
              variant="ghost"
              disabled={selectedPos < 0 || selectedPos >= orderedKeys.length - 1}
              onClick={() => selectBlank(orderedKeys[selectedPos + 1])}
              aria-label="Next blank"
            >
              ›
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
