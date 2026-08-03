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
  // A word left blank is submitted as an empty answer and graded incorrect —
  // there is no "skip"/"don't know" control.
  answers: Record<string, string>;
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
  const [error, setError] = useState<string | null>(null);
  const [pendingRetry, setPendingRetry] = useState<BufferedAnswer[] | null>(null);
  const bufferRef = useRef<BufferedAnswer[]>([]);
  const startTimeRef = useRef<number>(0);
  const firstInputRef = useRef<HTMLInputElement>(null);

  const {
    handleOverride, handleUndo, handleSkip: handleItemSkip, handleResume,
    handleOverrideWord,
  } = useQuizResultActions(quizType);

  useEffect(() => {
    if (etymologyOriginCards.length === 0 || quizType !== "etymology-origin") router.push("/");
  }, [etymologyOriginCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setInputs({});
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
          englishForms: b.card.englishForms,
          note: b.card.note,
          correct: r.correct,
          words: r.results.map((wr) => ({
            wordId: wr.wordId,
            expression: wr.expression,
            correct: wr.correct,
            correctMeaning: wr.correctMeaning,
            reason: wr.reason,
            userAnswer: b.answers[wr.wordId.toString()] ?? "",
            pronunciation: wr.pronunciation,
            examples: wr.examples,
            literal: wr.literal,
          })),
          notebookName: b.card.notebookName,
          nextReviewDate: r.nextReviewDate || undefined,
          learnedAt: r.learnedAt || undefined,
          senseId: r.senseId || undefined,
          // Note: isSkipped is intentionally NOT set here. It means "excluded
          // from future quizzes" (the origin-level SkipWord/Exclude action),
          // which is set only via the deliberate Exclude control — never by
          // grading a fresh submission.
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

  // handleSubmit grades the card as-is: any word left blank is submitted as an
  // empty answer and graded incorrect (a normal miss), matching the grammar
  // quiz model. There is no skip/"don't know" control — leaving a word blank
  // and submitting is how the learner records "I don't know this one".
  const handleSubmit = () => {
    if (phase !== "answering") return;
    const responseTime = responseTimeSince(startTimeRef.current);
    recordAndAdvance({
      card,
      answers: { ...inputs },
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
            {/* English combining forms (e.g. fac, fic, fect) as chips — study
                context, distinct from the source-language principal parts. */}
            {card.englishForms && card.englishForms.length > 0 && (
              <Box display="flex" gap={1.5} justifyContent="center" mt={3} flexWrap="wrap">
                {card.englishForms.map((ef) => (
                  <Box key={ef} px={2} py={0.5} borderRadius="md" bg="teal.100" _dark={{ bg: "teal.900" }}>
                    <Text fontSize="xs" color="teal.700" _dark={{ color: "teal.200" }} fontFamily="mono">{ef}</Text>
                  </Box>
                ))}
              </Box>
            )}
            {/* Origin note: free-text pedagogical hint about the root. */}
            {card.note && (
              <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mt={3} fontStyle="italic">
                {card.note}
              </Text>
            )}
          </Box>

          {/* One input per derived family word. The whole family is visible as
              context; the user types each word's meaning. A word left blank is
              graded incorrect on Submit — there is no skip control. */}
          <Box>
            <Text fontWeight="medium" mb={2}>Type the meaning of each word</Text>
            <VStack align="stretch" gap={3}>
              {card.words.map((w, i) => {
                const key = w.wordId.toString();
                return (
                  <Box key={key}>
                    <Box minW={0} mb={1}>
                      <Text fontSize="sm" fontWeight="semibold">{w.expression}</Text>
                      {/* Pronunciation is a safe hint — it doesn't reveal the
                          meaning. Example sentence + literal gloss are held
                          back until the feedback screen. */}
                      {w.pronunciation && (
                        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
                          /{w.pronunciation}/
                        </Text>
                      )}
                    </Box>
                    <Input
                      ref={i === 0 ? firstInputRef : undefined}
                      value={inputs[key] ?? ""}
                      onChange={(e) => {
                        setInputs((prev) => ({ ...prev, [key]: e.target.value }));
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") handleSubmit();
                      }}
                      placeholder="type the meaning..."
                      size="lg"
                      aria-label={`Meaning of ${w.expression}`}
                    />
                  </Box>
                );
              })}
            </VStack>
          </Box>

          <Box display="flex" gap={2} position="sticky" bottom={4}>
            <Button flex="1" colorPalette="blue" onClick={handleSubmit} size="lg">
              Submit
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
