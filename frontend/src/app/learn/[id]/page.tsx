"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import {
  Box,
  Button,
  Heading,
  Input,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  notebookClient,
  type GetNotebookDetailResponse,
  type StoryEntry,
} from "@/lib/client";
import {
  WordLookupPopup,
  useWordLookup,
} from "@/components/WordLookupPopup";
import { SceneContent } from "@/components/SceneContent";
import { findDeepLinkStoryIndex } from "@/lib/deepLink";

export default function LearnContentPage() {
  const params = useParams();
  const id = params.id as string;
  const searchParams = useSearchParams();
  const targetWord = searchParams.get("word") ?? "";
  const targetScene = searchParams.get("scene") ?? "";

  const [data, setData] = useState<GetNotebookDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedStoryIndex, setSelectedStoryIndex] = useState(0);

  useEffect(() => {
    notebookClient
      .getNotebookDetail({ notebookId: id })
      .then((res) => setData(res))
      .catch(() => setError("Failed to load notebook"))
      .finally(() => setLoading(false));
  }, [id]);

  // Derive the deep-link story index without setState-in-effect: when the
  // URL pins a target word we pick whichever story contains it (preferring
  // the matching scene when `?scene=` is also given) and let it override
  // the user's last manual selection.
  const deepLinkStoryIndex = useMemo(() => {
    if (!data) return -1;
    return findDeepLinkStoryIndex(data.stories, targetWord, targetScene);
  }, [data, targetWord, targetScene]);
  const effectiveStoryIndex = deepLinkStoryIndex >= 0 ? deepLinkStoryIndex : selectedStoryIndex;

  // Scroll the first highlighted occurrence of the target word into view and
  // remove the flash hook after a moment. Runs whenever the visible story
  // switches so each Open in Learn click re-fires the animation.
  useEffect(() => {
    if (!targetWord || !data) return;
    const el = document.getElementById("learn-deep-link-target");
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    const t = window.setTimeout(() => el.removeAttribute("data-deep-link-flash"), 1800);
    return () => window.clearTimeout(t);
  }, [targetWord, data, effectiveStoryIndex]);

  const handleDefinitionRemoved = useCallback(
    (storyIndex: number, sceneIndex: number, expression: string) => {
      setData((prev) => {
        if (!prev) return prev;
        const stories = prev.stories.map((s, si) => {
          if (si !== storyIndex) return s;
          return {
            ...s,
            scenes: s.scenes.map((sc, sci) => {
              if (sci !== sceneIndex) return sc;
              return {
                ...sc,
                definitions: sc.definitions.filter(
                  (d) => d.expression.toLowerCase() !== expression.toLowerCase(),
                ),
              };
            }),
          };
        });
        return { ...prev, stories };
      });
    },
    [],
  );

  const { lookup, popupRef, onTextSelect, onSaveDefinition, onDelete, onClose } =
    useWordLookup(id, data, handleDefinitionRemoved);

  if (loading) {
    return (
      <Box p={4} maxW="3xl" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error || !data) {
    return (
      <Box p={4} maxW="3xl" mx="auto">
        <Text color="red.500">{error ?? "Notebook not found"}</Text>
      </Box>
    );
  }

  const stories = data.stories;
  const currentStory: StoryEntry | undefined = stories[effectiveStoryIndex];

  return (
    <Box p={4} maxW="3xl" mx="auto" position="relative">
      <Box mb={2}>
        <Link href="/learn">
          <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="sm">
            &larr; Back to Learn
          </Text>
        </Link>
      </Box>

      <Heading size="lg" mb={4}>
        {data.name}
      </Heading>

      {stories.length > 0 && (
        <Box
          mb={4}
          display="flex"
          alignItems="center"
          gap={2}
        >
          <Box flex="1" minW={0}>
            <ChapterSelector
              stories={stories}
              selectedIndex={effectiveStoryIndex}
              onSelect={setSelectedStoryIndex}
            />
          </Box>
          <Link
            href={`/notebooks/${id}?chapter=${encodeURIComponent(currentStory?.event ?? "")}`}
          >
            <Button size="sm" variant="outline" flexShrink={0}>
              Word list
            </Button>
          </Link>
        </Box>
      )}

      {currentStory && (
        <VStack align="stretch" gap={6}>
          {currentStory.scenes.map((scene, sceneIdx) => (
            <SceneContent
              key={sceneIdx}
              scene={scene}
              storyIndex={effectiveStoryIndex}
              sceneIndex={sceneIdx}
              onTextSelect={onTextSelect}
              targetWord={targetWord}
            />
          ))}
        </VStack>
      )}

      {lookup && (
        <WordLookupPopup
          lookup={lookup}
          popupRef={popupRef}
          onSaveDefinition={onSaveDefinition}
          onDelete={onDelete}
          onClose={onClose}
        />
      )}
    </Box>
  );
}

// ChapterSelector renders a searchable combobox for picking a story/chapter.
// It is a lightweight custom control: Chakra v3 does not ship a combobox, and
// a native <select> cannot host a filter input. Keeps a small feature set —
// click to open, text filter, arrow keys + Enter + Escape, click outside to
// close — enough to make long chapter lists navigable without a new dep.
function ChapterSelector({
  stories,
  selectedIndex,
  onSelect,
}: {
  stories: StoryEntry[];
  selectedIndex: number;
  onSelect: (index: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [highlighted, setHighlighted] = useState(selectedIndex);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    const entries = stories.map((story, index) => ({ story, index }));
    const query = filter.trim().toLowerCase();
    if (!query) return entries;
    return entries.filter(({ story }) =>
      (story.event || "").toLowerCase().includes(query),
    );
  }, [stories, filter]);

  // Click outside to close
  useEffect(() => {
    if (!open) return;
    function handle(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handle);
    return () => document.removeEventListener("mousedown", handle);
  }, [open]);

  // Focus the filter input after the panel mounts.
  useEffect(() => {
    if (!open) return;
    const t = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(t);
  }, [open]);

  // `highlighted` is derived: if the stored value has been filtered out, fall
  // back to the first visible item. Computing this during render (instead of
  // chasing it with an effect) avoids a wasted re-render and cascading state.
  const effectiveHighlighted =
    filtered.length === 0
      ? -1
      : filtered.some((f) => f.index === highlighted)
        ? highlighted
        : filtered[0].index;

  // Scroll highlighted item into view during keyboard navigation
  useEffect(() => {
    if (!open || !listRef.current || effectiveHighlighted < 0) return;
    const el = listRef.current.querySelector<HTMLElement>(
      `[data-chapter-index="${effectiveHighlighted}"]`,
    );
    el?.scrollIntoView({ block: "nearest" });
  }, [effectiveHighlighted, open]);

  function openPanel() {
    setFilter("");
    setHighlighted(selectedIndex);
    setOpen(true);
  }

  function commit(index: number) {
    onSelect(index);
    setOpen(false);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Escape") {
      e.preventDefault();
      setOpen(false);
      return;
    }
    if (filtered.length === 0) return;
    const pos = filtered.findIndex((f) => f.index === effectiveHighlighted);
    if (e.key === "ArrowDown") {
      e.preventDefault();
      const next = filtered[(pos + 1) % filtered.length];
      setHighlighted(next.index);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      const next =
        filtered[(pos - 1 + filtered.length) % filtered.length];
      setHighlighted(next.index);
    } else if (e.key === "Enter") {
      e.preventDefault();
      commit(effectiveHighlighted);
    }
  }

  const selected = stories[selectedIndex];

  return (
    <Box ref={containerRef} position="relative">
      <Button
        onClick={() => (open ? setOpen(false) : openPanel())}
        aria-haspopup="listbox"
        aria-expanded={open}
        variant="outline"
        size="sm"
        w="100%"
        justifyContent="space-between"
      >
        <Text truncate fontSize="sm" fontWeight="medium">
          {selected?.event || `Chapter ${selectedIndex + 1}`}
        </Text>
        <Text fontSize="xs" color="fg.muted" ml={2} flexShrink={0}>
          {open ? "\u25B2" : "\u25BC"}
        </Text>
      </Button>

      {open && (
        <Box
          position="absolute"
          top="100%"
          left={0}
          right={0}
          mt={1}
          bg="bg.panel"
          borderWidth="1px"
          borderColor="border.emphasized"
          borderRadius="md"
          boxShadow="lg"
          zIndex={50}
          maxH="60vh"
          display="flex"
          flexDirection="column"
        >
          <Box p={2} borderBottomWidth="1px">
            <Input
              ref={inputRef}
              size="sm"
              placeholder="Search chapters..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              onKeyDown={handleKeyDown}
              aria-label="Search chapters"
            />
          </Box>
          <Box
            ref={listRef}
            overflowY="auto"
            flex="1"
            role="listbox"
            aria-label="Chapters"
          >
            {filtered.length === 0 ? (
              <Box p={3}>
                <Text fontSize="sm" color="fg.muted">
                  No chapters match.
                </Text>
              </Box>
            ) : (
              filtered.map(({ story, index }) => {
                const isHighlighted = index === effectiveHighlighted;
                const isSelected = index === selectedIndex;
                return (
                  <Box
                    key={index}
                    data-chapter-index={index}
                    role="option"
                    aria-selected={isSelected}
                    onClick={() => commit(index)}
                    onMouseEnter={() => setHighlighted(index)}
                    px={3}
                    py={2}
                    cursor="pointer"
                    bg={isHighlighted ? "bg.muted" : undefined}
                    borderLeftWidth="3px"
                    borderLeftColor={
                      isSelected ? "blue.500" : "transparent"
                    }
                  >
                    <Text
                      fontSize="sm"
                      fontWeight={isSelected ? "semibold" : "normal"}
                      truncate
                    >
                      {story.event || `Chapter ${index + 1}`}
                    </Text>
                  </Box>
                );
              })
            )}
          </Box>
        </Box>
      )}
    </Box>
  );
}
