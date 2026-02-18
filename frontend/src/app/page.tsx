"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
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
import {
  getQuizOptions,
  startQuiz,
  type NotebookSummary,
} from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";

export default function QuizStartPage() {
  const router = useRouter();
  const setFlashcards = useQuizStore((s) => s.setFlashcards);

  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [includeUnstudied, setIncludeUnstudied] = useState(false);
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);

  useEffect(() => {
    getQuizOptions()
      .then((res) => {
        setNotebooks(res.notebooks ?? []);
      })
      .finally(() => setLoading(false));
  }, []);

  const allSelected =
    notebooks.length > 0 && selectedIds.size === notebooks.length;

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
      setSelectedIds(new Set(notebooks.map((n) => n.notebookId)));
    }
  };

  const totalDue = notebooks
    .filter((n) => selectedIds.has(n.notebookId))
    .reduce((sum, n) => sum + n.reviewCount, 0);

  const handleStart = async () => {
    setStarting(true);
    try {
      const res = await startQuiz({
        notebookIds: Array.from(selectedIds),
        includeUnstudied,
      });
      const flashcards = (res.flashcards ?? []).map((f) => ({
        noteId: BigInt(f.noteId),
        entry: f.entry,
        examples: f.examples ?? [],
      }));
      setFlashcards(flashcards);
      router.push("/quiz");
    } finally {
      setStarting(false);
    }
  };

  if (loading) {
    return (
      <Box p={4} maxW="md" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  return (
    <Box p={4} maxW="md" mx="auto">
      <Heading size="lg" mb={4}>
        Langner Quiz
      </Heading>

      <VStack align="stretch" gap={3}>
        <Checkbox.Root
          checked={allSelected}
          onCheckedChange={toggleAll}
        >
          <Checkbox.HiddenInput />
          <Checkbox.Control />
          <Checkbox.Label fontWeight="bold">All notebooks</Checkbox.Label>
        </Checkbox.Root>

        {notebooks.map((notebook) => (
          <Checkbox.Root
            key={notebook.notebookId}
            checked={selectedIds.has(notebook.notebookId)}
            onCheckedChange={() => toggleNotebook(notebook.notebookId)}
          >
            <Checkbox.HiddenInput />
            <Checkbox.Control />
            <Checkbox.Label>
              {notebook.name} ({notebook.reviewCount})
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

      <Text mt={4} fontWeight="bold">
        {totalDue} words due
      </Text>

      <Button
        mt={4}
        w="full"
        colorPalette="blue"
        disabled={selectedIds.size === 0 || starting}
        onClick={handleStart}
      >
        {starting ? <Spinner size="sm" /> : "Start"}
      </Button>
    </Box>
  );
}
