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
import {
  quizClient,
  EtymologyQuizMode,
  type NotebookSummary,
} from "@/lib/client";
import { useQuizStore, type QuizType } from "@/store/quizStore";

type EtymologyMode = "breakdown" | "assembly" | "freeform";

const modeTitles: Record<EtymologyMode, string> = {
  breakdown: "Breakdown Quiz",
  assembly: "Assembly Quiz",
  freeform: "Etymology Freeform",
};

function EtymologyQuizStartContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const mode = (searchParams.get("mode") as EtymologyMode) || "breakdown";

  const setQuizType = useQuizStore((s) => s.setQuizType);
  const setEtymologyCards = useQuizStore((s) => s.setEtymologyCards);
  const setEtymologyFreeformExpressions = useQuizStore(
    (s) => s.setEtymologyFreeformExpressions,
  );
  const setEtymologyFreeformNextReviewDates = useQuizStore(
    (s) => s.setEtymologyFreeformNextReviewDates,
  );

  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [includeUnstudied, setIncludeUnstudied] = useState(false);
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

  const etymologyNotebooks = notebooks.filter((n) => n.kind === "Etymology");
  const definitionNotebooks = notebooks.filter(
    (n) => n.kind !== "Etymology" && n.etymologyReviewCount > 0,
  );

  const allSelected =
    definitionNotebooks.length > 0 &&
    definitionNotebooks.every((n) => selectedIds.has(n.notebookId));

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
      setSelectedIds(new Set(definitionNotebooks.map((n) => n.notebookId)));
    }
  };

  const totalDue = definitionNotebooks
    .filter((n) => selectedIds.has(n.notebookId))
    .reduce((sum, n) => sum + n.etymologyReviewCount, 0);

  const handleStart = async () => {
    setStarting(true);
    try {
      const definitionIds = Array.from(selectedIds);
      const etymologyIds = etymologyNotebooks.map((n) => n.notebookId);

      if (mode === "freeform") {
        setQuizType("etymology-freeform" as QuizType);
        const res = await quizClient.startEtymologyFreeformQuiz({
          etymologyNotebookIds: etymologyIds,
          definitionNotebookIds: definitionIds,
        });
        setEtymologyFreeformExpressions(res.expressions ?? []);
        setEtymologyFreeformNextReviewDates(res.nextReviewDates ?? {});
        router.push("/quiz/etymology-freeform");
      } else {
        const quizMode =
          mode === "breakdown"
            ? EtymologyQuizMode.BREAKDOWN
            : EtymologyQuizMode.ASSEMBLY;
        const storeType =
          mode === "breakdown"
            ? ("etymology-breakdown" as QuizType)
            : ("etymology-assembly" as QuizType);
        setQuizType(storeType);
        const res = await quizClient.startEtymologyQuiz({
          etymologyNotebookIds: etymologyIds,
          definitionNotebookIds: definitionIds,
          mode: quizMode,
          includeUnstudied,
        });
        const cards = (res.cards ?? []).map((c) => ({
          cardId: c.cardId,
          expression: c.expression,
          meaning: c.meaning,
          originParts: c.originParts.map((p) => ({
            origin: p.origin,
            type: p.type,
            language: p.language,
            meaning: p.meaning,
          })),
          notebookName: c.notebookName,
        }));
        setEtymologyCards(cards);
        router.push(
          mode === "breakdown"
            ? "/quiz/etymology-breakdown"
            : "/quiz/etymology-assembly",
        );
      }
    } finally {
      setStarting(false);
    }
  };

  const title = modeTitles[mode] || "Etymology Quiz";
  const showNotebookSelection = mode !== "freeform";

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
            <Text fontWeight="medium" fontSize="sm" mb={1}>
              Select notebooks
            </Text>
            <Text fontSize="xs" color="gray.500" mb={2}>
              Only notebooks with etymology data are shown
            </Text>

            <VStack align="stretch" gap={3}>
              <Checkbox.Root checked={allSelected} onCheckedChange={toggleAll}>
                <Checkbox.HiddenInput />
                <Checkbox.Control />
                <Checkbox.Label fontWeight="bold">
                  All notebooks
                </Checkbox.Label>
              </Checkbox.Root>

              {definitionNotebooks.map((notebook) => (
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
                        {notebook.etymologyReviewCount} words
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

            <Text mt={4} fontWeight="bold" textAlign="center">
              {totalDue} words due for review
            </Text>
          </>
        )}

        {mode === "freeform" && (
          <Text mt={4} color="gray.600" textAlign="center">
            Etymology freeform quiz. Select notebooks with etymology data.
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

export default function EtymologyQuizStartPage() {
  return (
    <Suspense
      fallback={
        <Box p={4} maxW="sm" mx="auto" textAlign="center">
          <Spinner size="lg" />
        </Box>
      }
    >
      <EtymologyQuizStartContent />
    </Suspense>
  );
}
