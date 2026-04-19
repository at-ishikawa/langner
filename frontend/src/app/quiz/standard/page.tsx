"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Progress,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { AnswerInput } from "@/components/AnswerInput";
import { BatchFeedback } from "@/components/BatchFeedback";
import { standardResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

type QuizPhase = "answering" | "batch-feedback";

function highlightExpression(
  text: string,
  expression: string,
): React.ReactNode[] {
  const escaped = expression.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const parts = text.split(new RegExp(`(${escaped})`, "gi"));
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <Text as="span" key={i} fontWeight="bold" color="blue.600" _dark={{ color: "blue.300" }}>
        {part}
      </Text>
    ) : (
      <React.Fragment key={i}>{part}</React.Fragment>
    ),
  );
}

export default function QuizCardPage() {
  const router = useRouter();
  const flashcards = useQuizStore((s) => s.flashcards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const results = useQuizStore((s) => s.results);
  const feedbackInterval = useQuizStore((s) => s.feedbackInterval);
  const storeSubmitResult = useQuizStore((s) => s.submitResult);
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
    if (flashcards.length === 0 || quizType !== "standard") {
      router.push("/");
    }
  }, [flashcards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setError(null);
    setTimeout(() => inputRef.current?.focus(), 50);
  }, [currentIndex]);

  const total = flashcards.length;
  const progress = total > 0 ? ((currentIndex + 1) / total) * 100 : 0;

  const batchStart = useMemo(
    () => Math.floor(currentIndex / feedbackInterval) * feedbackInterval,
    [currentIndex, feedbackInterval],
  );

  const batchItems = useMemo(
    () => results.slice(batchStart).map((r, i) => standardResultToItem(r, batchStart + i)),
    [results, batchStart],
  );

  if (flashcards.length === 0) {
    return null;
  }

  const card = flashcards[currentIndex];
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
      const res = await quizClient.submitAnswer({
        noteId: card.noteId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      storeSubmitResult({
        noteId: card.noteId,
        entry: card.entry,
        answer: userAnswer,
        correct: res.correct,
        meaning: res.meaning,
        reason: res.reason,
        contexts: card.examples.map((ex) => ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`),
        wordDetail: res.wordDetail,
        learnedAt: res.learnedAt || undefined,
        images: res.images.length > 0 ? res.images : undefined,
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
      const res = await quizClient.submitAnswer({
        noteId: card.noteId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });

      storeSubmitResult({
        noteId: card.noteId,
        entry: card.entry,
        answer: "(skipped)",
        correct: false,
        meaning: res.meaning,
        reason: res.reason,
        contexts: card.examples.map((ex) => ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`),
        wordDetail: res.wordDetail,
        learnedAt: res.learnedAt || undefined,
        images: res.images.length > 0 ? res.images : undefined,
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
    <Box p={4} maxW="md" mx="auto">
      <Box mb={4}>
        <Text fontSize="sm" mb={1}>
          {currentIndex + 1} / {total}
        </Text>
        <Progress.Root value={progress} size="sm">
          <Progress.Track>
            <Progress.Range />
          </Progress.Track>
        </Progress.Root>
      </Box>

      {phase === "answering" ? (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center">
            {card.entry}
          </Heading>

          {card.examples.length > 0 && (
            <VStack align="stretch" gap={2}>
              {card.examples.map((ex, i) => (
                <Text key={i} fontSize="md" color="fg.muted">
                  {ex.speaker && <>{ex.speaker}: &ldquo;</>}
                  {!ex.speaker && <>&ldquo;</>}
                  {highlightExpression(ex.text, card.originalEntry || card.entry)}
                  &rdquo;
                </Text>
              ))}
            </VStack>
          )}

          <AnswerInput
            ref={inputRef}
            label="Meaning"
            value={answer}
            onChange={setAnswer}
            onKeyDown={handleKeyDown}
            onSubmit={handleSubmit}
            onSkip={handleSkip}
            placeholder="Type your answer"
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
          isEtymology={false}
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
