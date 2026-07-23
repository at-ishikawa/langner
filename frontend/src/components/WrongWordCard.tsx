"use client";

import { useState } from "react";
import Link from "next/link";
import { Badge, Box, Button, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import { PatternGlyphs } from "./PatternGlyphs";
import { QuizTypeChip } from "./QuizTypeChip";
import { analyticsClient, type WrongWord, type AttemptEntry, type RelatedGroup } from "@/lib/client";

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

// summariseSceneTitle keeps the breadcrumb short. Story-style notebooks
// (Speak English Like an American, Friends) declare the scene title as
// the lesson's multi-paragraph plot summary; rendering it raw produced a
// 200-character breadcrumb that visually competed with the meaning and
// looked like a (wrong) example sentence. This collapses it to a single
// short line — first sentence-ish — so the card stays scannable. Short
// titles (the typical flashcard / story case) pass through untouched.
function summariseSceneTitle(title: string): string {
  const firstLine = title.split(/\r?\n/).find((line) => line.trim().length > 0) ?? "";
  const trimmed = firstLine.trim();
  if (trimmed.length <= 80) return trimmed;
  return `${trimmed.slice(0, 77).trimEnd()}…`;
}

// chipLabelForGroup compresses one RelatedGroup into a chip body the
// collapsed card can show without taking a second visual row: the kind
// becomes a prefix tag ("antonym:") and the first 1-2 members follow,
// with "+N more" tacked on when the list is longer. Definitions-book
// "concept" groups don't need the kind prefix because the meaning label
// is already self-explanatory ("same sense: gaucherie").
function chipLabelForGroup(g: RelatedGroup): string {
  const PREVIEW = 2;
  const head = g.members.slice(0, PREVIEW).join(", ");
  const rest = g.members.length > PREVIEW ? ` +${g.members.length - PREVIEW}` : "";
  const tag =
    g.kind === "concept"
      ? "same sense"
      : g.kind === "origin_family"
        ? "family"
        : g.kind;
  return `${tag}: ${head}${rest}`;
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
        senseId: word.senseId,
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
          {word.skipped && (
            <Badge
              colorPalette="gray"
              variant="subtle"
              data-testid="excluded-badge"
              title="You skipped this word — it won't surface in future quizzes of this type until you clear the skip."
            >
              Excluded
            </Badge>
          )}
        </HStack>
        {!meaningIsPrompt && word.meaning && (
          <Text fontSize="sm" mb={1} data-testid="wrong-word-meaning">
            {word.meaning}
          </Text>
        )}
        <Text fontSize="sm" color="fg.muted" mb={2} data-testid="wrong-word-breadcrumb">
          {word.notebookTitle || word.notebookId}
          {word.sceneTitle && ` / ${summariseSceneTitle(word.sceneTitle)}`}
        </Text>
        {word.exampleSentence && (
          <Text fontSize="sm" color="fg.muted" fontStyle="italic" mb={2} data-testid="wrong-word-example">
            “{word.exampleSentence}”
          </Text>
        )}
        {(word.relatedGroups?.length ?? 0) > 0 && (
          <HStack gap={1} mb={2} flexWrap="wrap" data-testid="related-chip-preview">
            {(word.relatedGroups ?? []).slice(0, 2).map((g, i) => (
              <Badge
                key={`${g.kind}-${i}`}
                colorPalette="purple"
                variant="subtle"
                title={g.label || g.kind}
                data-testid={`related-chip-${g.kind}`}
              >
                {chipLabelForGroup(g)}
              </Badge>
            ))}
          </HStack>
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
          {(word.relatedGroups?.length ?? 0) > 0 && (
            <VStack
              align="stretch"
              gap={2}
              mb={3}
              data-testid="related-groups-expanded"
            >
              <Text fontSize="sm" fontWeight="semibold">
                Related words
              </Text>
              {(word.relatedGroups ?? []).map((g, i) => (
                <Box key={`${g.kind}-${i}`} data-testid={`related-group-${g.kind}`}>
                  <Text fontSize="xs" color="fg.muted" textTransform="uppercase">
                    {g.kind === "concept"
                      ? "Same sense"
                      : g.kind === "origin_family"
                        ? "Same origin family"
                        : g.kind}
                    {g.label ? ` · ${g.label}` : ""}
                  </Text>
                  <Text fontSize="sm">{g.members.join(" · ")}</Text>
                </Box>
              ))}
            </VStack>
          )}
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
