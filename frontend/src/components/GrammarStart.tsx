"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Checkbox, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";
import { useGrammarStore } from "@/store/grammarStore";

// GrammarStart is the Grammar tab's content: pick journal notebooks and start a
// correction quiz over their annotated mistakes. It renders inline under the
// Quiz hub's tab row, mirroring RelearnStart.
export default function GrammarStart() {
  const router = useRouter();
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const seedCards = useGrammarStore((s) => s.seedCards);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) => setNotebooks((res.notebooks ?? []).filter((n) => n.kind === "Journal")))
      .catch(() => setError("Failed to load journals"))
      .finally(() => setLoading(false));
  }, []);

  const totalDue = useMemo(
    () =>
      notebooks
        .filter((n) => selectedIds.has(n.notebookId))
        .reduce((sum, n) => sum + n.grammarReviewCount, 0),
    [notebooks, selectedIds],
  );

  const toggle = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleStart = async () => {
    if (selectedIds.size === 0) return;
    setStarting(true);
    try {
      const res = await quizClient.startGrammarQuiz({ notebookIds: Array.from(selectedIds) });
      seedCards(
        (res.cards ?? []).map((c) => ({
          notebookId: c.notebookId,
          cardId: c.cardId,
          entryId: c.entryId,
          sentence: c.sentence,
          incorrect: c.incorrect,
          category: c.category,
          note: c.note,
          status: c.status,
        })),
      );
      router.push("/quiz/grammar");
    } catch {
      setError("Failed to start the grammar quiz.");
    } finally {
      setStarting(false);
    }
  };

  if (loading) {
    return (
      <Box textAlign="center" py={6}>
        <Spinner size="sm" />
      </Box>
    );
  }

  return (
    <Box p={4}>
      <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }} mb={4}>
        Fix the grammar mistakes from your journal. Each correction is graded and
        scheduled for review like the other quizzes.
      </Text>

      {notebooks.length === 0 ? (
        <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
          No journal notebooks found. Add one under your configured journal
          directory to start.
        </Text>
      ) : (
        <VStack align="stretch" gap={3}>
          {notebooks.map((n) => (
            <Checkbox.Root
              key={n.notebookId}
              checked={selectedIds.has(n.notebookId)}
              onCheckedChange={() => toggle(n.notebookId)}
            >
              <Checkbox.HiddenInput />
              <Checkbox.Control />
              <Checkbox.Label flex="1">
                <Box display="flex" justifyContent="space-between" w="full">
                  <Text>{n.name}</Text>
                  <Text color="gray.500" fontSize="sm">{n.grammarReviewCount}</Text>
                </Box>
              </Checkbox.Label>
            </Checkbox.Root>
          ))}

          <Text fontWeight="bold" textAlign="center">
            {totalDue} mistakes due for review
          </Text>

          <Button
            colorPalette="blue"
            w="full"
            disabled={selectedIds.size === 0 || starting}
            onClick={handleStart}
          >
            {starting ? <Spinner size="sm" /> : "Start"}
          </Button>
        </VStack>
      )}

      {error && <Text color="red.500" mt={2}>{error}</Text>}
    </Box>
  );
}
