"use client";

import { useEffect, useRef, useState } from "react";
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
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";
import { AnswerInput } from "@/components/AnswerInput";

type QuizPhase = "answering" | "synonym-retry" | "feedback";

interface FeedbackData {
  correct: boolean;
  expression: string;
  meaning: string;
  reason: string;
  contexts: string[];
  pronunciation?: string;
  partOfSpeech?: string;
  learnedAt?: string;
  images?: string[];
  originParts?: { origin: string; type: string; language: string; meaning: string }[];
}

export default function ReverseQuizPage() {
  const router = useRouter();
  const reverseFlashcards = useQuizStore((s) => s.reverseFlashcards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitReverseResult);
  const storeSkipResult = useQuizStore((s) => s.skipResult);
  const storeOverrideResult = useQuizStore((s) => s.overrideResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [submittedAnswer, setSubmittedAnswer] = useState("");
  const [synonymAnswer, setSynonymAnswer] = useState("");
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
    if (reverseFlashcards.length === 0 || quizType !== "reverse") {
      router.push("/");
    }
  }, [reverseFlashcards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering");
    setAnswer("");
    setSubmittedAnswer("");
    setSynonymAnswer("");
    setFeedback(null);
    setOverridden(false);
    setSkipped(false);
    setDisplayCorrect(false);
    setOverrideOriginals(null);
    setTimeout(() => inputRef.current?.focus(), 100);
  }, [currentIndex]);

  if (reverseFlashcards.length === 0) {
    return null;
  }

  const card = reverseFlashcards[currentIndex];
  const total = reverseFlashcards.length;
  const progress = ((currentIndex + 1) / total) * 100;

  const handleSubmit = async (isRetry = false) => {
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
      const res = await quizClient.submitReverseAnswer({
        noteId: card.noteId,
        answer: userAnswer,
        responseTimeMs: BigInt(responseTimeMs),
      });

      // Synonym on first attempt: show hint and let user retry
      if (res.classification === "synonym" && !isRetry) {
        setSynonymAnswer(userAnswer);
        setPhase("synonym-retry");
        setLoading(false);
        setTimeout(() => inputRef.current?.focus(), 100);
        return;
      }

      // On retry with synonym, accept as correct with lower quality
      const correct = isRetry && res.classification === "synonym" ? true : res.correct;

      setFeedback({
        correct,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
        pronunciation: res.wordDetail?.pronunciation?.trim() || undefined,
        partOfSpeech: res.wordDetail?.partOfSpeech?.trim() || undefined,
        learnedAt: res.learnedAt || undefined,
        images: res.images.length > 0 ? res.images : undefined,
        originParts: res.wordDetail?.originParts?.length ? res.wordDetail.originParts : undefined,
      });
      setDisplayCorrect(correct);
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
      const res = await quizClient.submitReverseAnswer({
        noteId: card.noteId,
        answer: "I don't know",
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: false,
        expression: res.expression,
        meaning: res.meaning,
        reason: res.reason,
        contexts: res.contexts ?? [],
        pronunciation: res.wordDetail?.pronunciation?.trim() || undefined,
        partOfSpeech: res.wordDetail?.partOfSpeech?.trim() || undefined,
        learnedAt: res.learnedAt || undefined,
        images: res.images.length > 0 ? res.images : undefined,
        originParts: res.wordDetail?.originParts?.length ? res.wordDetail.originParts : undefined,
      });
      setDisplayCorrect(false);
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
      } else if (phase === "synonym-retry") {
        handleSubmit(true);
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
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Heading size="xl" textAlign="center" color="blue.700" _dark={{ color: "blue.300" }}>
            {card.meaning}
          </Heading>

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
                      quizType: ProtoQuizType.REVERSE,
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
                    storeOverrideResult(currentIndex, "reverse", res.nextReviewDate || "", {
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
                      quizType: ProtoQuizType.REVERSE,
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
                    storeSkipResult(currentIndex, "reverse");
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

                {/* Word, pronunciation, part of speech, reason, examples */}
                <Box>
                  <Text fontWeight="bold">Word</Text>
                  <Text fontStyle="italic">
                    {feedback.expression}
                    {(feedback.pronunciation || feedback.partOfSpeech) && (
                      <Text as="span" fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} fontStyle="normal">
                        {" "}
                        {[
                          feedback.pronunciation && `/${feedback.pronunciation}/`,
                          feedback.partOfSpeech,
                        ].filter(Boolean).join(" · ")}
                      </Text>
                    )}
                  </Text>
                </Box>

                {feedback.reason && (
                  <Box>
                    <Text fontWeight="bold">Reason</Text>
                    <Text>{feedback.reason}</Text>
                  </Box>
                )}

                {feedback.contexts.length > 0 && (
                  <Box>
                    <Text fontWeight="bold">Context</Text>
                    <VStack align="stretch" gap={1} mt={1}>
                      {feedback.contexts.map((ctx, i) => (
                        <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }} fontStyle="italic">
                          {i + 1}. {ctx}
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

                {feedback.originParts && feedback.originParts.length > 0 && (
                  <Box>
                    <Text fontWeight="bold" fontSize="sm">Etymology</Text>
                    <Box display="flex" gap={2} alignItems="center" flexWrap="wrap">
                      {feedback.originParts.map((p, i) => (
                        <Box key={i} display="flex" alignItems="center" gap={1}>
                          {i > 0 && <Text color="fg.muted">+</Text>}
                          <Text color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium" fontSize="sm">{p.origin}</Text>
                          <Text fontSize="sm" color="fg.muted">({p.meaning})</Text>
                          {p.language && (
                            <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                              <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                            </Box>
                          )}
                        </Box>
                      ))}
                    </Box>
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
