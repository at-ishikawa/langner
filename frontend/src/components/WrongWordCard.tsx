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

export function WrongWordCard({ word }: { word: WrongWord }) {
  const [expanded, setExpanded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [attempts, setAttempts] = useState<AttemptEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

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
        <HStack mb={1}>
          <Text fontSize="lg" color="red.500" aria-hidden="true">
            ✗
          </Text>
          <Text fontWeight="bold" fontSize="lg">
            {word.expression}
          </Text>
        </HStack>
        <Text fontSize="sm" color="fg.muted" mb={2}>
          {word.notebookTitle || word.notebookId}
          {word.sceneTitle && ` / ${word.sceneTitle}`}
        </Text>
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
                  href={`/learn/${word.notebookId}`}
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
