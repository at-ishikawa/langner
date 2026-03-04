"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Input,
  Spinner,
  Text,
  Textarea,
  VStack,
} from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

export default function FreeformQuizPage() {
  const router = useRouter();
  const quizType = useQuizStore((s) => s.quizType);
  const wordCount = useQuizStore((s) => s.wordCount);
  const storeSubmitResult = useQuizStore((s) => s.submitFreeformResult);
  const reset = useQuizStore((s) => s.reset);

  const [word, setWord] = useState("");
  const [meaning, setMeaning] = useState("");
  const [submittedWord, setSubmittedWord] = useState("");
  const [submittedMeaning, setSubmittedMeaning] = useState("");
  const [loading, setLoading] = useState(false);
  const [feedback, setFeedback] = useState<{
    correct: boolean;
    word: string;
    meaning: string;
    reason: string;
    notebookName: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const startTimeRef = useRef(Date.now());
  const wordInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (quizType !== "freeform") {
      router.push("/");
    }
    wordInputRef.current?.focus();
  }, [quizType, router]);

  const handleSubmit = async () => {
    if (!word.trim() || !meaning.trim()) return;

    const responseTimeMs = Date.now() - startTimeRef.current;
    setSubmittedWord(word.trim());
    setSubmittedMeaning(meaning.trim());
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitFreeformAnswer({
        word: word.trim(),
        meaning: meaning.trim(),
        responseTimeMs: BigInt(responseTimeMs),
      });

      setFeedback({
        correct: res.correct,
        word: res.word,
        meaning: res.meaning,
        reason: res.reason,
        notebookName: res.notebookName,
      });
      storeSubmitResult({
        word: res.word,
        answer: meaning.trim(),
        correct: res.correct,
        meaning: res.meaning,
        reason: res.reason,
        notebookName: res.notebookName,
      });
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    setWord("");
    setMeaning("");
    setSubmittedWord("");
    setSubmittedMeaning("");
    setFeedback(null);
    setError(null);
    startTimeRef.current = Date.now();
    wordInputRef.current?.focus();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && e.shiftKey) {
      handleSubmit();
    }
  };

  return (
    <Box p={4} maxW="md" mx="auto" minH="100dvh">
      <Heading size="lg" mb={4}>
        Freeform Quiz
      </Heading>

      <Text mb={4} color="gray.600">
        Type any word you&apos;re learning and its meaning
      </Text>

      {loading ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>Checking your answer...</Text>
        </Box>
      ) : feedback ? (
        <VStack align="stretch" gap={4}>
          <Box
            p={4}
            borderRadius="md"
            bg={feedback.correct ? "green.100" : "red.100"}
          >
            <Text fontWeight="bold" fontSize="lg">
              {feedback.correct ? "\u2713 Correct!" : "\u2717 Incorrect"}
            </Text>
          </Box>

          <Box>
            <Text fontWeight="bold">Word</Text>
            <Text fontSize="xl">{feedback.word}</Text>
          </Box>

          <Box>
            <Text fontWeight="bold">Correct meaning</Text>
            <Text fontStyle="italic">{feedback.meaning}</Text>
          </Box>

          {feedback.reason && (
            <Box>
              <Text fontWeight="bold">Reason</Text>
              <Text>{feedback.reason}</Text>
            </Box>
          )}

          {feedback.notebookName && (
            <Text fontSize="sm" color="gray.500">
              Found in: {feedback.notebookName}
            </Text>
          )}

          <Button colorPalette="blue" onClick={handleNext} mt={4}>
            Next Word
          </Button>

          <Button
            variant="ghost"
            onClick={() => {
              reset();
              router.push("/");
            }}
          >
            Back to Start
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Box>
            <Text fontWeight="medium" mb={1}>
              Word
            </Text>
            <Input
              ref={wordInputRef}
              value={word}
              onChange={(e) => setWord(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="e.g., hit the hay"
              size="lg"
            />
          </Box>

          <Box>
            <Text fontWeight="medium" mb={1}>
              Meaning
            </Text>
            <Textarea
              value={meaning}
              onChange={(e) => setMeaning(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="e.g., to go to bed; to sleep"
              size="lg"
              rows={2}
            />
          </Box>

          {error && <Text color="red.500">{error}</Text>}

          <Button
            colorPalette="blue"
            onClick={handleSubmit}
            disabled={!word.trim() || !meaning.trim()}
            size="lg"
          >
            Check Answer
          </Button>

          <Text fontSize="sm" color="gray.500" textAlign="center">
            {wordCount} words available in your notebooks
          </Text>
        </VStack>
      )}
    </Box>
  );
}
