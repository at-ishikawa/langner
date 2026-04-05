"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Input, Progress, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

type QuizPhase = "answering" | "feedback";

export default function EtymologyStandardPage() {
  const router = useRouter();
  const etymologyOriginCards = useQuizStore((s) => s.etymologyOriginCards);
  const quizType = useQuizStore((s) => s.quizType);
  const currentIndex = useQuizStore((s) => s.currentIndex);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyOriginResult);
  const storeSkipResult = useQuizStore((s) => s.skipResult);
  const nextCard = useQuizStore((s) => s.nextCard);

  const [phase, setPhase] = useState<QuizPhase>("answering");
  const [answer, setAnswer] = useState("");
  const [submittedAnswer, setSubmittedAnswer] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{
    correct: boolean; reason: string; correctMeaning: string;
    learnedAt?: string; noteId?: bigint;
  } | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
  const startTimeRef = useRef(Date.now());
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (etymologyOriginCards.length === 0 || quizType !== "etymology-standard") router.push("/");
  }, [etymologyOriginCards, quizType, router]);

  useEffect(() => {
    startTimeRef.current = Date.now();
    setPhase("answering"); setAnswer(""); setSubmittedAnswer("");
    setFeedback(null); setOverridden(false); setSkipped(false); setDisplayCorrect(false);
    setTimeout(() => inputRef.current?.focus(), 50);
  }, [currentIndex]);

  if (etymologyOriginCards.length === 0) return null;

  const card = etymologyOriginCards[currentIndex];
  const total = etymologyOriginCards.length;
  const progress = ((currentIndex + 1) / total) * 100;

  const handleSubmit = async () => {
    if (!answer.trim()) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    const userAnswer = answer.trim();
    setSubmittedAnswer(userAnswer); setAnswer(""); setPhase("feedback");
    setLoading(true); setFeedback(null); setError(null);
    try {
      const res = await quizClient.submitEtymologyStandardAnswer({
        cardId: card.cardId, answer: userAnswer, responseTimeMs: BigInt(responseTimeMs),
      });
      const fb = { correct: res.correct, reason: res.reason, correctMeaning: res.correctMeaning,
        learnedAt: res.learnedAt || undefined, noteId: res.noteId ? BigInt(res.noteId) : undefined };
      setFeedback(fb); setDisplayCorrect(res.correct);
      storeSubmitResult({ noteId: fb.noteId, cardId: card.cardId, origin: card.origin,
        answer: userAnswer, correct: res.correct, reason: res.reason,
        correctAnswer: res.correctMeaning, type: card.type, language: card.language,
        learnedAt: fb.learnedAt });
    } catch { setError("Failed to submit answer"); } finally { setLoading(false); }
  };

  const handleNext = () => { if (currentIndex + 1 >= total) router.push("/quiz/complete"); else nextCard(); };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") { if (phase === "answering") handleSubmit(); else if (phase === "feedback" && !loading) handleNext(); }
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
            <Heading size="xl">{card.origin}</Heading>
            <Box display="flex" gap={2} justifyContent="center" mt={2}>
              {card.type && <Box px={2} py={0.5} borderRadius="full" bg="blue.100" _dark={{ bg: "blue.900" }}><Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }}>{card.type}</Text></Box>}
              {card.language && <Box px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}><Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{card.language}</Text></Box>}
            </Box>
          </Box>
          <Box><Text fontWeight="medium" mb={1}>What does this origin mean?</Text>
            <Input ref={inputRef} value={answer} onChange={(e) => setAnswer(e.target.value)} onKeyDown={handleKeyDown} placeholder="type the meaning..." size="lg" />
          </Box>
          <Button colorPalette="blue" onClick={handleSubmit} disabled={!answer.trim()} size="lg" position="sticky" bottom={4}>Submit</Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          {loading ? (<Box textAlign="center" py={8}><Spinner size="lg" mb={4} /><Text>Checking your answer...</Text></Box>
          ) : feedback ? (
            <>
              <Box p={3} borderRadius="md" bg={displayCorrect ? "green.100" : "red.100"} color={displayCorrect ? "green.800" : "red.800"} _dark={{ bg: displayCorrect ? "green.900" : "red.900", color: displayCorrect ? "green.200" : "red.200" }} textAlign="center">
                <Text fontWeight="bold" fontSize="md">{displayCorrect ? "\u2713 Correct" : "\u2717 Incorrect"}{overridden && <Text as="span" fontWeight="normal" fontStyle="italic"> (overridden)</Text>}</Text>
              </Box>
              <Box p={4} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                <Text fontSize="xl" fontWeight="bold">{card.origin} = {feedback.correctMeaning}</Text>
                <Box display="flex" gap={2} mt={1}>
                  {card.type && <Box px={2} py={0.5} borderRadius="full" bg="blue.100" _dark={{ bg: "blue.900" }}><Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }}>{card.type}</Text></Box>}
                  {card.language && <Box px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}><Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{card.language}</Text></Box>}
                </Box>
              </Box>
              {submittedAnswer && <Box><Text fontWeight="medium" fontSize="sm" mb={1}>Your answer</Text>
                <Box p={3} borderWidth="1.5px" borderRadius="lg" borderColor={displayCorrect ? "green.600" : "red.600"} bg="white" _dark={{ bg: "gray.800" }} display="flex" justifyContent="space-between" alignItems="center">
                  <Text textDecoration={displayCorrect ? "none" : "line-through"} color={displayCorrect ? undefined : "red.600"}>{submittedAnswer}</Text>
                  <Text fontWeight="medium" color={displayCorrect ? "green.600" : "red.600"}>{displayCorrect ? "\u2713" : "\u2717"}</Text>
                </Box></Box>}
              {feedback.reason && <Box><Text fontWeight="bold">Reason</Text><Text>{feedback.reason}</Text></Box>}
              <FeedbackActions isCorrect={displayCorrect} noteId={feedback.noteId} isOverridden={overridden} isSkipped={skipped}
                nextLabel={currentIndex + 1 >= total ? "See Results" : "Next"} onNext={handleNext}
                onOverride={feedback.noteId ? async () => { try { await quizClient.overrideAnswer({ noteId: feedback.noteId!, quizType: ProtoQuizType.ETYMOLOGY_STANDARD, learnedAt: feedback.learnedAt!, markCorrect: !displayCorrect }); setOverridden(true); setDisplayCorrect(!displayCorrect); } catch {} } : undefined}
                onSkip={feedback.noteId ? async () => { try { await quizClient.skipWord({ noteId: feedback.noteId! }); setSkipped(true); storeSkipResult(currentIndex, "etymology-standard"); } catch {} } : undefined} />
            </>
          ) : error ? (<><Text color="red.500">{error}</Text>
            <Button w="full" colorPalette="blue" variant="outline" onClick={() => { setPhase("answering"); setError(null); setAnswer(submittedAnswer); setTimeout(() => inputRef.current?.focus(), 50); }}>Retry</Button>
            <Button w="full" colorPalette="blue" onClick={handleNext}>{currentIndex + 1 >= total ? "See Results" : "Skip"}</Button></>
          ) : null}
        </VStack>
      )}
    </Box>
  );
}
