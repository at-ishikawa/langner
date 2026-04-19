"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Progress, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { AnswerInput } from "@/components/AnswerInput";
import { BatchFeedback } from "@/components/BatchFeedback";
import { etymologyResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

type QuizPhase = "answering" | "batch-feedback";

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
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  const { handleOverride, handleUndo, handleSkip: handleItemSkip, handleResume } =
    useQuizResultActions(quizType);

  useEffect(() => {
    if (etymologyOriginCards.length === 0 || quizType !== "etymology-reverse") router.push("/");
  }, [etymologyOriginCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setError(null);
    setTimeout(() => inputRef.current?.focus(), 50);
  }, [currentIndex]);

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

  const afterSubmit = () => {
    const isBatchBoundary = (currentIndex + 1) % feedbackInterval === 0;
    if (isFinalCard || isBatchBoundary) {
      setPhase("batch-feedback");
    } else {
      nextCard();
    }
  };

  const handleSubmit = async () => {
    if (loading || !answer.trim()) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    setLoading(true);
    setError(null);

    try {
      const res = await quizClient.submitEtymologyReverseAnswer({
        cardId: card.cardId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });
      storeSubmitResult({
        noteId: res.noteId ? BigInt(res.noteId) : undefined,
        cardId: card.cardId,
        origin: card.origin,
        answer: userAnswer,
        correct: res.correct,
        reason: res.reason,
        correctAnswer: res.correctOrigin,
        type: res.type,
        language: res.language,
        learnedAt: res.learnedAt || undefined,
      });
      setAnswer("");
      afterSubmit();
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleSkip = async () => {
    if (loading) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    setLoading(true);
    setError(null);

    try {
      const res = await quizClient.submitEtymologyReverseAnswer({
        cardId: card.cardId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });
      storeSubmitResult({
        noteId: res.noteId ? BigInt(res.noteId) : undefined,
        cardId: card.cardId,
        origin: card.origin,
        answer: "(skipped)",
        correct: false,
        reason: res.reason,
        correctAnswer: res.correctOrigin,
        type: res.type,
        language: res.language,
        learnedAt: res.learnedAt || undefined,
      });
      setAnswer("");
      afterSubmit();
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleContinue = () => {
    if (isFinalCard) {
      router.push("/quiz/complete");
    } else {
      nextCard();
    }
  };

  const handleSeeResults = () => router.push("/quiz/complete");

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && phase === "answering" && !loading) {
      handleSubmit();
    }
  };

  return (
    <Box p={4} maxW="sm" mx="auto" onKeyDown={handleKeyDown}>
      <Box mb={4}>
        <Text fontSize="sm" mb={1}>{currentIndex + 1} / {total}</Text>
        <Progress.Root value={progress} size="sm"><Progress.Track><Progress.Range /></Progress.Track></Progress.Root>
      </Box>
      {phase === "answering" ? (
        <VStack align="stretch" gap={4}>
          <Box p={4} borderWidth="1px" borderRadius="lg" textAlign="center" bg="white" _dark={{ bg: "gray.800" }}>
            <Heading size="lg" color="blue.700" _dark={{ color: "blue.300" }}>{card.meaning}</Heading>
            <Text fontSize="sm" color="fg.muted" mt={1}>What origin has this meaning?</Text>
          </Box>
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

          {loading && (
            <Box textAlign="center" py={2}>
              <Spinner size="sm" mr={2} />
              <Text as="span">Submitting...</Text>
            </Box>
          )}

          {error && (
            <VStack align="stretch" gap={2}>
              <Text color="red.500">{error}</Text>
              <Button
                w="full"
                colorPalette="blue"
                variant="outline"
                onClick={() => {
                  setError(null);
                  setTimeout(() => inputRef.current?.focus(), 50);
                }}
              >
                Retry
              </Button>
            </VStack>
          )}
        </VStack>
      ) : (
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
      )}
    </Box>
  );
}
