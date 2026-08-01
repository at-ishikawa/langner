"use client";

import { useEffect, useMemo, useRef } from "react";
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

type Phase = "answering" | "review";

// Grading is submitted in small chunks so results stream back and pills fill in
// progressively — the user never waits on the whole post. A few chunks run at
// once; the server grades each chunk (mostly deterministic, LLM only for
// answers that differ from the reference).
const CHUNK_SIZE = 6;
const CHUNK_CONCURRENCY = 3;

function chunk<T>(arr: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < arr.length; i += size) out.push(arr.slice(i, i + size));
  return out;
}

// Run tasks with a bounded number in flight, preserving no particular order.
async function mapWithConcurrency<T>(
  items: T[],
  limit: number,
  fn: (item: T) => Promise<void>,
): Promise<void> {
  let cursor = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (cursor < items.length) {
      const i = cursor++;
      await fn(items[i]);
    }
  });
  await Promise.all(workers);
}

// A post is rendered as an ordered list of plain-text runs and blank tokens so
// each mistake is corrected in place. Blanks are located by their incorrect
// span; duplicates map to successive occurrences in blank order. Any blank whose
// span can't be placed is appended at the end so the rendered set always equals
// the submitted set — submit, review pills, and the score header never diverge.
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

type PillStatus = "pending" | "correct" | "incorrect" | "skipped";

function pillStatus(r: GrammarResultState | undefined): PillStatus {
  if (!r) return "pending";
  if (r.isSkipped) return "skipped";
  return r.correct ? "correct" : "incorrect";
}

const PILL_STYLES: Record<
  PillStatus,
  { bg: string; darkBg: string; color: string; darkColor: string; glyph: string; label: string }
> = {
  pending: { bg: "gray.100", darkBg: "gray.700", color: "gray.500", darkColor: "gray.400", glyph: "…", label: "grading" },
  correct: { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200", glyph: "✓", label: "correct" },
  incorrect: { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200", glyph: "✗", label: "incorrect" },
  skipped: { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300", glyph: "–", label: "excluded" },
};

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
  const markPostSubmitted = useGrammarStore((s) => s.markPostSubmitted);
  const recordPostResults = useGrammarStore((s) => s.recordPostResults);
  const selectBlank = useGrammarStore((s) => s.selectBlank);
  const markReviewed = useGrammarStore((s) => s.markReviewed);
  const nextPost = useGrammarStore((s) => s.nextPost);

  const actions = useGrammarResultActions();

  const errorRef = useRef<HTMLParagraphElement>(null);
  const startTimeRef = useRef<number>(0);
  const pillRefs = useRef<Map<string, HTMLElement>>(new Map());

  const post: GrammarPostCard | undefined = posts[currentPostIndex];
  const isLastPost = currentPostIndex + 1 >= posts.length;
  const phase: Phase = submittedPostIndices.includes(currentPostIndex) ? "review" : "answering";

  // Direct navigation without a seeded session → back to the hub.
  useEffect(() => {
    if (posts.length === 0) router.replace("/quiz?tab=grammar");
  }, [posts.length, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
  }, [currentPostIndex]);

  // Whatever blank is showing in the sheet counts as reviewed.
  useEffect(() => {
    if (phase === "review" && selectedKey) markReviewed(selectedKey);
  }, [phase, selectedKey, markReviewed]);

  // Keep the selected pill in view so the highlighted word and the sheet are
  // visible together, even in a long post.
  useEffect(() => {
    if (phase === "review" && selectedKey) {
      pillRefs.current.get(selectedKey)?.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  }, [phase, selectedKey]);

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

  const gradedCount = orderedKeys.filter((k) => postResults.has(k)).length;
  const pendingCount = orderedKeys.length - gradedCount;
  const correctCount = orderedKeys.filter((k) => postResults.get(k)?.result.correct).length;
  const unreviewedWrong = orderedKeys.filter((k) => {
    const r = postResults.get(k)?.result;
    return r && !r.correct && !r.isSkipped && !reviewedKeys.includes(k);
  }).length;
  const selected = selectedKey ? postResults.get(selectedKey) : undefined;
  const selectedPos = selectedKey ? orderedKeys.indexOf(selectedKey) : -1;

  const handleSubmit = () => {
    if (phase !== "answering") return;
    markPostSubmitted(currentPostIndex); // → review immediately, pills pending
    const responseTimeMs = responseTimeSince(startTimeRef.current);
    const answers = post.blanks.map((b) => {
      const answer = (inputs[b.noteId.toString()] ?? "").trim();
      return { noteId: b.noteId, answer, responseTimeMs, isSkipped: answer === "" };
    });
    const answerByNote = new Map(answers.map((a) => [a.noteId.toString(), a.answer]));

    void mapWithConcurrency(chunk(answers, CHUNK_SIZE), CHUNK_CONCURRENCY, async (group) => {
      for (let attempt = 0; attempt < 2; attempt++) {
        try {
          const res = await quizClient.submitGrammarPost({ answers: group });
          recordPostResults(
            res.results.map((r) => ({
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
            })),
          );
          return;
        } catch {
          if (attempt === 1 && errorRef.current) {
            errorRef.current.textContent = "Some corrections couldn't be graded. Try again.";
          }
        }
      }
    });
  };

  const handleNextPost = () => {
    if (isLastPost) router.push("/quiz/grammar/complete");
    else nextPost();
  };

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
              {pendingCount > 0 ? ` · grading ${pendingCount}…` : ""}
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
            const isPending = status === "pending";
            return (
              <Text
                as="span"
                key={i}
                ref={(el: HTMLElement | null) => {
                  if (el) pillRefs.current.set(key, el);
                  else pillRefs.current.delete(key);
                }}
                role={isPending ? undefined : "button"}
                tabIndex={isPending ? undefined : 0}
                aria-pressed={isPending ? undefined : isSel}
                aria-label={`${seg.blank.incorrect || "correction"} — ${c.label}`}
                onClick={isPending ? undefined : () => selectBlank(key)}
                onKeyDown={
                  isPending
                    ? undefined
                    : (e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          selectBlank(key);
                        }
                      }
                }
                cursor={isPending ? "default" : "pointer"}
                opacity={isPending ? 0.7 : 1}
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

          // Answering: wrong span stays visible with a textbox beside it.
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
                value={inputs[key] ?? ""}
                onChange={(e) => setInput(key, e.target.value)}
              />
            </Text>
          );
        })}
      </Box>

      {phase === "answering" ? (
        <>
          <Button colorPalette="blue" w="full" size="lg" onClick={handleSubmit}>
            Check corrections
          </Button>
          {emptyCount > 0 && (
            <Text fontSize="xs" color="fg.muted" mt={1} textAlign="center">
              {emptyCount} left blank will be counted as skipped.
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
          <Text ref={errorRef} fontSize="xs" color="red.500" mt={1} textAlign="center" aria-live="assertive" />
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
