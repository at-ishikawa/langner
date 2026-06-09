"use client";

import { useState } from "react";
import Link from "next/link";
import { Box, Button, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import { PatternGlyphs } from "./PatternGlyphs";
import { QuizTypeChip } from "./QuizTypeChip";
import { analyticsClient, type WrongWord, type AttemptEntry } from "@/lib/client";

function streakSummary(w: WrongWord): string {
  if (w.currentWrongStreak >= 2) {
    return `${w.currentWrongStreak} wrong in a row`;
  }
  if (w.previousCorrectStreak > 0) {
    return `after ${w.previousCorrectStreak} correct`;
  }
  return "1st wrong attempt";
}

function formatAttemptDate(dateStr: string): string {
  const [y, m, d] = dateStr.split("-").map(Number);
  if (!y || !m || !d) return dateStr;
  const dt = new Date(y, m - 1, d);
  return dt.toLocaleDateString("en-US", { month: "short", day: "2-digit" });
}

function attemptStreakLabel(a: AttemptEntry): string {
  if (a.streakBeforeWrong > 0) return `after ${a.streakBeforeWrong} wrong`;
  if (a.streakBeforeCorrect > 0) return `after ${a.streakBeforeCorrect} correct`;
  return "(first attempt)";
}

// learnHrefFor builds the deep link from a wrong word to its source page
// in the Learn section. The destination depends on the notebook kind:
// etymology origins go to the etymology hub keyed by ?origin=…, flashcards
// to the flashcard detail page keyed by ?word=…, and stories to the story
// reader keyed by ?word=…&scene=…. Unknown kinds fall back to the bare
// notebook page so the link never produces a 404.
function learnHrefFor(w: WrongWord): string {
  const id = encodeURIComponent(w.notebookId);
  const expr = encodeURIComponent(w.expression);
  switch (w.notebookKind) {
    case "etymology":
      return `/notebooks/etymology/${id}?origin=${expr}`;
    case "flashcard":
      return `/notebooks/${id}?word=${expr}`;
    case "story": {
      const scene = w.sceneTitle ? `&scene=${encodeURIComponent(w.sceneTitle)}` : "";
      return `/learn/${id}?word=${expr}${scene}`;
    }
    default:
      return `/learn/${id}`;
  }
}

export function WrongWordCard({ word }: { word: WrongWord }) {
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [attempts, setAttempts] = useState<AttemptEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Reverse and freeform quizzes prompt with the meaning; rendering the
  // meaning as the headline reproduces the original cognitive task.
  const meaningIsPrompt = word.quizType === "reverse" || word.quizType === "etymology_assembly" || word.quizType.endsWith("_freeform");

  async function toggle() {
    if (expanded) {
      setExpanded(false);
      return;
    }
    setExpanded(true);
    if (attempts !== null) return;
    setLoading(true);
    setError(null);
    try {
      const resp = await analyticsClient.getWordHistory({
        noteId: word.noteId,
        notebookId: word.notebookId,
        expression: word.expression,
        quizType: word.quizType,
      });
      setAttempts(resp.attempts);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Box
      borderWidth="1px"
      borderRadius="lg"
      p={4}
      mb={3}
      bg="white"
      _dark={{ bg: "gray.800" }}
      data-testid={`wrong-word-${word.expression}-${word.quizType}`}
    >
      <Box as="button" onClick={toggle} width="100%" textAlign="left" aria-expanded={expanded}>
        {meaningIsPrompt && word.meaning && (
          <Text fontSize="md" mb={1} data-testid="wrong-word-prompt-meaning">
            {word.meaning}
          </Text>
        )}
        <HStack mb={1}>
          <Text fontSize="lg" color="red.500" aria-hidden="true">
            ✗
          </Text>
          <Text fontWeight="bold" fontSize="lg">
            {word.expression}
          </Text>
        </HStack>
        {!meaningIsPrompt && word.meaning && (
          <Text fontSize="sm" mb={1} data-testid="wrong-word-meaning">
            {word.meaning}
          </Text>
        )}
        <Text fontSize="sm" color="fg.muted" mb={2}>
          {word.notebookTitle || word.notebookId}
          {word.sceneTitle && ` / ${word.sceneTitle}`}
        </Text>
        {word.exampleSentence && (
          <Text fontSize="sm" color="fg.muted" fontStyle="italic" mb={2} data-testid="wrong-word-example">
            “{word.exampleSentence}”
          </Text>
        )}
        <HStack justifyContent="space-between">
          <QuizTypeChip quizType={word.quizType} />
          <PatternGlyphs pattern={word.recentPattern} />
        </HStack>
        <Text fontSize="xs" color="fg.muted" fontStyle="italic" mt={2}>
          {streakSummary(word)}
        </Text>
      </Box>

      {expanded && (
        <Box mt={4} pt={4} borderTopWidth="1px">
          {loading && <Spinner size="sm" data-testid="word-history-loading" />}
          {error && (
            <Text color="red.500" fontSize="sm">
              Failed to load history: {error}
            </Text>
          )}
          {!loading && !error && attempts && (
            <VStack align="stretch" gap={2}>
              {attempts.map((a, i) => (
                <HStack key={i} fontSize="sm" justifyContent="space-between" data-testid="attempt-row">
                  <Text>{formatAttemptDate(a.date)}</Text>
                  <QuizTypeChip quizType={a.quizType} />
                  <Text color={a.result === "wrong" ? "red.500" : "green.600"}>
                    {a.result === "wrong" ? "✗ wrong" : "✓ correct"}
                  </Text>
                  <Text>Q{a.quality}</Text>
                  <Text color="fg.muted" fontSize="xs">
                    {attemptStreakLabel(a)}
                  </Text>
                </HStack>
              ))}
              {word.notebookId && (
                <Button
                  as={Link}
                  // @ts-expect-error Chakra Button + Next Link href prop
                  href={learnHrefFor(word)}
                  mt={2}
                  size="sm"
                  variant="outline"
                  data-testid="open-in-learn"
                >
                  Open in Learn
                </Button>
              )}
            </VStack>
          )}
        </Box>
      )}
    </Box>
  );
}
