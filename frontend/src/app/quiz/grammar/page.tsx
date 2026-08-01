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
// span; duplicates map to successive occurrences in blank order.
type Segment =
  | { type: "text"; text: string }
  | { type: "blank"; blank: GrammarBlank };

function segmentPost(postText: string, blanks: GrammarBlank[]): Segment[] {
  // Assign each blank a position: for repeated spans, advance a per-string
  // cursor so the Nth blank with a span points at that span's Nth occurrence.
  const cursors = new Map<string, number>();
  const placed = blanks
    .map((blank) => {
      const from = cursors.get(blank.incorrect) ?? 0;
      const pos = blank.incorrect ? postText.indexOf(blank.incorrect, from) : -1;
      if (pos >= 0) cursors.set(blank.incorrect, pos + blank.incorrect.length);
      return { blank, pos };
    })
    .filter((p) => p.pos >= 0)
    .sort((a, b) => a.pos - b.pos);

  const segments: Segment[] = [];
  let cursor = 0;
  for (const { blank, pos } of placed) {
    if (pos < cursor) continue; // overlapping span, skip to keep text intact
    if (pos > cursor) segments.push({ type: "text", text: postText.slice(cursor, pos) });
    segments.push({ type: "blank", blank });
    cursor = pos + blank.incorrect.length;
  }
  if (cursor < postText.length) segments.push({ type: "text", text: postText.slice(cursor) });
  return segments;
}

function pillColors(r: GrammarResultState | undefined, selected: boolean) {
  const kind = !r ? "pending" : r.isSkipped ? "skipped" : r.correct ? "correct" : "incorrect";
  const map = {
    pending: { bg: "blue.50", darkBg: "blue.950", color: "blue.700", darkColor: "blue.200" },
    correct: { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200" },
    incorrect: { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200" },
    skipped: { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300" },
  }[kind];
  return { ...map, selected };
}

export default function GrammarQuizPage() {
  const router = useRouter();
  const posts = useGrammarStore((s) => s.posts);
  const currentPostIndex = useGrammarStore((s) => s.currentPostIndex);
  const inputs = useGrammarStore((s) => s.inputs);
  const results = useGrammarStore((s) => s.results);
  const submittedPostIndices = useGrammarStore((s) => s.submittedPostIndices);
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

  const handleSelect = (key: string) => {
    selectBlank(key);
    markReviewed(key);
  };

  const handleNextPost = () => {
    if (isLastPost) {
      router.push("/quiz/grammar/complete");
    } else {
      nextPost();
    }
  };

  const correctCount = orderedKeys.filter((k) => postResults.get(k)?.result.correct).length;
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
        lineHeight="1.9"
        whiteSpace="pre-wrap"
      >
        {segments.map((seg, i) => {
          if (seg.type === "text") return <span key={i}>{seg.text}</span>;
          const key = seg.blank.noteId.toString();

          if (phase === "review") {
            const r = postResults.get(key)?.result;
            const c = pillColors(r, key === selectedKey);
            return (
              <Text
                as="span"
                key={i}
                role="button"
                tabIndex={0}
                onClick={() => handleSelect(key)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") handleSelect(key);
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
                boxShadow={c.selected ? "0 0 0 2px var(--chakra-colors-blue-500)" : undefined}
              >
                {seg.blank.incorrect}
              </Text>
            );
          }

          // Answering: wrong span stays visible with a textbox beside it.
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
              <Input
                size="xs"
                display="inline-block"
                w="auto"
                minW="6rem"
                mx={1}
                verticalAlign="baseline"
                aria-label={`Correction for "${seg.blank.incorrect}"`}
                placeholder="fix"
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
          {error && (
            <Text color="red.500" mt={2} fontSize="sm">
              {error}
            </Text>
          )}
        </>
      ) : (
        <Button colorPalette="blue" w="full" size="lg" onClick={handleNextPost}>
          {isLastPost ? "See results" : "Next post"}
        </Button>
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
              onClick={() => handleSelect(orderedKeys[selectedPos - 1])}
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
              onClick={() => handleSelect(orderedKeys[selectedPos + 1])}
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
