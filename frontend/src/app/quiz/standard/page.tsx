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
import { useQuizStore, type Flashcard } from "@/store/quizStore";
import { AnswerInput } from "@/components/AnswerInput";
import { BatchFeedback } from "@/components/BatchFeedback";
import { standardResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

type QuizPhase = "answering" | "grading" | "batch-feedback";

interface BufferedAnswer {
  card: Flashcard;
  answer: string;
  displayAnswer: string;
  responseTimeMs: bigint;
}

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
  const [error, setError] = useState<string | null>(null);
  const [pendingRetry, setPendingRetry] = useState<BufferedAnswer[] | null>(null);
  const bufferRef = useRef<BufferedAnswer[]>([]);
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
    setAnswer("");
    setError(null);
    if (phase === "answering") {
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [currentIndex, phase]);

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

  const flushBatch = async (toFlush: BufferedAnswer[]) => {
    setPhase("grading");
    setError(null);
    try {
      const res = await quizClient.batchSubmitAnswers({
        answers: toFlush.map((b) => ({
          noteId: b.card.noteId,
          answer: b.answer,
          responseTimeMs: b.responseTimeMs,
        })),
      });
      toFlush.forEach((b, i) => {
        const r = res.responses[i];
        storeSubmitResult({
          noteId: b.card.noteId,
          entry: b.card.entry,
          answer: b.displayAnswer,
          correct: r.correct,
          meaning: r.meaning,
          reason: r.reason,
          contexts: b.card.examples.map((ex) => ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`),
          wordDetail: r.wordDetail,
          learnedAt: r.learnedAt || undefined,
          images: r.images.length > 0 ? r.images : undefined,
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
    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    recordAndAdvance({
      card,
      answer: userAnswer,
      displayAnswer: userAnswer,
      responseTimeMs: BigInt(responseTimeMs),
    });
  };

  const handleSkip = () => {
    if (phase !== "answering") return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    recordAndAdvance({
      card,
      answer: "I don't know",
      displayAnswer: "(skipped)",
      responseTimeMs: BigInt(responseTimeMs),
    });
  };

  const handleRetry = () => {
    if (pendingRetry) {
      void flushBatch(pendingRetry);
    }
  };

  const handleContinue = () => {
    if (isFinalCard) {
      router.push("/quiz/complete");
    } else {
      setPhase("answering");
      nextCard();
    }
  };

  const handleSeeResults = () => router.push("/quiz/complete");

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && phase === "answering") {
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

      {phase === "batch-feedback" ? (
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
      ) : phase === "grading" ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>Checking your answers...</Text>
        </Box>
      ) : (
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
