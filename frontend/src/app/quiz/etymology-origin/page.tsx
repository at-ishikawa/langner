"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Input, Progress, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore, type EtymologyOriginCard } from "@/store/quizStore";
import { BatchFeedback } from "@/components/BatchFeedback";
import { etymologyResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";
import { responseTimeSince } from "@/lib/responseTime";

type QuizPhase = "answering" | "grading" | "batch-feedback";

interface BufferedAnswer {
  card: EtymologyOriginCard;
  // answers keyed by the family word's word_id (as a string) → typed meaning.
  answers: Record<string, string>;
  // skipped keyed by the family word's word_id → true when the learner
  // tapped "Don't Know" for THAT word specifically. Per-word, not
  // all-or-nothing: sibling words with a typed answer are graded normally
  // regardless of which words are skipped.
  skipped: Record<string, boolean>;
  responseTimeMs: bigint;
}

export default function EtymologyOriginPage() {
  const router = useRouter();
  const etymologyOriginCards = useQuizStore((s) => s.etymologyOriginCards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const etymologyResults = useQuizStore((s) => s.etymologyOriginResults);
  const feedbackInterval = useQuizStore((s) => s.feedbackInterval);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyOriginResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  // inputs maps a family word's word_id (string) → the typed meaning.
  const [inputs, setInputs] = useState<Record<string, string>>({});
  // skippedWords maps a family word's word_id (string) → whether the learner
  // tapped "Don't Know" for that word. Independent per word.
  const [skippedWords, setSkippedWords] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  const [pendingRetry, setPendingRetry] = useState<BufferedAnswer[] | null>(null);
  const bufferRef = useRef<BufferedAnswer[]>([]);
  const startTimeRef = useRef<number>(0);
  const firstInputRef = useRef<HTMLInputElement>(null);

  const {
    handleOverride, handleUndo, handleSkip: handleItemSkip, handleResume,
    handleOverrideWord, handleExcludeWord,
  } = useQuizResultActions(quizType);

  useEffect(() => {
    if (etymologyOriginCards.length === 0 || quizType !== "etymology-origin") router.push("/");
  }, [etymologyOriginCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setInputs({});
    setSkippedWords({});
    if (phase === "answering") {
      setTimeout(() => firstInputRef.current?.focus(), 50);
    }
  }, [currentIndex, phase]);

  const total = etymologyOriginCards.length;
  const progress = total > 0 ? ((currentIndex + 1) / total) * 100 : 0;

  const batchStart = useMemo(
    () => Math.floor(currentIndex / feedbackInterval) * feedbackInterval,
    [currentIndex, feedbackInterval],
  );

  const batchItems = useMemo(
    () => etymologyResults.slice(batchStart).map((r, i) => etymologyResultToItem(r, batchStart + i)),
    [etymologyResults, batchStart],
  );

  if (etymologyOriginCards.length === 0) return null;

  const card = etymologyOriginCards[currentIndex];
  const isFinalCard = currentIndex + 1 >= total;
  const principalParts = card.forms.map((f) => f.form).filter(Boolean).join(", ");
  // A word is "ready" once it's either answered or explicitly marked "Don't
  // Know" — the two are independent per word, so answering some words and
  // skipping others is a normal, expected combination.
  const allFilled = card.words.length > 0 && card.words.every((w) => {
    const key = w.wordId.toString();
    return skippedWords[key] || (inputs[key] ?? "").trim();
  });

  const flushBatch = async (toFlush: BufferedAnswer[]) => {
    setPhase("grading");
    setError(null);
    try {
      const res = await quizClient.batchSubmitEtymologyOriginAnswers({
        answers: toFlush.map((b) => ({
          cardId: b.card.cardId,
          answers: b.card.words.map((w) => {
            const key = w.wordId.toString();
            return {
              wordId: w.wordId,
              answer: b.answers[key] ?? "",
              skipped: b.skipped[key] ?? false,
            };
          }),
          responseTimeMs: b.responseTimeMs,
        })),
      });
      toFlush.forEach((b, i) => {
        const r = res.responses[i];
        storeSubmitResult({
          noteId: r.noteId ? BigInt(r.noteId) : undefined,
          cardId: b.card.cardId,
          origin: r.origin || b.card.origin,
          meaning: r.meaning || b.card.meaning,
          type: b.card.type,
          language: b.card.language,
          forms: b.card.forms,
          correct: r.correct,
          words: r.results.map((wr) => ({
            wordId: wr.wordId,
            expression: wr.expression,
            correct: wr.correct,
            correctMeaning: wr.correctMeaning,
            reason: wr.reason,
            userAnswer: b.answers[wr.wordId.toString()] ?? "",
            skipped: wr.skipped,
          })),
          notebookName: b.card.notebookName,
          nextReviewDate: r.nextReviewDate || undefined,
          learnedAt: r.learnedAt || undefined,
          senseId: r.senseId || undefined,
          // Note: isSkipped is intentionally NOT set here. It means "excluded
          // from future quizzes" (the origin-level SkipWord/Exclude action) —
          // a per-word "Don't Know" during answering must never be confused
          // with that (bug: doing so made the origin display as Excluded on
          // the feedback screen even though the user never tapped Exclude).
        });
      });
      bufferRef.current = [];
      setPendingRetry(null);
      setPhase("batch-feedback");
    } catch {
      setError("Failed to submit answers");
      setPendingRetry(toFlush);
      setPhase("answering");
    }
  };

  const recordAndAdvance = (entry: BufferedAnswer) => {
    bufferRef.current = [...bufferRef.current, entry];
    const isBatchBoundary = (currentIndex + 1) % feedbackInterval === 0;
    if (isFinalCard || isBatchBoundary) {
      void flushBatch(bufferRef.current);
    } else {
      nextCard();
    }
  };

  const handleSubmit = () => {
    if (!allFilled || phase !== "answering") return;
    const responseTime = responseTimeSince(startTimeRef.current);
    recordAndAdvance({
      card,
      answers: { ...inputs },
      skipped: { ...skippedWords },
      responseTimeMs: responseTime,
    });
  };

  // handleSkip is the whole-card "Don't Know" shortcut for whatever the user
  // hasn't answered yet: it marks only the words that are still blank (and
  // any word already toggled "Skip") as skipped, and submits immediately
  // without requiring every word to be filled first. Words the user already
  // typed an answer for are submitted as-is and graded normally — tapping
  // this button must never discard or skip an answer the user already typed
  // (regression: it used to blow away every typed answer with answers: {}
  // and mark all words skipped, so 2 typed + 2 blank + "Don't Know" reported
  // all 4 words as skipped instead of grading the 2 typed ones).
  const handleSkip = () => {
    if (phase !== "answering") return;
    const responseTime = responseTimeSince(startTimeRef.current);
    const skippedForSubmit = Object.fromEntries(
      card.words.map((w) => {
        const key = w.wordId.toString();
        const isBlank = !(inputs[key] ?? "").trim();
        return [key, (skippedWords[key] ?? false) || isBlank];
      }),
    );
    recordAndAdvance({
      card,
      answers: { ...inputs },
      skipped: skippedForSubmit,
      responseTimeMs: responseTime,
    });
  };

  const handleRetry = () => {
    if (pendingRetry) void flushBatch(pendingRetry);
  };

  const handleContinue = () => {
    if (isFinalCard) router.push("/quiz/complete");
    else {
      setPhase("answering");
      nextCard();
    }
  };

  const handleSeeResults = () => router.push("/quiz/complete");

  return (
    <Box p={4} maxW="sm" mx="auto">
      <Box mb={4}>
        <Text fontSize="sm" mb={1}>{currentIndex + 1} / {total}</Text>
        <Progress.Root value={progress} size="sm"><Progress.Track><Progress.Range /></Progress.Track></Progress.Root>
      </Box>
      {phase === "batch-feedback" ? (
        <BatchFeedback
          items={batchItems}
          isEtymology={true}
          isFinal={isFinalCard}
          onContinue={handleContinue}
          onSeeResults={handleSeeResults}
          onOverride={handleOverride}
          onUndo={handleUndo}
          onSkip={handleItemSkip}
          onResume={handleResume}
          onOverrideWord={handleOverrideWord}
          onExcludeWord={handleExcludeWord}
        />
      ) : phase === "grading" ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>Checking your answers...</Text>
        </Box>
      ) : (
        <VStack align="stretch" gap={4}>
          {/* Origin header: origin text, principal parts, language/type, and
              the origin's own meaning as context. */}
          <Box p={4} borderWidth="1px" borderRadius="lg" textAlign="center" bg="white" _dark={{ bg: "gray.800" }}>
            <Heading size="xl">{card.origin}</Heading>
            {principalParts && (
              <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }} mt={1} fontStyle="italic">
                {principalParts}
              </Text>
            )}
            <Box display="flex" gap={2} justifyContent="center" mt={2} flexWrap="wrap">
              {card.type && <Box px={2} py={0.5} borderRadius="full" bg="blue.100" _dark={{ bg: "blue.900" }}><Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }}>{card.type}</Text></Box>}
              {card.language && <Box px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}><Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{card.language}</Text></Box>}
              {card.sense && <Box px={2} py={0.5} borderRadius="full" bg="purple.100" _dark={{ bg: "purple.900" }}><Text fontSize="xs" color="purple.600" _dark={{ color: "purple.300" }}>{card.sense}</Text></Box>}
            </Box>
            {card.meaning && (
              <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }} mt={3}>
                {card.meaning}
              </Text>
            )}
          </Box>

          {/* One input per derived family word, each with its own "Skip"
              toggle. The whole family is visible as context; the user types
              each word's meaning, or taps Skip for words they don't know —
              independently, so answering some and skipping others in the
              same submission is normal. */}
          <Box>
            <Text fontWeight="medium" mb={2}>Type the meaning of each word</Text>
            <VStack align="stretch" gap={3}>
              {card.words.map((w, i) => {
                const key = w.wordId.toString();
                const skipped = skippedWords[key] ?? false;
                return (
                  <Box key={key}>
                    <Box display="flex" alignItems="center" justifyContent="space-between" mb={1}>
                      <Text fontSize="sm" fontWeight="semibold">{w.expression}</Text>
                      <Button
                        size="xs"
                        variant={skipped ? "solid" : "outline"}
                        colorPalette="gray"
                        aria-label={skipped ? `Answer ${w.expression} instead` : `Skip ${w.expression}`}
                        onClick={() => {
                          setSkippedWords((prev) => ({ ...prev, [key]: !skipped }));
                          if (!skipped) setInputs((prev) => ({ ...prev, [key]: "" }));
                        }}
                      >
                        {skipped ? "Skipped" : "Skip"}
                      </Button>
                    </Box>
                    <Input
                      ref={i === 0 ? firstInputRef : undefined}
                      value={inputs[key] ?? ""}
                      onChange={(e) => {
                        setInputs((prev) => ({ ...prev, [key]: e.target.value }));
                        if (skipped) setSkippedWords((prev) => ({ ...prev, [key]: false }));
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && allFilled) handleSubmit();
                      }}
                      placeholder={skipped ? "marked as don't know" : "type the meaning..."}
                      disabled={skipped}
                      size="lg"
                      aria-label={`Meaning of ${w.expression}`}
                    />
                  </Box>
                );
              })}
            </VStack>
          </Box>

          <Box display="flex" gap={2} position="sticky" bottom={4}>
            <Button flex="1" colorPalette="blue" onClick={handleSubmit} disabled={!allFilled} size="lg">
              Submit
            </Button>
            <Button flex="1" variant="outline" onClick={handleSkip} size="lg">
              Don&apos;t Know
            </Button>
          </Box>

          {error && (
            <VStack align="stretch" gap={2}>
              <Text color="red.500">{error}</Text>
              {pendingRetry && (
                <Button w="full" colorPalette="blue" variant="outline" onClick={handleRetry}>
                  Retry grading
                </Button>
              )}
            </VStack>
          )}
        </VStack>
      )}
    </Box>
  );
}
