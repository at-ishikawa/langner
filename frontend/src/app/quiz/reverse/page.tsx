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
import { quizClient, type SubmitReverseAnswerResponse } from "@/lib/client";
import { useQuizStore, type ReverseFlashcard } from "@/store/quizStore";
import { AnswerInput } from "@/components/AnswerInput";
import { BatchFeedback } from "@/components/BatchFeedback";
import { reverseResultToItem } from "@/lib/quizResultItems";
import { useQuizResultActions } from "@/lib/useQuizResultActions";
import { responseTimeSince } from "@/lib/responseTime";

type QuizPhase = "answering" | "grading" | "synonym-retry" | "retry-grading" | "batch-feedback";

interface BufferedAnswer {
  card: ReverseFlashcard;
  answer: string;
  displayAnswer: string;
  responseTimeMs: bigint;
}

interface RetrySlot {
  index: number; // index into buffer
  originalAnswer: string;
}

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
  const [error, setError] = useState<string | null>(null);
  const [retryQueueIdx, setRetryQueueIdx] = useState(0);

  const bufferRef = useRef<BufferedAnswer[]>([]);
  const initialResponsesRef = useRef<SubmitReverseAnswerResponse[]>([]);
  const retrySlotsRef = useRef<RetrySlot[]>([]);
  const retryAnswersRef = useRef<Record<number, BufferedAnswer>>({});
  const startTimeRef = useRef<number>(0);
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
    setAnswer("");
    setError(null);
    if (phase === "answering" || phase === "synonym-retry") {
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [currentIndex, phase, retryQueueIdx]);

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

  if (reverseFlashcards.length === 0) return null;

  const card = reverseFlashcards[currentIndex];
  const isFinalCard = currentIndex + 1 >= total;

  const buildRequest = (b: BufferedAnswer) => ({
    noteId: b.card.noteId,
    answer: b.answer,
    responseTimeMs: b.responseTimeMs,
  });

  const persistResults = () => {
    const buffer = bufferRef.current;
    const initial = initialResponsesRef.current;
    buffer.forEach((b, i) => {
      const init = initial[i];
      const retry = retryAnswersRef.current[i];
      let finalCorrect = init.correct;
      let finalReason = init.reason;
      let finalAnswer = b.displayAnswer;
      const isRetrySynonym = retry !== undefined;
      if (isRetrySynonym) {
        finalAnswer = `${b.displayAnswer} -> ${retry.displayAnswer}`;
      }
      storeSubmitResult({
        noteId: b.card.noteId,
        answer: finalAnswer,
        correct: finalCorrect,
        expression: init.expression,
        meaning: init.meaning,
        reason: finalReason,
        contexts: init.contexts ?? [],
        wordDetail: init.wordDetail,
        learnedAt: init.learnedAt || undefined,
        images: init.images.length > 0 ? init.images : undefined,
      });
    });
  };

  const mergeRetry = (retryResponses: SubmitReverseAnswerResponse[]) => {
    retrySlotsRef.current.forEach((slot, j) => {
      const retryRes = retryResponses[j];
      const init = initialResponsesRef.current[slot.index];
      if (retryRes.classification === "synonym") {
        // Accept synonym on retry as correct (matches existing behavior)
        initialResponsesRef.current[slot.index] = {
          ...init,
          correct: true,
          reason: retryRes.reason + " (accepted on retry)",
        };
      } else {
        initialResponsesRef.current[slot.index] = retryRes;
      }
    });
  };

  const flushRetryBatch = async () => {
    setPhase("retry-grading");
    setError(null);
    try {
      const ordered = retrySlotsRef.current.map((slot) => retryAnswersRef.current[slot.index]);
      const res = await quizClient.batchSubmitReverseAnswers({
        answers: ordered.map(buildRequest),
      });
      mergeRetry(res.responses);
      persistResults();
      bufferRef.current = [];
      initialResponsesRef.current = [];
      retrySlotsRef.current = [];
      retryAnswersRef.current = {};
      setPhase("batch-feedback");
    } catch {
      setError("Failed to submit retry answers");
      setPhase("synonym-retry");
    }
  };

  const startSynonymRetryFlow = (responses: SubmitReverseAnswerResponse[]) => {
    initialResponsesRef.current = responses;
    const synonymSlots: RetrySlot[] = [];
    responses.forEach((r, i) => {
      if (r.classification === "synonym") {
        synonymSlots.push({ index: i, originalAnswer: bufferRef.current[i].displayAnswer });
      }
    });
    if (synonymSlots.length === 0) {
      persistResults();
      bufferRef.current = [];
      initialResponsesRef.current = [];
      setPhase("batch-feedback");
      return;
    }
    retrySlotsRef.current = synonymSlots;
    retryAnswersRef.current = {};
    setRetryQueueIdx(0);
    setPhase("synonym-retry");
  };

  const flushBatch = async (toFlush: BufferedAnswer[]) => {
    setPhase("grading");
    setError(null);
    try {
      const res = await quizClient.batchSubmitReverseAnswers({
        answers: toFlush.map(buildRequest),
      });
      startSynonymRetryFlow(res.responses);
    } catch {
      setError("Failed to submit answers");
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
    if (phase !== "answering" || !answer.trim()) return;
    const userAnswer = answer.trim();
    recordAndAdvance({
      card,
      answer: userAnswer,
      displayAnswer: userAnswer,
      responseTimeMs: responseTimeSince(startTimeRef.current),
    });
  };

  const handleSkip = () => {
    if (phase !== "answering") return;
    recordAndAdvance({
      card,
      answer: "I don't know",
      displayAnswer: "(skipped)",
      responseTimeMs: responseTimeSince(startTimeRef.current),
    });
  };

  const recordRetryAndAdvance = (entry: BufferedAnswer) => {
    const slot = retrySlotsRef.current[retryQueueIdx];
    retryAnswersRef.current[slot.index] = entry;
    if (retryQueueIdx + 1 >= retrySlotsRef.current.length) {
      void flushRetryBatch();
    } else {
      setRetryQueueIdx(retryQueueIdx + 1);
    }
  };

  const handleRetrySubmit = () => {
    if (phase !== "synonym-retry" || !answer.trim()) return;
    const userAnswer = answer.trim();
    const slot = retrySlotsRef.current[retryQueueIdx];
    const originalBuffered = bufferRef.current[slot.index];
    recordRetryAndAdvance({
      card: originalBuffered.card,
      answer: userAnswer,
      displayAnswer: userAnswer,
      responseTimeMs: responseTimeSince(startTimeRef.current),
    });
  };

  const handleRetrySkip = () => {
    if (phase !== "synonym-retry") return;
    const slot = retrySlotsRef.current[retryQueueIdx];
    const originalBuffered = bufferRef.current[slot.index];
    // Sending "I don't know" makes the backend classify it as wrong on retry,
    // so mergeRetry replaces the initial synonym response with an incorrect one.
    recordRetryAndAdvance({
      card: originalBuffered.card,
      answer: "I don't know",
      displayAnswer: "(skipped)",
      responseTimeMs: responseTimeSince(startTimeRef.current),
    });
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
    if (e.key !== "Enter") return;
    if (phase === "answering") handleSubmit();
    else if (phase === "synonym-retry") handleRetrySubmit();
  };

  const currentRetrySlot =
    phase === "synonym-retry" && retryQueueIdx < retrySlotsRef.current.length
      ? retrySlotsRef.current[retryQueueIdx]
      : null;
  const retryCard = currentRetrySlot ? bufferRef.current[currentRetrySlot.index]?.card : null;

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
      ) : phase === "grading" || phase === "retry-grading" ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>{phase === "grading" ? "Checking your answers..." : "Checking retries..."}</Text>
        </Box>
      ) : phase === "synonym-retry" && retryCard && currentRetrySlot ? (
        <VStack align="stretch" gap={4}>
          <Text fontSize="sm" color="fg.muted">
            Retry {retryQueueIdx + 1} / {retrySlotsRef.current.length}
          </Text>
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {retryCard.meaning}
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
              Your word &quot;{currentRetrySlot.originalAnswer}&quot; means the same thing. Try the exact word.
            </Text>
          </Box>

          {retryCard.contexts.length > 0 && (
            <VStack align="stretch" gap={2}>
              {retryCard.contexts.map((ctx, i) => (
                <Text key={i} fontSize="md" color="gray.600" _dark={{ color: "gray.400" }} fontStyle="italic">
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
            onSubmit={handleRetrySubmit}
            onSkip={handleRetrySkip}
            placeholder="Try again..."
          />
          {error && (
            <VStack align="stretch" gap={2}>
              <Text color="red.500">{error}</Text>
              <Button w="full" colorPalette="blue" variant="outline" onClick={flushRetryBatch}>
                Retry grading
              </Button>
            </VStack>
          )}
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {card.meaning}
          </Heading>

          {card.contexts.length > 0 && (
            <VStack align="stretch" gap={2}>
              {card.contexts.map((ctx, i) => (
                <Text key={i} fontSize="md" color="gray.600" _dark={{ color: "gray.400" }} fontStyle="italic">
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
            onSubmit={handleSubmit}
            onSkip={handleSkip}
            placeholder="Type the word"
          />

          {error && (
            <VStack align="stretch" gap={2}>
              <Text color="red.500">{error}</Text>
              <Button w="full" colorPalette="blue" variant="outline" onClick={() => void flushBatch(bufferRef.current)}>
                Retry grading
              </Button>
            </VStack>
          )}
        </VStack>
      )}
    </Box>
  );
}
