"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Checkbox, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";
import { useGrammarStore } from "@/store/grammarStore";

// GrammarStart is the Grammar tab's content: pick which journals — and which
// individual entries within them (e.g. w16, w17) — to drill, mirroring the
// vocabulary quiz's per-section narrowing.
export default function GrammarStart() {
  const router = useRouter();
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  // selected maps a journal id → the set of its selected entry titles.
  const [selected, setSelected] = useState<Map<string, Set<string>>>(new Map());
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const seedPosts = useGrammarStore((s) => s.seedPosts);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) =>
        setNotebooks(
          (res.notebooks ?? []).filter((n) => n.kind === "Grammar" && n.grammarReviewCount > 0),
        ),
      )
      .catch((e) =>
        setError(
          e instanceof Error ? `Failed to load journals: ${e.message}` : "Failed to load journals",
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  const isEntrySelected = (jid: string, title: string) => selected.get(jid)?.has(title) ?? false;

  const toggleEntry = (jid: string, title: string) => {
    setSelected((prev) => {
      const next = new Map(prev);
      const set = new Set(next.get(jid));
      if (set.has(title)) set.delete(title);
      else set.add(title);
      if (set.size === 0) next.delete(jid);
      else next.set(jid, set);
      return next;
    });
  };

  const toggleJournal = (n: NotebookSummary) => {
    const titles = (n.sections ?? []).map((s) => s.title);
    setSelected((prev) => {
      const next = new Map(prev);
      const current = next.get(n.notebookId);
      if (current && current.size === titles.length) next.delete(n.notebookId); // all → none
      else next.set(n.notebookId, new Set(titles)); // some/none → all
      return next;
    });
  };

  const totalDue = useMemo(() => {
    let sum = 0;
    for (const n of notebooks) {
      const set = selected.get(n.notebookId);
      if (!set) continue;
      for (const sec of n.sections ?? []) {
        if (set.has(sec.title)) sum += sec.grammarReviewCount;
      }
    }
    return sum;
  }, [notebooks, selected]);

  const anySelected = selected.size > 0;

  const handleStart = async () => {
    if (!anySelected) return;
    setStarting(true);
    try {
      const notebookSections = Array.from(selected.entries()).map(([notebookId, titles]) => ({
        notebookId,
        sectionTitles: Array.from(titles),
      }));
      const res = await quizClient.startGrammarQuiz({ notebookSections });
      seedPosts(res.posts ?? []);
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
        Fix the grammar mistakes from your journal. Pick whole journals or just
        the entries you want to drill.
      </Text>

      {notebooks.length === 0 ? (
        <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
          No journal entries are due for review right now.
        </Text>
      ) : (
        <VStack align="stretch" gap={4}>
          {notebooks.map((n) => {
            const titles = (n.sections ?? []).map((s) => s.title);
            const selCount = selected.get(n.notebookId)?.size ?? 0;
            return (
              <Box key={n.notebookId}>
                {/* Journal header — toggles all its entries. */}
                <Checkbox.Root
                  checked={selCount > 0 && selCount === titles.length ? true : selCount > 0 ? "indeterminate" : false}
                  onCheckedChange={() => toggleJournal(n)}
                >
                  <Checkbox.HiddenInput />
                  <Checkbox.Control />
                  <Checkbox.Label flex="1">
                    <Box display="flex" justifyContent="space-between" w="full">
                      <Text fontWeight="semibold">{n.name}</Text>
                      <Text color="gray.500" fontSize="sm">{n.grammarReviewCount}</Text>
                    </Box>
                  </Checkbox.Label>
                </Checkbox.Root>

                {/* Entries within the journal. */}
                <VStack align="stretch" gap={1} pl={6} mt={1}>
                  {(n.sections ?? []).map((sec) => (
                    <Checkbox.Root
                      key={sec.title}
                      checked={isEntrySelected(n.notebookId, sec.title)}
                      onCheckedChange={() => toggleEntry(n.notebookId, sec.title)}
                    >
                      <Checkbox.HiddenInput />
                      <Checkbox.Control />
                      <Checkbox.Label flex="1">
                        <Box display="flex" justifyContent="space-between" w="full">
                          <Text fontSize="sm">{sec.title}</Text>
                          <Text color="gray.500" fontSize="sm">{sec.grammarReviewCount}</Text>
                        </Box>
                      </Checkbox.Label>
                    </Checkbox.Root>
                  ))}
                </VStack>
              </Box>
            );
          })}

          <Text fontWeight="bold" textAlign="center">
            {totalDue} mistakes due for review
          </Text>

          <Button colorPalette="blue" w="full" disabled={!anySelected || starting} onClick={handleStart}>
            {starting ? <Spinner size="sm" /> : "Start"}
          </Button>
        </VStack>
      )}

      {error && <Text color="red.500" mt={2}>{error}</Text>}
    </Box>
  );
}
