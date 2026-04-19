"use client";

import { useEffect, useMemo, useRef, useState } from "react";
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
import { reverseResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";

type QuizPhase = "answering" | "synonym-retry" | "batch-feedback";

export default function ReverseQuizPage() {
  const router = useRouter();
  const reverseFlashcards = useQuizStore((s) => s.reverseFlashcards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const feedbackInterval = useQuizStore((s) => s.feedbackInterval);
  const storeSubmitResult = useQuizStore((s) => s.submitReverseResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [synonymAnswer, setSynonymAnswer] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  const { handleOverride, handleUndo, handleSkip: handleItemSkip, handleResume } =
    useQuizResultActions(quizType);

  useEffect(() => {
    if (reverseFlashcards.length === 0 || quizType !== "reverse") {
      router.push("/");
    }
  }, [reverseFlashcards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setSynonymAnswer("");
    setError(null);
    setTimeout(() => inputRef.current?.focus(), 100);
  }, [currentIndex]);

  const total = reverseFlashcards.length;
  const progress = total > 0 ? ((currentIndex + 1) / total) * 100 : 0;

  const batchStart = useMemo(
    () => Math.floor(currentIndex / feedbackInterval) * feedbackInterval,
    [currentIndex, feedbackInterval],
  );

  const batchItems = useMemo(
    () => reverseResults.slice(batchStart).map((r, i) => reverseResultToItem(r, batchStart + i)),
    [reverseResults, batchStart],
  );

  if (reverseFlashcards.length === 0) {
    return null;
  }

  const card = reverseFlashcards[currentIndex];
  const isFinalCard = currentIndex + 1 >= total;

  const afterSubmit = () => {
    const isBatchBoundary = (currentIndex + 1) % feedbackInterval === 0;
    if (isFinalCard || isBatchBoundary) {
      setPhase("batch-feedback");
    } else {
      nextCard();
    }
  };

  const handleSubmit = async (isRetry = false) => {
    if (loading || !answer.trim()) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    setLoading(true);
    setError(null);

    try {
      const res = await quizClient.submitReverseAnswer({
        noteId: card.noteId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      if (res.classification === "synonym" && !isRetry) {
        setSynonymAnswer(userAnswer);
        setAnswer("");
        setPhase("synonym-retry");
        setLoading(false);
        setTimeout(() => inputRef.current?.focus(), 100);
        return;
      }

      const correct = isRetry && res.classification === "synonym" ? true : res.correct;

      storeSubmitResult({
        noteId: card.noteId,
        answer: isRetry ? `${synonymAnswer} -> ${userAnswer}` : userAnswer,
        correct,
        expression: res.expression,
        meaning: res.meaning,
        reason: isRetry && res.classification === "synonym"
          ? res.reason + " (accepted on retry)"
          : res.reason,
        contexts: res.contexts ?? [],
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
      const res = await quizClient.submitReverseAnswer({
        noteId: card.noteId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });

      storeSubmitResult({
        noteId: card.noteId,
        answer: "(skipped)",
        correct: false,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
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
    if (e.key !== "Enter" || loading) return;
    if (phase === "answering") handleSubmit();
    else if (phase === "synonym-retry") handleSubmit(true);
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

      {phase === "synonym-retry" ? (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {card.meaning}
          </Heading>

          <Box
            p={3}
            borderRadius="md"
            bg="orange.100"
            color="orange.800"
            _dark={{ bg: "orange.900", color: "orange.200" }}
          >
            <Text fontWeight="bold">
              That&apos;s a valid synonym! But we&apos;re looking for a specific word.
            </Text>
            <Text fontSize="sm" mt={1}>
              Your word &quot;{synonymAnswer}&quot; means the same thing. Try the exact word.
            </Text>
          </Box>

          {card.contexts.length > 0 && (
            <VStack align="stretch" gap={2}>
              {card.contexts.map((ctx, i) => (
                <Text
                  key={i}
                  fontSize="md"
                  color="gray.600"
                  _dark={{ color: "gray.400" }}
                  fontStyle="italic"
                >
                  {ctx.maskedContext}
                </Text>
              ))}
            </VStack>
          )}

          <AnswerInput
            ref={inputRef}
            label="Word"
            value={answer}
            onChange={setAnswer}
            onKeyDown={handleKeyDown}
            onSubmit={() => handleSubmit(true)}
            onSkip={handleSkip}
            placeholder="Try again..."
          />
        </VStack>
      ) : phase === "answering" ? (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {card.meaning}
          </Heading>

          {card.contexts.length > 0 && (
            <VStack align="stretch" gap={2}>
              {card.contexts.map((ctx, i) => (
                <Text
                  key={i}
                  fontSize="md"
                  color="gray.600"
                  _dark={{ color: "gray.400" }}
                  fontStyle="italic"
                >
                  {ctx.maskedContext}
                </Text>
              ))}
            </VStack>
          )}

          <AnswerInput
            ref={inputRef}
            label="Word"
            value={answer}
            onChange={setAnswer}
            onKeyDown={handleKeyDown}
            onSubmit={() => handleSubmit()}
            onSkip={handleSkip}
            placeholder="Type the word"
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
