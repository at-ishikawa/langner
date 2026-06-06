"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Progress, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore, type EtymologyOriginCard } from "@/store/quizStore";
import { AnswerInput } from "@/components/AnswerInput";
import { BatchFeedback } from "@/components/BatchFeedback";
import { RelationGraph } from "@/components/RelationGraph";
import { etymologyResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";
import { responseTimeSince } from "@/lib/responseTime";

type QuizPhase = "answering" | "grading" | "batch-feedback";

interface BufferedAnswer {
  card: EtymologyOriginCard;
  answer: string;
  displayAnswer: string;
  responseTimeMs: bigint;
  isSkipped?: boolean;
}

export default function EtymologyReversePage() {
  const router = useRouter();
  const etymologyOriginCards = useQuizStore((s) => s.etymologyOriginCards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const etymologyResults = useQuizStore((s) => s.etymologyOriginResults);
  const feedbackInterval = useQuizStore((s) => s.feedbackInterval);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyOriginResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pendingRetry, setPendingRetry] = useState<BufferedAnswer[] | null>(null);
  const bufferRef = useRef<BufferedAnswer[]>([]);
  const startTimeRef = useRef<number>(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const { handleOverride, handleUndo, handleSkip: handleItemSkip, handleResume } =
    useQuizResultActions(quizType);

  useEffect(() => {
    if (etymologyOriginCards.length === 0 || quizType !== "etymology-reverse") router.push("/");
  }, [etymologyOriginCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setAnswer("");
    if (phase === "answering") {
      setTimeout(() => inputRef.current?.focus(), 50);
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

  const flushBatch = async (toFlush: BufferedAnswer[]) => {
    setPhase("grading");
    setError(null);
    try {
      const res = await quizClient.batchSubmitEtymologyReverseAnswers({
        answers: toFlush.map((b) => ({
          cardId: b.card.cardId,
          answer: b.answer,
          responseTimeMs: b.responseTimeMs,
          isSkipped: b.isSkipped ?? false,
        })),
      });
      toFlush.forEach((b, i) => {
        const r = res.responses[i];
        storeSubmitResult({
          noteId: r.noteId ? BigInt(r.noteId) : undefined,
          cardId: b.card.cardId,
          origin: b.card.origin,
          // The meaning was the question the user was shown — preserve
          // it on the result card so the feedback view isn't just
          // "origin: metron, meaning: metron" (the bug before this
          // change, where meaning was misset to correctAnswer = origin).
          meaning: b.card.meaning,
          answer: b.displayAnswer,
          correct: r.correct,
          reason: r.reason,
          correctAnswer: r.correctOrigin,
          type: r.type,
          language: r.language,
          learnedAt: r.learnedAt || undefined,
          exampleWords: r.exampleWords,
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
    if (!answer.trim() || phase !== "answering") return;
    const responseTime = responseTimeSince(startTimeRef.current);
    const userAnswer = answer.trim();
    recordAndAdvance({
      card,
      answer: userAnswer,
      displayAnswer: userAnswer,
      responseTimeMs: responseTime,
    });
  };

  const handleSkip = () => {
    if (phase !== "answering") return;
    const responseTime = responseTimeSince(startTimeRef.current);
    recordAndAdvance({
      card,
      answer: "",
      displayAnswer: "(skipped)",
      responseTimeMs: responseTime,
      isSkipped: true,
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

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && phase === "answering") handleSubmit();
  };

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
        />
      ) : phase === "grading" ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>Checking your answers...</Text>
        </Box>
      ) : (
        <VStack align="stretch" gap={4}>
          {card.graphPrompt ? (
            <RelationGraph
              prompt={card.graphPrompt}
              value={answer}
              onValueChange={setAnswer}
            />
          ) : (
            <Box p={4} borderWidth="1px" borderRadius="lg" textAlign="center" bg="white" _dark={{ bg: "gray.800" }}>
              <Heading size="lg" color="blue.700" _dark={{ color: "blue.300" }}>{card.meaning}</Heading>
              <Text fontSize="sm" color="fg.muted" mt={1}>What origin has this meaning?</Text>
            </Box>
          )}
          {!card.graphPrompt && (
            <AnswerInput
              ref={inputRef}
              label="Origin"
              value={answer}
              onChange={setAnswer}
              onKeyDown={handleKeyDown}
              onSubmit={handleSubmit}
              onSkip={handleSkip}
              placeholder="type the origin..."
              stickySubmit
            />
          )}
          {card.graphPrompt && (
            // Graph quiz has its own typed input INSIDE the RelationGraph
            // (the blank node), so we skip AnswerInput's Input but still
            // need the same Submit + Don't Know button row that every
            // other quiz page renders via AnswerInput. Matches that
            // styling exactly (flex layout, sizes, colorPalette) so the
            // graph variant doesn't look like a different app.
            <Box display="flex" gap={2} position="sticky" bottom={4}>
              <Button
                flex="1"
                colorPalette="blue"
                onClick={handleSubmit}
                disabled={!answer.trim()}
                size="lg"
              >
                Submit
              </Button>
              <Button flex="1" variant="outline" onClick={handleSkip} size="lg">
                Don&apos;t Know
              </Button>
            </Box>
          )}
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
