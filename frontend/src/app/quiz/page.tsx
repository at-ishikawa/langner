"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
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
import { quizClient, EtymologyQuizMode, type NotebookSummary } from "@/lib/client";
import { useQuizStore, type QuizType } from "@/store/quizStore";

const quizTypes: { value: QuizType; label: string; description: string }[] = [
  { value: "standard", label: "Standard", description: "See word -> Type meaning" },
  { value: "reverse", label: "Reverse", description: "See meaning -> Type word" },
  { value: "freeform", label: "Freeform", description: "Type any word + meaning" },
  { value: "etymology-breakdown", label: "Etymology", description: "Practice word origins and roots" },
];

type EtymologyMode = "breakdown" | "assembly";

export default function QuizStartPage() {
  const router = useRouter();
  const setFlashcards = useQuizStore((s) => s.setFlashcards);
  const setReverseFlashcards = useQuizStore((s) => s.setReverseFlashcards);
  const setWordCount = useQuizStore((s) => s.setWordCount);
  const setFreeformExpressions = useQuizStore((s) => s.setFreeformExpressions);
  const setFreeformNextReviewDates = useQuizStore((s) => s.setFreeformNextReviewDates);
  const setQuizType = useQuizStore((s) => s.setQuizType);
  const setEtymologyCards = useQuizStore((s) => s.setEtymologyCards);
  const setEtymologyFreeformExpressions = useQuizStore((s) => s.setEtymologyFreeformExpressions);
  const setEtymologyFreeformNextReviewDates = useQuizStore((s) => s.setEtymologyFreeformNextReviewDates);

  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [includeUnstudied, setIncludeUnstudied] = useState(false);
  const [listMissingContext, setListMissingContext] = useState(false);
  const [quizType, setQuizTypeLocal] = useState<QuizType>("standard");
  const [etymologyMode, setEtymologyMode] = useState<EtymologyMode>("breakdown");
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

  const isEtymology = quizType === "etymology-breakdown";

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
    .reduce((sum, n) => {
      if (isEtymology) return sum + n.etymologyReviewCount;
      if (quizType === "reverse") return sum + n.reverseReviewCount;
      return sum + n.reviewCount;
    }, 0);

  const handleQuizTypeChange = (type: QuizType) => {
    setQuizTypeLocal(type);
    setQuizType(type);
  };

  const handleStart = async () => {
    setStarting(true);
    try {
      if (quizType === "standard") {
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
      } else if (quizType === "reverse") {
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
      } else if (quizType === "freeform") {
        setQuizType("freeform");
        const res = await quizClient.startFreeformQuiz({});
        setWordCount(res.wordCount);
        setFreeformExpressions(res.expressions ?? []);
        setFreeformNextReviewDates(res.expressionNextReviewDate ?? {});
        router.push("/quiz/freeform");
      } else if (isEtymology) {
        const selectedNotebookIds = Array.from(selectedIds);
        if (etymologyMode === "breakdown" || etymologyMode === "assembly") {
          const mode = etymologyMode === "breakdown"
            ? EtymologyQuizMode.BREAKDOWN
            : EtymologyQuizMode.ASSEMBLY;
          const storeType = etymologyMode === "breakdown"
            ? "etymology-breakdown" as QuizType
            : "etymology-assembly" as QuizType;
          setQuizType(storeType);
          const res = await quizClient.startEtymologyQuiz({
            etymologyNotebookIds: selectedNotebookIds,
            definitionNotebookIds: selectedNotebookIds,
            mode,
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
          router.push(etymologyMode === "breakdown" ? "/quiz/etymology-breakdown" : "/quiz/etymology-assembly");
        }
      }
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

  if (error) {
    return (
      <Box p={4} maxW="md" mx="auto">
        <Text color="red.500">{error}</Text>
      </Box>
    );
  }

  const showNotebookSelection = quizType !== "freeform";

  return (
    <Box p={4} maxW="md" mx="auto">
      <Box mb={2}>
        <Link href="/">
          <Text color="blue.600" fontSize="sm" _dark={{ color: "blue.300" }}>&larr; Back</Text>
        </Link>
      </Box>
      <Heading size="lg" mb={4}>Quiz</Heading>

      <Text fontWeight="medium" mb={2}>
        Select quiz type
      </Text>

      <VStack align="stretch" gap={2} mb={4}>
        {quizTypes.map((type) => (
          <Box
            key={type.value}
            p={3}
            borderWidth="2px"
            borderRadius="md"
            cursor="pointer"
            onClick={() => handleQuizTypeChange(type.value)}
            bg={quizType === type.value ? "blue.50" : "white"}
            borderColor={quizType === type.value ? "blue.500" : "gray.200"}
            _dark={{
              bg: quizType === type.value ? "blue.900" : "gray.800",
              borderColor: quizType === type.value ? "blue.400" : "gray.600",
            }}
          >
            <Text fontWeight="medium">{type.label}</Text>
            <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>
              {type.description}
            </Text>
          </Box>
        ))}
      </VStack>

      {isEtymology && (
        <Box mb={4}>
          <Text fontWeight="medium" mb={2}>Quiz mode</Text>
          <VStack align="stretch" gap={2}>
            {[
              { value: "breakdown" as EtymologyMode, label: "Breakdown", desc: "See word -> identify origins" },
              { value: "assembly" as EtymologyMode, label: "Assembly", desc: "See origins -> guess the word" },
            ].map((m) => (
              <Box
                key={m.value}
                p={3}
                borderWidth="2px"
                borderRadius="md"
                cursor="pointer"
                onClick={() => setEtymologyMode(m.value)}
                bg={etymologyMode === m.value ? "blue.50" : "white"}
                borderColor={etymologyMode === m.value ? "blue.500" : "gray.200"}
                _dark={{
                  bg: etymologyMode === m.value ? "blue.900" : "gray.800",
                  borderColor: etymologyMode === m.value ? "blue.400" : "gray.600",
                }}
              >
                <Text fontWeight="medium">{m.label}</Text>
                <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>{m.desc}</Text>
              </Box>
            ))}
          </VStack>
        </Box>
      )}

      {showNotebookSelection && (
        <>
          <Text fontWeight="medium" mb={2}>
            Select notebooks
          </Text>

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
                <Checkbox.Label flex="1">
                  <Box display="flex" justifyContent="space-between" w="full">
                    <Text>{notebook.name}</Text>
                    <Text>
                      {isEtymology
                        ? notebook.etymologyReviewCount
                        : quizType === "reverse"
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

          {quizType === "reverse" && (
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

          <Text mt={4} fontWeight="bold">
            {totalDue} words due for review
          </Text>
        </>
      )}

      {quizType === "freeform" && (
        <Text mt={4} color="gray.600">
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
  );
}
