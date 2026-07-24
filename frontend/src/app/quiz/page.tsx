"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Box,
  Button,
  Checkbox,
  Heading,
  Input,
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
import RelearnStart from "@/components/RelearnStart";
import GrammarStart from "@/components/GrammarStart";

type Tab = "vocabulary" | "etymology" | "relearn" | "grammar";
type VocabMode = "standard" | "reverse" | "freeform";
type EtyMode = "standard" | "reverse" | "freeform";

const vocabularyModes: { key: VocabMode; title: string; description: string }[] = [
  { key: "standard", title: "Standard", description: "See a word, type its meaning" },
  { key: "reverse", title: "Reverse", description: "See a meaning, type the word" },
  { key: "freeform", title: "Freeform", description: "Type any word and its meaning" },
];

const etymologyModes: { key: EtyMode; title: string; description: string }[] = [
  { key: "standard", title: "Standard", description: "See an origin, type its meaning" },
  { key: "reverse", title: "Reverse", description: "See a meaning, type the origin" },
  { key: "freeform", title: "Freeform", description: "Type any origin and its meaning" },
];

export default function QuizHubPage() {
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("vocabulary");
  const [selectedVocabMode, setSelectedVocabMode] = useState<VocabMode | null>(null);
  const [selectedEtyMode, setSelectedEtyMode] = useState<EtyMode | null>(null);

  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  // sectionSelections holds per-notebook section title sets when the user
  // narrows a notebook to specific chapters/sessions. Absence means "all
  // sections" (the default when a notebook is checked).
  const [sectionSelections, setSectionSelections] = useState<Map<string, Set<string>>>(new Map());
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
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
  const setEtymologyOriginCards = useQuizStore((s) => s.setEtymologyOriginCards);
  const setEtymologyFreeformOrigins = useQuizStore((s) => s.setEtymologyFreeformOrigins);
  const setEtymologyFreeformNextReviewDates = useQuizStore((s) => s.setEtymologyFreeformNextReviewDates);
  const feedbackInterval = useQuizStore((s) => s.feedbackInterval);
  const setFeedbackInterval = useQuizStore((s) => s.setFeedbackInterval);
  const [feedbackIntervalText, setFeedbackIntervalText] = useState(
    feedbackInterval.toString(),
  );

  // Re-fetch the notebook summary list whenever includeUnstudied flips
  // so per-notebook + per-section counts match what the actual quiz
  // will load. Only the FIRST fetch shows the full-page spinner; toggle-
  // driven refetches keep the current list visible and swap counts in
  // place when the response arrives — toggling the switch shouldn't feel
  // like a page reload. initialLoadRef gates that distinction.
  // Open the Relearn tab when arrived at via /quiz?tab=relearn (from the
  // relearn complete screen's "Relearn again", or a session that ended empty).
  useEffect(() => {
    if (new URLSearchParams(window.location.search).get("tab") === "relearn") {
      setTab("relearn");
    }
  }, []);

  const initialLoadRef = useRef(true);
  useEffect(() => {
    if (initialLoadRef.current) {
      setLoading(true);
    }
    quizClient
      .getQuizOptions({ includeUnstudied })
      .then((res) => setNotebooks(res.notebooks ?? []))
      .catch(() => setError("Failed to load notebooks"))
      .finally(() => {
        setLoading(false);
        initialLoadRef.current = false;
      });
  }, [includeUnstudied]);

  const selectedMode = tab === "vocabulary" ? selectedVocabMode : selectedEtyMode;

  const isFreeformMode =
    (tab === "vocabulary" && selectedVocabMode === "freeform") ||
    (tab === "etymology" && selectedEtyMode === "freeform");

  // Notebook lists. Hide notebooks with zero review count for the selected
  // mode; when includeUnstudied is on (or in freeform, which always covers
  // unstudied words) we can't know the unstudied count so show all of them.
  const displayedNotebooks = useMemo(() => {
    const base = notebooks.filter((n) =>
      tab === "vocabulary" ? n.kind !== "Etymology" : n.kind === "Etymology",
    );
    if (includeUnstudied || isFreeformMode) return base;
    return base.filter((n) => {
      if (tab === "etymology") {
        return selectedEtyMode === "reverse"
          ? n.etymologyReverseReviewCount > 0
          : n.etymologyReviewCount > 0;
      }
      if (selectedVocabMode === "reverse") return n.reverseReviewCount > 0;
      return n.reviewCount > 0;
    });
  }, [notebooks, tab, includeUnstudied, isFreeformMode, selectedVocabMode, selectedEtyMode]);

  // Drop selections that are hidden by the current filter so the user doesn't
  // accidentally start a quiz referencing notebooks they can no longer see.
  useEffect(() => {
    const displayedIds = new Set(displayedNotebooks.map((n) => n.notebookId));
    setSelectedIds((prev) => {
      const next = new Set(Array.from(prev).filter((id) => displayedIds.has(id)));
      return next.size === prev.size ? prev : next;
    });
    setSectionSelections((prev) => {
      const next = new Map<string, Set<string>>();
      for (const [id, sections] of prev.entries()) {
        if (displayedIds.has(id)) next.set(id, sections);
      }
      return next.size === prev.size ? prev : next;
    });
  }, [displayedNotebooks]);

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
    // Clearing a notebook also clears any per-section narrowing it had —
    // re-checking later starts fresh with "all sections".
    setSectionSelections((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Map(prev);
      next.delete(id);
      return next;
    });
  };

  const toggleAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
      setSectionSelections(new Map());
    } else {
      setSelectedIds(new Set(displayedNotebooks.map((n) => n.notebookId)));
      // "All notebooks" implies "all sections" — drop any narrowing.
      setSectionSelections(new Map());
    }
  };

  const toggleSection = (notebook: NotebookSummary, sectionTitle: string) => {
    const id = notebook.notebookId;
    const allTitles = (notebook.sections ?? []).map((s) => s.title);
    const notebookSelected = selectedIds.has(id);
    const existing = sectionSelections.get(id);

    // Resolve the *currently displayed* set of checked sections so the
    // toggle matches what the user sees:
    //   - notebook unchecked → no sections shown checked → start empty.
    //   - notebook checked, no explicit narrowing → all sections shown
    //     checked → start with the full set.
    //   - notebook checked, explicit narrowing → start with that set.
    let current: Set<string>;
    if (!notebookSelected) {
      current = new Set();
    } else if (existing) {
      current = new Set(existing);
    } else {
      current = new Set(allTitles);
    }
    if (current.has(sectionTitle)) current.delete(sectionTitle);
    else current.add(sectionTitle);

    setSectionSelections((prev) => {
      const next = new Map(prev);
      if (current.size === 0) {
        next.delete(id);
      } else if (current.size === allTitles.length) {
        // Everything checked → drop the explicit set so we always send
        // "all sections" (resilient to future section additions).
        next.delete(id);
      } else {
        next.set(id, current);
      }
      return next;
    });

    setSelectedIds((prev) => {
      const isNowSelected = current.size > 0;
      if (isNowSelected === prev.has(id)) return prev;
      const next = new Set(prev);
      if (isNowSelected) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const isSectionChecked = (notebookId: string, sectionTitle: string): boolean => {
    if (!selectedIds.has(notebookId)) return false;
    const sel = sectionSelections.get(notebookId);
    if (!sel) return true; // notebook checked + no narrowing = all sections in
    return sel.has(sectionTitle);
  };

  const toggleExpanded = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // pickModeCount maps a section/notebook count source to the active mode.
  // Etymology has separate standard and reverse counts because the same
  // origin can be due in one mode and not the other (each mode tracks its
  // own SR interval and skip flag).
  const pickModeCount = (counts: {
    reviewCount: number;
    reverseReviewCount: number;
    etymologyReviewCount: number;
    etymologyReverseReviewCount: number;
  }): number => {
    if (tab === "etymology") {
      return selectedEtyMode === "reverse"
        ? counts.etymologyReverseReviewCount
        : counts.etymologyReviewCount;
    }
    if (selectedVocabMode === "reverse") return counts.reverseReviewCount;
    return counts.reviewCount;
  };

  // totalDue sums only the words actually due across the active selection,
  // honouring per-section narrowing. When a notebook has no per-section
  // narrowing we fall back to its notebook-level count.
  const totalDue = displayedNotebooks
    .filter((n) => selectedIds.has(n.notebookId))
    .reduce((sum, n) => {
      const sel = sectionSelections.get(n.notebookId);
      if (!sel || (n.sections ?? []).length === 0) {
        return sum + pickModeCount(n);
      }
      const sectionSum = (n.sections ?? [])
        .filter((s) => sel.has(s.title))
        .reduce((acc, s) => acc + pickModeCount(s), 0);
      return sum + sectionSum;
    }, 0);

  const handleTabChange = (newTab: Tab) => {
    setTab(newTab);
    setSelectedIds(new Set());
    setSectionSelections(new Map());
    setExpandedIds(new Set());
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
    setSectionSelections(new Map());
    setExpandedIds(new Set());
  };

  // buildNotebookSections turns the local selection state into the
  // NotebookSection list the backend expects. A notebook with no per-section
  // narrowing is sent with an empty section_titles list ("all sections").
  const buildNotebookSections = (): { notebookId: string; sectionTitles: string[] }[] => {
    return Array.from(selectedIds).map((id) => {
      const sel = sectionSelections.get(id);
      return { notebookId: id, sectionTitles: sel ? Array.from(sel) : [] };
    });
  };

  const showNotebookSelection =
    selectedMode !== null &&
    !(tab === "vocabulary" && selectedVocabMode === "freeform");

  const showFeedbackInterval = selectedMode !== null && !isFreeformMode;

  const handleStart = async () => {
    const parsed = parseInt(feedbackIntervalText, 10);
    if (!isFreeformMode && (!Number.isFinite(parsed) || parsed < 1)) {
      setError("Feedback interval must be a positive number");
      return;
    }
    if (!isFreeformMode) {
      setFeedbackInterval(parsed);
    }
    setStarting(true);
    try {
      const notebookSections = buildNotebookSections();
      if (tab === "vocabulary") {
        if (selectedVocabMode === "standard") {
          setQuizType("standard");
          const res = await quizClient.startQuiz({
            notebookSections,
            includeUnstudied,
          });
          setFlashcards(
            (res.flashcards ?? []).map((f) => ({
              noteId: f.noteId, entry: f.entry, originalEntry: f.originalEntry, examples: f.examples,
              conceptHead: f.conceptHead, conceptMembers: f.conceptMembers, conceptMeaning: f.conceptMeaning,
            })),
          );
          router.push("/quiz/standard");
        } else if (selectedVocabMode === "reverse") {
          setQuizType("reverse");
          const res = await quizClient.startReverseQuiz({
            notebookSections,
            listMissingContext,
            includeUnstudied,
          });
          setReverseFlashcards(
            (res.flashcards ?? []).map((f) => ({
              noteId: f.noteId, meaning: f.meaning, contexts: f.contexts,
              notebookName: f.notebookName, storyTitle: f.storyTitle, sceneTitle: f.sceneTitle,
              conceptHead: f.conceptHead, conceptMembers: f.conceptMembers, conceptMeaning: f.conceptMeaning,
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
        if (selectedEtyMode === "freeform") {
          setQuizType("etymology-freeform" as QuizType);
          // Etymology freeform doesn't yet support per-session narrowing;
          // the canonical "all sessions of selected notebooks" form keeps
          // the existing API shape.
          const res = await quizClient.startEtymologyFreeformQuiz({
            etymologyNotebookIds: Array.from(selectedIds),
          });
          setEtymologyFreeformOrigins(res.origins ?? []);
          setEtymologyFreeformNextReviewDates(res.nextReviewDates ?? {});
          router.push("/quiz/etymology-freeform");
        } else {
          const quizMode = selectedEtyMode === "standard"
            ? EtymologyQuizMode.STANDARD : EtymologyQuizMode.REVERSE;
          const storeType = selectedEtyMode === "standard"
            ? "etymology-standard" as QuizType : "etymology-reverse" as QuizType;
          setQuizType(storeType);
          const res = await quizClient.startEtymologyQuiz({
            notebookSections,
            mode: quizMode,
            includeUnstudied,
          });
          setEtymologyOriginCards(
            (res.cards ?? []).map((c) => ({
              cardId: c.cardId, origin: c.origin, type: c.type,
              language: c.language, meaning: c.meaning,
              notebookName: c.notebookName, sessionTitle: c.sessionTitle,
              sense: c.sense,
              exampleWords: c.exampleWords ?? [],
              graphPrompt: c.graphPrompt,
            })),
          );
          router.push(selectedEtyMode === "standard" ? "/quiz/etymology-standard" : "/quiz/etymology-reverse");
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
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh">
      {/* Header */}
      <Box bg="white" _dark={{ bg: "gray.800", borderColor: "gray.600" }} borderBottomWidth="1px" borderColor="gray.200">
        <Box px={4} pt={2}>
          <Link href="/">
            <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="xs">&lt; Home</Text>
          </Link>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">Quiz</Heading>
        </Box>
      </Box>

      {/* Tabs — Vocabulary / Etymology switch mode cards in place; Relearn is a
          cross-quiz-type flow whose tab navigates to its own start screen. */}
      <Box bg="white" _dark={{ bg: "gray.800", borderColor: "gray.600" }} borderBottomWidth="1px" borderColor="gray.200" display="flex">
        {(["vocabulary", "etymology", "relearn", "grammar"] as Tab[]).map((t) => (
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
              color={tab === t ? "blue.600" : "gray.500"}
              _dark={{ color: tab === t ? "blue.300" : "gray.400" }}
            >
              {t === "vocabulary" ? "Vocabulary" : t === "etymology" ? "Etymology" : t === "relearn" ? "Relearn" : "Grammar"}
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
                bg="blue.600"
                _dark={{ bg: "blue.300" }}
              />
            )}
          </Box>
        ))}
      </Box>

      {/* Content: the Relearn tab renders its start screen inline (so it stays
          within the tabbed hub); the other tabs show mode cards + notebooks. */}
      {tab === "relearn" ? (
        <RelearnStart />
      ) : tab === "grammar" ? (
        <GrammarStart />
      ) : (
      <Box p={4} display="flex" flexDirection="column" gap={3}>
        {modes.map((mode) => {
          const isSelected = selectedMode === mode.key;
          return (
            <Box
              key={mode.key}
              p={4}
              bg="white"
              borderWidth={isSelected ? "2px" : "1px"}
              borderColor={isSelected ? "blue.600" : "gray.200"}
              _dark={{ bg: "gray.800", borderColor: isSelected ? "blue.400" : "gray.600" }}
              borderRadius="lg"
              cursor="pointer"
              onClick={() => handleModeSelect(mode.key)}
            >
              <Box display="flex" alignItems="center" justifyContent="space-between">
                <Box>
                  <Text fontWeight="semibold" fontSize="md">{mode.title}</Text>
                  <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }}>{mode.description}</Text>
                </Box>
                {!isSelected && (
                  <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} flexShrink={0}>&rsaquo;</Text>
                )}
              </Box>
            </Box>
          );
        })}

        {/* Quiz options (shown above the notebook picker so filters apply) */}
        {selectedMode !== null && (
          <VStack align="stretch" gap={3} mt={1}>
            {showNotebookSelection && !isFreeformMode && (
              <Switch.Root
                checked={includeUnstudied}
                onCheckedChange={(e) => setIncludeUnstudied(e.checked)}
              >
                <Switch.HiddenInput />
                <Switch.Control />
                <Switch.Label>Include unstudied words</Switch.Label>
              </Switch.Root>
            )}

            {tab === "vocabulary" && selectedVocabMode === "reverse" && (
              <Switch.Root
                checked={listMissingContext}
                onCheckedChange={(e) => setListMissingContext(e.checked)}
              >
                <Switch.HiddenInput />
                <Switch.Control />
                <Switch.Label>List words missing context</Switch.Label>
              </Switch.Root>
            )}

            {showFeedbackInterval && (
              <Box>
                <Text fontWeight="medium" fontSize="sm" mb={1}>
                  Questions per feedback screen
                </Text>
                <Input
                  type="number"
                  min={1}
                  value={feedbackIntervalText}
                  onChange={(e) => setFeedbackIntervalText(e.target.value)}
                  placeholder="10"
                />
                <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mt={1}>
                  See feedback for multiple answers at once. Default: 10.
                </Text>
              </Box>
            )}
          </VStack>
        )}

        {/* Notebook selection (filtered by selected options) */}
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

            {displayedNotebooks.length === 0 ? (
              <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} mt={2}>
                {isFreeformMode ? (
                  "No notebooks available for this mode."
                ) : (
                  <>
                    No notebooks have words due for this mode. Turn on{" "}
                    <Text as="span" fontWeight="medium">Include unstudied words</Text>{" "}
                    to see more.
                  </>
                )}
              </Text>
            ) : (
              <VStack align="stretch" gap={3}>
                <Checkbox.Root checked={allSelected} onCheckedChange={toggleAll}>
                  <Checkbox.HiddenInput />
                  <Checkbox.Control />
                  <Checkbox.Label fontWeight="bold">All notebooks</Checkbox.Label>
                </Checkbox.Root>

                {displayedNotebooks.map((notebook) => {
                  const sections = notebook.sections ?? [];
                  const expanded = expandedIds.has(notebook.notebookId);
                  const sel = sectionSelections.get(notebook.notebookId);
                  const partial =
                    selectedIds.has(notebook.notebookId) && sel !== undefined && sel.size < sections.length;
                  return (
                    <Box key={notebook.notebookId}>
                      <Box display="flex" alignItems="center" gap={2}>
                        <Checkbox.Root
                          checked={selectedIds.has(notebook.notebookId)}
                          onCheckedChange={() => toggleNotebook(notebook.notebookId)}
                          flex="1"
                        >
                          <Checkbox.HiddenInput />
                          <Checkbox.Control />
                          <Checkbox.Label flex="1">
                            <Box display="flex" justifyContent="space-between" w="full">
                              <Text>
                                {notebook.name}
                                {partial && (
                                  <Text as="span" color="gray.500" fontSize="xs" ml={1}>
                                    ({sel!.size}/{sections.length})
                                  </Text>
                                )}
                              </Text>
                              <Text color="gray.500" fontSize="sm">
                                {pickModeCount(notebook)}
                              </Text>
                            </Box>
                          </Checkbox.Label>
                        </Checkbox.Root>
                        {sections.length > 0 && (
                          <Box
                            as="button"
                            onClick={() => toggleExpanded(notebook.notebookId)}
                            fontSize="xs"
                            color="blue.600"
                            _dark={{ color: "blue.300" }}
                            cursor="pointer"
                            px={2}
                            aria-label={expanded ? "Hide sections" : "Show sections"}
                          >
                            {expanded ? "▲" : "▼"}
                          </Box>
                        )}
                      </Box>
                      {expanded && sections.length > 0 && (
                        <VStack align="stretch" gap={1} pl={6} mt={1} mb={1}>
                          {sections.map((section) => (
                            <Checkbox.Root
                              key={section.title}
                              checked={isSectionChecked(notebook.notebookId, section.title)}
                              onCheckedChange={() => toggleSection(notebook, section.title)}
                              size="sm"
                            >
                              <Checkbox.HiddenInput />
                              <Checkbox.Control />
                              <Checkbox.Label fontSize="sm" color="gray.700" _dark={{ color: "gray.300" }} flex="1">
                                <Box display="flex" justifyContent="space-between" w="full">
                                  <Text as="span" truncate>{section.title}</Text>
                                  <Text as="span" color="gray.500" fontSize="xs" ml={2}>
                                    {pickModeCount(section)}
                                  </Text>
                                </Box>
                              </Checkbox.Label>
                            </Checkbox.Root>
                          ))}
                        </VStack>
                      )}
                    </Box>
                  );
                })}
              </VStack>
            )}

            {displayedNotebooks.length > 0 && (
              <Text mt={4} fontWeight="bold" textAlign="center">
                {totalDue} words due for review
              </Text>
            )}
          </Box>
        )}

        {/* Freeform vocab -- no notebook selection needed */}
        {tab === "vocabulary" && selectedVocabMode === "freeform" && (
          <Text color="gray.600" _dark={{ color: "gray.400" }} textAlign="center">
            {notebooks.reduce((sum, n) => sum + n.reviewCount, 0)} words
            available across all notebooks
          </Text>
        )}

        {/* Start button -- only show when a mode is selected */}
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
      )}
    </Box>
  );
}
