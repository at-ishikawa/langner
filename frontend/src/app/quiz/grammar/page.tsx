"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Box, Button, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import { AnswerInput } from "@/components/AnswerInput";
import { quizClient } from "@/lib/client";
import { useGrammarStore, type GrammarResult } from "@/store/grammarStore";

// renderPost shows the full journal post with its line breaks preserved and
// the incorrect span highlighted in place, so the mistake is fixed in context.
// Falls back to the plain post when the span isn't found.
function renderPost(post: string, incorrect: string) {
  const idx = incorrect ? post.indexOf(incorrect) : -1;
  if (idx < 0) {
    return <Text whiteSpace="pre-wrap">{post}</Text>;
  }
  return (
    <Text whiteSpace="pre-wrap">
      {post.slice(0, idx)}
      <Text as="span" color="red.600" bg="red.100" _dark={{ color: "red.200", bg: "red.900" }} fontWeight="bold" textDecoration="underline">
        {post.slice(idx, idx + incorrect.length)}
      </Text>
      {post.slice(idx + incorrect.length)}
    </Text>
  );
}

export default function GrammarQuizPage() {
  const router = useRouter();
  const cards = useGrammarStore((s) => s.cards);
  const currentIndex = useGrammarStore((s) => s.currentIndex);
  const results = useGrammarStore((s) => s.results);
  const submitResult = useGrammarStore((s) => s.submitResult);
  const nextCard = useGrammarStore((s) => s.nextCard);
  const reset = useGrammarStore((s) => s.reset);

  const [answer, setAnswer] = useState("");
  const [feedback, setFeedback] = useState<GrammarResult | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const card = cards[currentIndex];

  // If the session wasn't seeded (e.g. direct navigation), go back to the hub.
  useEffect(() => {
    if (cards.length === 0) router.replace("/quiz");
  }, [cards.length, router]);

  useEffect(() => {
    if (!feedback) inputRef.current?.focus();
  }, [feedback, currentIndex]);

  const handleSubmit = async (skipped: boolean) => {
    if (!card || submitting) return;
    setSubmitting(true);
    try {
      const res = await quizClient.submitGrammarAnswer({
        notebookId: card.notebookId,
        cardId: card.cardId,
        answer: skipped ? "" : answer,
        isSkipped: skipped,
      });
      const result: GrammarResult = {
        cardId: card.cardId,
        sentence: card.sentence,
        incorrect: card.incorrect,
        category: card.category,
        answer: skipped ? "" : answer,
        correct: res.correct,
        correctAnswer: res.correctAnswer,
        reason: res.reason,
        nextReviewDate: res.nextReviewDate,
      };
      submitResult(result);
      setFeedback(result);
    } finally {
      setSubmitting(false);
    }
  };

  const handleNext = () => {
    setFeedback(null);
    setAnswer("");
    nextCard();
  };

  const handleDone = () => {
    const correct = results.filter((r) => r.correct).length;
    reset();
    router.push(`/quiz?grammarDone=${correct}/${results.length}`);
  };

  if (cards.length === 0) {
    return (
      <Box maxW="sm" mx="auto" p={4} textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  // Session complete.
  if (currentIndex >= cards.length) {
    const correct = results.filter((r) => r.correct).length;
    return (
      <Box maxW="sm" mx="auto" p={4}>
        <Heading size="md" textAlign="center" mb={4}>Grammar quiz complete</Heading>
        <Text textAlign="center" fontSize="lg" mb={6}>
          {correct} / {results.length} correct
        </Text>
        <VStack align="stretch" gap={2} mb={6}>
          {results.map((r, i) => (
            <Box
              key={`${r.cardId}-${i}`}
              p={3}
              borderWidth="1px"
              borderRadius="md"
              borderColor={r.correct ? "green.300" : "red.300"}
              _dark={{ borderColor: r.correct ? "green.600" : "red.600" }}
            >
              <Text fontSize="xs" color="gray.500">{r.category}</Text>
              <Text fontSize="sm">
                <Text as="span" textDecoration="line-through" color="red.500">{r.incorrect}</Text>
                {" → "}
                <Text as="span" color="green.600" _dark={{ color: "green.300" }}>{r.correctAnswer}</Text>
              </Text>
            </Box>
          ))}
        </VStack>
        <Button colorPalette="blue" w="full" onClick={handleDone}>Done</Button>
      </Box>
    );
  }

  return (
    <Box maxW="sm" mx="auto" p={4} minH="100vh">
      <Box mb={2}>
        <Link href="/quiz">
          <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="xs">&lt; Quiz</Text>
        </Link>
      </Box>
      <Text fontSize="xs" color="gray.500" mb={3}>
        {currentIndex + 1} / {cards.length}
      </Text>

      <Box p={4} bg="white" _dark={{ bg: "gray.800" }} borderWidth="1px" borderColor="gray.200" borderRadius="lg" mb={4}>
        <Box display="flex" justifyContent="space-between" mb={2}>
          <Text fontSize="xs" color="gray.500">{card.category}</Text>
          {card.line > 0 && <Text fontSize="xs" color="gray.500">line {card.line}</Text>}
        </Box>
        <Box maxH="45vh" overflowY="auto" fontSize="sm">
          {renderPost(card.sentence, card.incorrect)}
        </Box>
      </Box>

      {feedback ? (
        <VStack align="stretch" gap={3}>
          <Box
            p={3}
            borderRadius="md"
            bg={feedback.correct ? "green.50" : "red.50"}
            _dark={{ bg: feedback.correct ? "green.900" : "red.900" }}
          >
            <Text fontWeight="bold" color={feedback.correct ? "green.700" : "red.700"} _dark={{ color: feedback.correct ? "green.200" : "red.200" }}>
              {feedback.correct ? "Correct" : "Not quite"}
            </Text>
            <Text fontSize="sm" mt={1}>
              <Text as="span" fontWeight="medium">Correction:</Text> {feedback.correctAnswer}
            </Text>
            {feedback.reason && <Text fontSize="sm" mt={1} color="gray.600" _dark={{ color: "gray.300" }}>{feedback.reason}</Text>}
          </Box>
          <Button colorPalette="blue" w="full" onClick={handleNext} size="lg">
            {currentIndex + 1 >= cards.length ? "See results" : "Next"}
          </Button>
        </VStack>
      ) : (
        <AnswerInput
          ref={inputRef}
          label="Type the correction"
          value={answer}
          onChange={setAnswer}
          onSubmit={() => handleSubmit(false)}
          onSkip={() => handleSubmit(true)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && answer.trim()) handleSubmit(false);
          }}
          placeholder="Type just the corrected words for the highlighted part"
        />
      )}
    </Box>
  );
}
