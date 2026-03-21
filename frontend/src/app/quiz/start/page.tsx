"use client";

import { useEffect, useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import {
  Box,
  Button,
  Checkbox,
  Heading,
  Spinner,
  Switch,
  Text,
  VStack,
} from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";
import { useQuizStore, type QuizType } from "@/store/quizStore";

type VocabMode = "standard" | "reverse" | "freeform";

const modeTitles: Record<VocabMode, string> = {
  standard: "Standard Quiz",
  reverse: "Reverse Quiz",
  freeform: "Freeform Quiz",
};

function VocabularyQuizStartContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const mode = (searchParams.get("mode") as VocabMode) || "standard";

  const setFlashcards = useQuizStore((s) => s.setFlashcards);
  const setReverseFlashcards = useQuizStore((s) => s.setReverseFlashcards);
  const setWordCount = useQuizStore((s) => s.setWordCount);
  const setFreeformExpressions = useQuizStore((s) => s.setFreeformExpressions);
  const setFreeformNextReviewDates = useQuizStore(
    (s) => s.setFreeformNextReviewDates,
  );
  const setQuizType = useQuizStore((s) => s.setQuizType);

  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [includeUnstudied, setIncludeUnstudied] = useState(false);
  const [listMissingContext, setListMissingContext] = useState(false);
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) => {
        setNotebooks(res.notebooks ?? []);
      })
      .catch(() => setError("Failed to load notebooks"))
      .finally(() => setLoading(false));
  }, []);

  const displayedNotebooks = notebooks.filter((n) => n.kind !== "Etymology");

  const allSelected =
    displayedNotebooks.length > 0 &&
    displayedNotebooks.every((n) => selectedIds.has(n.notebookId));

  const toggleNotebook = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const toggleAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(displayedNotebooks.map((n) => n.notebookId)));
    }
  };

  const totalDue = displayedNotebooks
    .filter((n) => selectedIds.has(n.notebookId))
    .reduce((sum, n) => {
      if (mode === "reverse") return sum + n.reverseReviewCount;
      return sum + n.reviewCount;
    }, 0);

  const handleStart = async () => {
    setStarting(true);
    try {
      if (mode === "standard") {
        setQuizType("standard");
        const res = await quizClient.startQuiz({
          notebookIds: Array.from(selectedIds),
          includeUnstudied,
        });
        const flashcards = (res.flashcards ?? []).map((f) => ({
          noteId: f.noteId,
          entry: f.entry,
          examples: f.examples,
        }));
        setFlashcards(flashcards);
        router.push("/quiz/standard");
      } else if (mode === "reverse") {
        setQuizType("reverse");
        const res = await quizClient.startReverseQuiz({
          notebookIds: Array.from(selectedIds),
          listMissingContext,
        });
        const flashcards = (res.flashcards ?? []).map((f) => ({
          noteId: f.noteId,
          meaning: f.meaning,
          contexts: f.contexts,
          notebookName: f.notebookName,
          storyTitle: f.storyTitle,
          sceneTitle: f.sceneTitle,
        }));
        setReverseFlashcards(flashcards);
        router.push("/quiz/reverse");
      } else if (mode === "freeform") {
        setQuizType("freeform");
        const res = await quizClient.startFreeformQuiz({});
        setWordCount(res.wordCount);
        setFreeformExpressions(res.expressions ?? []);
        setFreeformNextReviewDates(res.expressionNextReviewDate ?? {});
        router.push("/quiz/freeform");
      }
    } finally {
      setStarting(false);
    }
  };

  const showNotebookSelection = mode !== "freeform";
  const title = modeTitles[mode] || "Quiz";

  if (loading) {
    return (
      <Box p={4} maxW="sm" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error) {
    return (
      <Box p={4} maxW="sm" mx="auto">
        <Text color="red.500">{error}</Text>
      </Box>
    );
  }

  return (
    <Box maxW="sm" mx="auto" bg="#f8f9fa" minH="100vh">
      {/* Header */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb">
        <Box px={4} pt={2}>
          <Link href="/quiz">
            <Text color="#2563eb" fontSize="xs">
              &lt; Quiz
            </Text>
          </Link>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">{title}</Heading>
        </Box>
      </Box>

      <Box p={4}>
        {showNotebookSelection && (
          <>
            <Text fontWeight="medium" fontSize="sm" mb={2}>
              Select notebooks
            </Text>

            <VStack align="stretch" gap={3}>
              <Checkbox.Root checked={allSelected} onCheckedChange={toggleAll}>
                <Checkbox.HiddenInput />
                <Checkbox.Control />
                <Checkbox.Label fontWeight="bold">
                  All notebooks
                </Checkbox.Label>
              </Checkbox.Root>

              {displayedNotebooks.map((notebook) => (
                <Checkbox.Root
                  key={notebook.notebookId}
                  checked={selectedIds.has(notebook.notebookId)}
                  onCheckedChange={() => toggleNotebook(notebook.notebookId)}
                >
                  <Checkbox.HiddenInput />
                  <Checkbox.Control />
                  <Checkbox.Label flex="1">
                    <Box
                      display="flex"
                      justifyContent="space-between"
                      w="full"
                    >
                      <Text>{notebook.name}</Text>
                      <Text color="gray.500" fontSize="sm">
                        {mode === "reverse"
                          ? notebook.reverseReviewCount
                          : notebook.reviewCount}
                      </Text>
                    </Box>
                  </Checkbox.Label>
                </Checkbox.Root>
              ))}
            </VStack>

            <Box mt={4}>
              <Switch.Root
                checked={includeUnstudied}
                onCheckedChange={(e) => setIncludeUnstudied(e.checked)}
              >
                <Switch.HiddenInput />
                <Switch.Control />
                <Switch.Label>Include unstudied words</Switch.Label>
              </Switch.Root>
            </Box>

            {mode === "reverse" && (
              <Box mt={2}>
                <Switch.Root
                  checked={listMissingContext}
                  onCheckedChange={(e) => setListMissingContext(e.checked)}
                >
                  <Switch.HiddenInput />
                  <Switch.Control />
                  <Switch.Label>List words missing context</Switch.Label>
                </Switch.Root>
              </Box>
            )}

            <Text mt={4} fontWeight="bold" textAlign="center">
              {totalDue} words due for review
            </Text>
          </>
        )}

        {mode === "freeform" && (
          <Text mt={4} color="gray.600" textAlign="center">
            {notebooks.reduce((sum, n) => sum + n.reviewCount, 0)} words
            available across all notebooks
          </Text>
        )}

        <Button
          mt={4}
          w="full"
          colorPalette="blue"
          disabled={
            (showNotebookSelection && selectedIds.size === 0) || starting
          }
          onClick={handleStart}
        >
          {starting ? <Spinner size="sm" /> : "Start"}
        </Button>
      </Box>
    </Box>
  );
}

export default function VocabularyQuizStartPage() {
  return (
    <Suspense
      fallback={
        <Box p={4} maxW="sm" mx="auto" textAlign="center">
          <Spinner size="lg" />
        </Box>
      }
    >
      <VocabularyQuizStartContent />
    </Suspense>
  );
}
