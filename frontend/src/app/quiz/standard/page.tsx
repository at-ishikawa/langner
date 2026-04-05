"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Input,
  Progress,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

type QuizPhase = "answering" | "feedback";

interface FeedbackData {
  correct: boolean;
  meaning: string;
  reason: string;
  pronunciation?: string;
  partOfSpeech?: string;
  learnedAt?: string;
  images?: string[];
}

export default function QuizCardPage() {
  const router = useRouter();
  const flashcards = useQuizStore((s) => s.flashcards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitResult);
  const storeSkipResult = useQuizStore((s) => s.skipResult);
  const storeOverrideResult = useQuizStore((s) => s.overrideResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [submittedAnswer, setSubmittedAnswer] = useState("");
  const [feedback, setFeedback] = useState<FeedbackData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
  const [overrideOriginals, setOverrideOriginals] = useState<{
    quality: number;
    status: string;
    intervalDays: number;
  } | null>(null);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (flashcards.length === 0 || quizType !== "standard") {
      router.push("/");
    }
  }, [flashcards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setSubmittedAnswer("");
    setFeedback(null);
    setOverridden(false);
    setSkipped(false);
    setDisplayCorrect(false);
    setOverrideOriginals(null);
    setTimeout(() => inputRef.current?.focus(), 50);
  }, [currentIndex]);

  if (flashcards.length === 0) {
    return null;
  }

  const card = flashcards[currentIndex];
  const total = flashcards.length;
  const progress = ((currentIndex + 1) / total) * 100;

  const handleSubmit = async () => {
    if (!answer.trim()) return;

    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    setSubmittedAnswer(userAnswer);
    setAnswer("");
    setPhase("feedback");
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitAnswer({
        noteId: card.noteId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: res.correct,
        meaning: res.meaning,
        reason: res.reason,
        pronunciation: res.wordDetail?.pronunciation?.trim() || undefined,
        partOfSpeech: res.wordDetail?.partOfSpeech?.trim() || undefined,
        learnedAt: res.learnedAt || undefined,
        images: res.images.length > 0 ? res.images : undefined,
      });
      setDisplayCorrect(res.correct);
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
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleSkip = async () => {
    const responseTimeMs = Date.now() - startTimeRef.current;
    setSubmittedAnswer("");
    setAnswer("");
    setPhase("feedback");
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitAnswer({
        noteId: card.noteId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: false,
        meaning: res.meaning,
        reason: res.reason,
        pronunciation: res.wordDetail?.pronunciation?.trim() || undefined,
        partOfSpeech: res.wordDetail?.partOfSpeech?.trim() || undefined,
        learnedAt: res.learnedAt || undefined,
        images: res.images.length > 0 ? res.images : undefined,
      });
      setDisplayCorrect(false);
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
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    if (currentIndex + 1 >= total) {
      router.push("/quiz/complete");
    } else {
      nextCard();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      if (phase === "answering") {
        handleSubmit();
      } else if (phase === "feedback" && !loading) {
        handleNext();
      }
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
                  {ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`}
                </Text>
              ))}
            </VStack>
          )}

          <Box>
            <Text fontWeight="medium" mb={1}>
              Meaning
            </Text>
            <Input
              ref={inputRef}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type your answer"
              size="lg"
            />
          </Box>

          <Button
            colorPalette="blue"
            onClick={handleSubmit}
            disabled={!answer.trim()}
            size="lg"
          >
            Submit
          </Button>

          <Button
            variant="outline"
            onClick={handleSkip}
            size="lg"
          >
            Don&apos;t Know
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center">
            {card.entry}
          </Heading>
          {feedback && (feedback.pronunciation || feedback.partOfSpeech) && (
            <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} textAlign="center">
              {[
                feedback.pronunciation && `/${feedback.pronunciation}/`,
                feedback.partOfSpeech,
              ].filter(Boolean).join(" · ")}
            </Text>
          )}

          {loading ? (
            <Box textAlign="center" py={8}>
              <Spinner size="lg" mb={4} />
              <Text>Checking your answer...</Text>
            </Box>
          ) : feedback ? (
            <>
              <FeedbackActions
                isCorrect={displayCorrect}
                noteId={card.noteId}
                isOverridden={overridden}
                isSkipped={skipped}
                nextLabel={currentIndex + 1 >= total ? "See Results" : "Next"}
                onNext={handleNext}
                onOverride={async () => {
                  try {
                    const res = await quizClient.overrideAnswer({
                      noteId: card.noteId,
                      quizType: ProtoQuizType.STANDARD,
                      learnedAt: feedback.learnedAt!,
                      markCorrect: !displayCorrect,
                    });
                    setOverridden(true);
                    setDisplayCorrect(!displayCorrect);
                    setOverrideOriginals({
                      quality: res.originalQuality,
                      status: res.originalStatus,
                      intervalDays: res.originalIntervalDays,
                    });
                    storeOverrideResult(currentIndex, "standard", res.nextReviewDate || "", {
                      quality: res.originalQuality,
                      status: res.originalStatus,
                      intervalDays: res.originalIntervalDays,
                    });
                  } catch { /* silently fail */ }
                }}
                onUndo={async () => {
                  try {
                    const res = await quizClient.undoOverrideAnswer({
                      noteId: card.noteId,
                      quizType: ProtoQuizType.STANDARD,
                      learnedAt: feedback.learnedAt!,
                      originalQuality: overrideOriginals?.quality ?? 0,
                      originalStatus: overrideOriginals?.status ?? "",
                      originalIntervalDays: overrideOriginals?.intervalDays ?? 0,
                    });
                    setOverridden(false);
                    setOverrideOriginals(null);
                    setDisplayCorrect(res.correct);
                  } catch {
                    setOverridden(false);
                    setOverrideOriginals(null);
                    setDisplayCorrect(feedback.correct);
                  }
                }}
                onSkip={async () => {
                  try {
                    await quizClient.skipWord({ noteId: card.noteId });
                    setSkipped(true);
                    storeSkipResult(currentIndex, "standard");
                  } catch { /* silently fail */ }
                }}
                onSeeResults={currentIndex + 1 < total ? () => router.push("/quiz/complete") : undefined}
              >
                {/* Your answer */}
                {submittedAnswer ? (
                  <Text textDecoration={displayCorrect ? "none" : "line-through"}>
                    Your answer: {submittedAnswer}
                  </Text>
                ) : (
                  <Text color="gray.500" _dark={{ color: "gray.400" }} fontStyle="italic">
                    Skipped
                  </Text>
                )}

                <Box>
                  <Text fontWeight="bold">Meaning</Text>
                  <Text>{feedback.meaning}</Text>
                </Box>

                {feedback.reason && (
                  <Box>
                    <Text fontWeight="bold">Reason</Text>
                    <Text>{feedback.reason}</Text>
                  </Box>
                )}

                {card.examples.length > 0 && (
                  <Box>
                    <Text fontWeight="bold">Examples</Text>
                    <VStack align="stretch" gap={1} mt={1}>
                      {card.examples.map((ex, i) => (
                        <Text key={i} fontSize="sm" color="fg.muted" fontStyle="italic">
                          {ex.speaker ? `${ex.speaker}: "${ex.text}"` : `"${ex.text}"`}
                        </Text>
                      ))}
                    </VStack>
                  </Box>
                )}

                {feedback.images && feedback.images.length > 0 && (
                  <Box display="flex" gap={2} flexWrap="wrap">
                    {feedback.images.map((src, i) => (
                      <img key={i} src={src} alt="" style={{ maxHeight: "150px", borderRadius: "4px" }} />
                    ))}
                  </Box>
                )}
              </FeedbackActions>
            </>
          ) : error ? (
            <>
              <Text color="red.500">{error}</Text>
              <Button
                w="full"
                colorPalette="blue"
                variant="outline"
                onClick={() => {
                  setPhase("answering");
                  setError(null);
                  setAnswer(submittedAnswer);
                  setTimeout(() => inputRef.current?.focus(), 50);
                }}
              >
                Retry
              </Button>
              <Button
                w="full"
                colorPalette="blue"
                onClick={handleNext}
              >
                {currentIndex + 1 >= total ? "See Results" : "Skip"}
              </Button>
            </>
          ) : null}
        </VStack>
      )}
    </Box>
  );
}
