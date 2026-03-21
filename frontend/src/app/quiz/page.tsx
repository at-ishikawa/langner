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
import {
  quizClient,
  EtymologyQuizMode,
  type NotebookSummary,
} from "@/lib/client";
import { useQuizStore, type QuizType } from "@/store/quizStore";

type Tab = "vocabulary" | "etymology";
type VocabMode = "standard" | "reverse" | "freeform";
type EtyMode = "breakdown" | "assembly" | "freeform";

const vocabularyModes: { key: VocabMode; title: string; description: string }[] = [
  { key: "standard", title: "Standard", description: "See a word, type its meaning" },
  { key: "reverse", title: "Reverse", description: "See a meaning, type the word" },
  { key: "freeform", title: "Freeform", description: "Type any word and its meaning" },
];

const etymologyModes: { key: EtyMode; title: string; description: string }[] = [
  { key: "breakdown", title: "Breakdown", description: "See a word, identify its origins and meanings" },
  { key: "assembly", title: "Assembly", description: "See origins and meanings, type the word" },
  { key: "freeform", title: "Freeform", description: "Type any word and break down its origins" },
];

export default function QuizHubPage() {
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("vocabulary");
  const [selectedVocabMode, setSelectedVocabMode] = useState<VocabMode | null>(null);
  const [selectedEtyMode, setSelectedEtyMode] = useState<EtyMode | null>(null);

  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [includeUnstudied, setIncludeUnstudied] = useState(false);
  const [listMissingContext, setListMissingContext] = useState(false);
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const setFlashcards = useQuizStore((s) => s.setFlashcards);
  const setReverseFlashcards = useQuizStore((s) => s.setReverseFlashcards);
  const setWordCount = useQuizStore((s) => s.setWordCount);
  const setFreeformExpressions = useQuizStore((s) => s.setFreeformExpressions);
  const setFreeformNextReviewDates = useQuizStore((s) => s.setFreeformNextReviewDates);
  const setQuizType = useQuizStore((s) => s.setQuizType);
  const setEtymologyCards = useQuizStore((s) => s.setEtymologyCards);
  const setEtymologyFreeformExpressions = useQuizStore((s) => s.setEtymologyFreeformExpressions);
  const setEtymologyFreeformNextReviewDates = useQuizStore((s) => s.setEtymologyFreeformNextReviewDates);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) => setNotebooks(res.notebooks ?? []))
      .catch(() => setError("Failed to load notebooks"))
      .finally(() => setLoading(false));
  }, []);

  const selectedMode = tab === "vocabulary" ? selectedVocabMode : selectedEtyMode;

  // Notebook lists
  const vocabNotebooks = notebooks.filter((n) => n.kind !== "Etymology");
  const etymologySourceNotebooks = notebooks.filter((n) => n.kind === "Etymology");
  const etymologyDefNotebooks = notebooks.filter(
    (n) => n.kind !== "Etymology" && n.etymologyReviewCount > 0,
  );
  const displayedNotebooks = tab === "vocabulary" ? vocabNotebooks : etymologyDefNotebooks;

  const allSelected =
    displayedNotebooks.length > 0 &&
    displayedNotebooks.every((n) => selectedIds.has(n.notebookId));

  const toggleNotebook = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleAll = () => {
    if (allSelected) setSelectedIds(new Set());
    else setSelectedIds(new Set(displayedNotebooks.map((n) => n.notebookId)));
  };

  const totalDue = displayedNotebooks
    .filter((n) => selectedIds.has(n.notebookId))
    .reduce((sum, n) => {
      if (tab === "etymology") return sum + n.etymologyReviewCount;
      if (selectedVocabMode === "reverse") return sum + n.reverseReviewCount;
      return sum + n.reviewCount;
    }, 0);

  const handleTabChange = (newTab: Tab) => {
    setTab(newTab);
    setSelectedIds(new Set());
  };

  const handleModeSelect = (mode: string) => {
    if (tab === "vocabulary") {
      const m = mode as VocabMode;
      setSelectedVocabMode(selectedVocabMode === m ? null : m);
    } else {
      const m = mode as EtyMode;
      setSelectedEtyMode(selectedEtyMode === m ? null : m);
    }
    setSelectedIds(new Set());
  };

  const showNotebookSelection =
    selectedMode !== null &&
    !(tab === "vocabulary" && selectedVocabMode === "freeform");

  const handleStart = async () => {
    setStarting(true);
    try {
      if (tab === "vocabulary") {
        if (selectedVocabMode === "standard") {
          setQuizType("standard");
          const res = await quizClient.startQuiz({
            notebookIds: Array.from(selectedIds),
            includeUnstudied,
          });
          setFlashcards(
            (res.flashcards ?? []).map((f) => ({
              noteId: f.noteId, entry: f.entry, examples: f.examples,
            })),
          );
          router.push("/quiz/standard");
        } else if (selectedVocabMode === "reverse") {
          setQuizType("reverse");
          const res = await quizClient.startReverseQuiz({
            notebookIds: Array.from(selectedIds),
            listMissingContext,
          });
          setReverseFlashcards(
            (res.flashcards ?? []).map((f) => ({
              noteId: f.noteId, meaning: f.meaning, contexts: f.contexts,
              notebookName: f.notebookName, storyTitle: f.storyTitle, sceneTitle: f.sceneTitle,
            })),
          );
          router.push("/quiz/reverse");
        } else if (selectedVocabMode === "freeform") {
          setQuizType("freeform");
          const res = await quizClient.startFreeformQuiz({});
          setWordCount(res.wordCount);
          setFreeformExpressions(res.expressions ?? []);
          setFreeformNextReviewDates(res.expressionNextReviewDate ?? {});
          router.push("/quiz/freeform");
        }
      } else {
        const definitionIds = Array.from(selectedIds);
        const etymologyIds = etymologySourceNotebooks.map((n) => n.notebookId);

        if (selectedEtyMode === "freeform") {
          setQuizType("etymology-freeform" as QuizType);
          const res = await quizClient.startEtymologyFreeformQuiz({
            etymologyNotebookIds: etymologyIds,
            definitionNotebookIds: definitionIds,
          });
          setEtymologyFreeformExpressions(res.expressions ?? []);
          setEtymologyFreeformNextReviewDates(res.nextReviewDates ?? {});
          router.push("/quiz/etymology-freeform");
        } else {
          const quizMode = selectedEtyMode === "breakdown"
            ? EtymologyQuizMode.BREAKDOWN : EtymologyQuizMode.ASSEMBLY;
          const storeType = selectedEtyMode === "breakdown"
            ? "etymology-breakdown" as QuizType : "etymology-assembly" as QuizType;
          setQuizType(storeType);
          const res = await quizClient.startEtymologyQuiz({
            etymologyNotebookIds: etymologyIds,
            definitionNotebookIds: definitionIds,
            mode: quizMode,
            includeUnstudied,
          });
          setEtymologyCards(
            (res.cards ?? []).map((c) => ({
              cardId: c.cardId, expression: c.expression, meaning: c.meaning,
              originParts: c.originParts.map((p) => ({
                origin: p.origin, type: p.type, language: p.language, meaning: p.meaning,
              })),
              notebookName: c.notebookName,
            })),
          );
          router.push(selectedEtyMode === "breakdown" ? "/quiz/etymology-breakdown" : "/quiz/etymology-assembly");
        }
      }
    } finally {
      setStarting(false);
    }
  };

  const modes = tab === "vocabulary" ? vocabularyModes : etymologyModes;

  if (loading) {
    return (
      <Box maxW="sm" mx="auto" p={4} textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  return (
    <Box maxW="sm" mx="auto" bg="#f8f9fa" minH="100vh">
      {/* Header */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb">
        <Box px={4} pt={2}>
          <Link href="/">
            <Text color="#2563eb" fontSize="xs">&lt; Home</Text>
          </Link>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">Quiz</Heading>
        </Box>
      </Box>

      {/* Tabs */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb" display="flex">
        {(["vocabulary", "etymology"] as Tab[]).map((t) => (
          <Box
            key={t}
            flex={1}
            textAlign="center"
            py={2}
            cursor="pointer"
            onClick={() => handleTabChange(t)}
            position="relative"
          >
            <Text
              fontSize="sm"
              fontWeight={tab === t ? "semibold" : "normal"}
              color={tab === t ? "#2563eb" : "#999"}
            >
              {t === "vocabulary" ? "Vocabulary" : "Etymology"}
            </Text>
            {tab === t && (
              <Box
                position="absolute"
                bottom={0}
                left="50%"
                transform="translateX(-50%)"
                w="60%"
                h="3px"
                borderRadius="full"
                bg="#2563eb"
              />
            )}
          </Box>
        ))}
      </Box>

      {/* Mode cards */}
      <Box p={4} display="flex" flexDirection="column" gap={3}>
        {modes.map((mode) => {
          const isSelected = selectedMode === mode.key;
          return (
            <Box
              key={mode.key}
              p={4}
              bg="white"
              borderWidth={isSelected ? "2px" : "1px"}
              borderColor={isSelected ? "#2563eb" : "#e5e7eb"}
              borderRadius="lg"
              cursor="pointer"
              onClick={() => handleModeSelect(mode.key)}
            >
              <Box display="flex" alignItems="center" justifyContent="space-between">
                <Box>
                  <Text fontWeight="semibold" fontSize="md">{mode.title}</Text>
                  <Text fontSize="xs" color="#666">{mode.description}</Text>
                </Box>
                {!isSelected && (
                  <Text fontSize="sm" color="#999" flexShrink={0}>&rsaquo;</Text>
                )}
              </Box>
            </Box>
          );
        })}

        {/* Notebook selection (inline below selected mode) */}
        {showNotebookSelection && (
          <Box mt={1}>
            <Text fontWeight="medium" fontSize="sm" mb={1}>
              Select notebooks
            </Text>
            {tab === "etymology" && (
              <Text fontSize="xs" color="gray.500" mb={2}>
                Only notebooks with etymology data are shown
              </Text>
            )}

            <VStack align="stretch" gap={3}>
              <Checkbox.Root checked={allSelected} onCheckedChange={toggleAll}>
                <Checkbox.HiddenInput />
                <Checkbox.Control />
                <Checkbox.Label fontWeight="bold">All notebooks</Checkbox.Label>
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
                    <Box display="flex" justifyContent="space-between" w="full">
                      <Text>{notebook.name}</Text>
                      <Text color="gray.500" fontSize="sm">
                        {tab === "etymology"
                          ? `${notebook.etymologyReviewCount} words`
                          : selectedVocabMode === "reverse"
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

            {tab === "vocabulary" && selectedVocabMode === "reverse" && (
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
          </Box>
        )}

        {/* Freeform vocab — no notebook selection needed */}
        {tab === "vocabulary" && selectedVocabMode === "freeform" && (
          <Text color="gray.600" textAlign="center">
            {notebooks.reduce((sum, n) => sum + n.reviewCount, 0)} words
            available across all notebooks
          </Text>
        )}

        {/* Start button — only show when a mode is selected */}
        {selectedMode !== null && (
          <Button
            mt={2}
            w="full"
            colorPalette="blue"
            disabled={
              (showNotebookSelection && selectedIds.size === 0) || starting
            }
            onClick={handleStart}
          >
            {starting ? <Spinner size="sm" /> : "Start"}
          </Button>
        )}

        {error && (
          <Text color="red.500" mt={2}>{error}</Text>
        )}
      </Box>
    </Box>
  );
}
