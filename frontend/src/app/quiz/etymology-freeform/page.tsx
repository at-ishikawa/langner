"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Input, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

export default function EtymologyFreeformQuizPage() {
  const router = useRouter();
  const quizType = useQuizStore((s) => s.quizType);
  const etymologyOriginResults = useQuizStore((s) => s.etymologyOriginResults);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyOriginResult);
  const etymologyFreeformOrigins = useQuizStore((s) => s.etymologyFreeformOrigins);
  const etymologyFreeformNextReviewDates = useQuizStore((s) => s.etymologyFreeformNextReviewDates);
  const reset = useQuizStore((s) => s.reset);

  const [origin, setOrigin] = useState("");
  const [meaning, setMeaning] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{
    correct: boolean; reason: string; correctMeaning: string;
    type: string; language: string; notebookName?: string;
    learnedAt?: string; noteId?: bigint;
    allSenses: { meaning: string; type: string; language: string; sessionTitle: string }[];
  } | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [skipped, setSkipped] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(false);
  const [overrideOriginals, setOverrideOriginals] = useState<{
    quality: number;
    status: string;
    intervalDays: number;
  } | null>(null);
  const startTimeRef = useRef(Date.now());
  const originInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (quizType !== "etymology-freeform") router.push("/");
    originInputRef.current?.focus();
  }, [quizType, router]);

  const originStatus: null | boolean | string = useMemo(() => {
    const trimmed = origin.trim();
    if (!trimmed || etymologyFreeformOrigins.length === 0) return null;
    const lower = trimmed.toLowerCase();
    const found = etymologyFreeformOrigins.some((e) => e.toLowerCase() === lower);
    if (!found) return false;
    const nextReview = etymologyFreeformNextReviewDates[lower];
    if (nextReview) return nextReview;
    return true;
  }, [origin, etymologyFreeformOrigins, etymologyFreeformNextReviewDates]);

  const canSubmit = origin.trim() && meaning.trim();

  const handleSubmit = async () => {
    if (!canSubmit) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    setLoading(true); setFeedback(null); setError(null);
    try {
      const res = await quizClient.submitEtymologyFreeformAnswer({
        origin: origin.trim(), meaning: meaning.trim(),
        responseTimeMs: BigInt(responseTimeMs),
      });
      const fb = {
        correct: res.correct, reason: res.reason, correctMeaning: res.correctMeaning,
        type: res.type, language: res.language,
        notebookName: res.notebookName || undefined,
        learnedAt: res.learnedAt || undefined,
        noteId: res.noteId ? BigInt(res.noteId) : undefined,
        allSenses: (res.allSenses ?? []).map((s) => ({
          meaning: s.meaning, type: s.type, language: s.language, sessionTitle: s.sessionTitle,
        })),
      };
      setFeedback(fb);
      setDisplayCorrect(fb.correct);
      storeSubmitResult({
        noteId: fb.noteId, origin: origin.trim(), answer: meaning.trim(),
        correct: res.correct, reason: res.reason, correctAnswer: res.correctMeaning,
        type: res.type, language: res.language, notebookName: res.notebookName || undefined,
        learnedAt: fb.learnedAt,
      });
    } catch { setError("Failed to submit answer"); } finally { setLoading(false); }
  };

  const handleNext = () => {
    setOrigin(""); setMeaning(""); setFeedback(null); setError(null);
    setOverridden(false); setSkipped(false); setDisplayCorrect(false); setOverrideOriginals(null);
    startTimeRef.current = Date.now(); originInputRef.current?.focus();
  };

  return (
    <Box p={4} maxW="sm" mx="auto" minH="100dvh">
      <Heading size="lg" mb={2}>Freeform Etymology</Heading>
      <Text mb={4} color="gray.600" _dark={{ color: "gray.400" }} fontSize="sm">
        Type any origin and its meaning. The origin must exist in your etymology notebooks.
      </Text>
      {loading ? (
        <Box textAlign="center" py={8}><Spinner size="lg" mb={4} /><Text>Checking your answer...</Text></Box>
      ) : error ? (
        <VStack align="stretch" gap={4}>
          <Text color="red.500">{error}</Text>
          <Button w="full" colorPalette="blue" variant="outline" onClick={() => { setError(null); originInputRef.current?.focus(); }}>Retry</Button>
        </VStack>
      ) : feedback ? (
        <VStack align="stretch" gap={4}>
          <FeedbackActions isCorrect={displayCorrect} noteId={feedback.noteId} isOverridden={overridden} isSkipped={skipped}
            nextLabel="Next Origin" onNext={handleNext}
            onOverride={feedback.noteId ? async () => {
              try {
                const res = await quizClient.overrideAnswer({ noteId: feedback.noteId!, quizType: ProtoQuizType.ETYMOLOGY_FREEFORM, learnedAt: feedback.learnedAt!, markCorrect: !displayCorrect });
                setOverridden(true); setDisplayCorrect(!displayCorrect);
                setOverrideOriginals({ quality: res.originalQuality, status: res.originalStatus, intervalDays: res.originalIntervalDays });
              } catch {}
            } : undefined}
            onUndo={feedback.noteId ? async () => {
              try {
                const res = await quizClient.undoOverrideAnswer({ noteId: feedback.noteId!, quizType: ProtoQuizType.ETYMOLOGY_FREEFORM, learnedAt: feedback.learnedAt!, originalQuality: overrideOriginals?.quality ?? 0, originalStatus: overrideOriginals?.status ?? "", originalIntervalDays: overrideOriginals?.intervalDays ?? 0 });
                setOverridden(false); setOverrideOriginals(null); setDisplayCorrect(res.correct);
              } catch { setOverridden(false); setOverrideOriginals(null); setDisplayCorrect(feedback.correct); }
            } : undefined}
            onSkip={feedback.noteId ? async () => { try { await quizClient.skipWord({ noteId: feedback.noteId! }); setSkipped(true); } catch {} } : undefined}
            onSeeResults={etymologyOriginResults.length > 0 ? () => router.push("/quiz/complete") : undefined}
          >
            <Box p={4} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
              <Text fontSize="xl" fontWeight="bold">{origin} = {feedback.correctMeaning}</Text>
              <Box display="flex" gap={2} mt={1}>
                {feedback.type && <Box px={2} py={0.5} borderRadius="full" bg="blue.100" _dark={{ bg: "blue.900" }}><Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }}>{feedback.type}</Text></Box>}
                {feedback.language && <Box px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}><Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{feedback.language}</Text></Box>}
              </Box>
            </Box>
            {feedback.allSenses.length > 1 && (
              <Box p={3} borderWidth="1px" borderRadius="lg" bg="yellow.50" _dark={{ bg: "yellow.900", borderColor: "yellow.700" }}>
                <Text fontSize="sm" fontWeight="bold" mb={2}>Other senses of {origin}</Text>
                <VStack align="stretch" gap={1}>
                  {feedback.allSenses
                    .filter((s) => s.meaning !== feedback.correctMeaning)
                    .map((s, i) => (
                      <Text key={i} fontSize="sm">
                        <Text as="span" fontWeight="medium">{s.meaning}</Text>
                        {s.sessionTitle && <Text as="span" color="gray.500" _dark={{ color: "gray.400" }}> — {s.sessionTitle}</Text>}
                      </Text>
                    ))}
                </VStack>
              </Box>
            )}
            {feedback.reason && <Box><Text fontWeight="bold">Reason</Text><Text>{feedback.reason}</Text></Box>}
            {feedback.notebookName && <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>Found in: {feedback.notebookName}</Text>}
          </FeedbackActions>
          <Button variant="ghost" onClick={() => { reset(); router.push("/"); }}>Back to Start</Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Box><Text fontWeight="medium" mb={1} fontSize="sm">Origin</Text>
            <Input ref={originInputRef} value={origin} onChange={(e) => setOrigin(e.target.value)} placeholder="e.g., spect" size="lg" />
            {originStatus === true && <Text fontSize="sm" color="green.500" mt={1}>Found in notebooks</Text>}
            {originStatus === false && <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} mt={1}>Origin not found in notebooks</Text>}
            {typeof originStatus === "string" && <Text fontSize="sm" color="orange.500" _dark={{ color: "orange.300" }} mt={1}>Not due until {originStatus}</Text>}
          </Box>
          <Box><Text fontWeight="medium" mb={1} fontSize="sm">Meaning</Text>
            <Input value={meaning} onChange={(e) => setMeaning(e.target.value)} placeholder="e.g., to look or see" size="lg" />
          </Box>
          <Button colorPalette="blue" onClick={handleSubmit} disabled={!canSubmit || originStatus === false || typeof originStatus === "string"} size="lg">Submit</Button>
          {etymologyOriginResults.length > 0 && <Button colorPalette="green" variant="outline" onClick={() => router.push("/quiz/complete")}>See Results</Button>}
          <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} textAlign="center">{etymologyFreeformOrigins.length} origins available in your notebooks</Text>
        </VStack>
      )}
    </Box>
  );
}
