"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useParams } from "next/navigation";
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
  type NotebookWord,
  type StoryEntry,
  type StoryScene,
} from "@/lib/client";
import {
  WordLookupPopup,
  useWordLookup,
} from "@/components/WordLookupPopup";

// renderHighlightedText wraps every occurrence of a defined expression in the
// given text with a bold/colored span. It also honors explicit {{ word }}
// markers authored in the source notebook. Kept here rather than in /lib
// because it is only used by the story reader.
function renderHighlightedText(
  text: string,
  definitions: NotebookWord[],
): React.ReactNode[] {
  const markerParts = text.split(/\{\{\s*([^}]+?)\s*\}\}/);
  const nodes: React.ReactNode[] = [];

  for (let i = 0; i < markerParts.length; i++) {
    if (i % 2 === 1) {
      nodes.push(
        <Text
          as="span"
          key={`m-${i}`}
          fontWeight="bold"
          color="blue.600"
          _dark={{ color: "blue.300" }}
        >
          {markerParts[i].trim()}
        </Text>,
      );
      continue;
    }

    const segment = markerParts[i];
    const expressions = definitions
      .map((d) => d.expression)
      .filter((e) => e.length > 0)
      .sort((a, b) => b.length - a.length);

    if (expressions.length === 0) {
      nodes.push(
        <Text as="span" key={`t-${i}`}>
          {segment}
        </Text>,
      );
      continue;
    }

    const escapedExpressions = expressions.map((e) =>
      e.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
    );
    const regex = new RegExp(`\\b(${escapedExpressions.join("|")})\\b`, "gi");
    const subSegments = segment.split(regex);

    subSegments.forEach((sub, j) => {
      const isDefined = expressions.some(
        (e) => e.toLowerCase() === sub.toLowerCase(),
      );
      if (isDefined) {
        nodes.push(
          <Text
            as="span"
            key={`t-${i}-${j}`}
            fontWeight="bold"
            color="blue.600"
            _dark={{ color: "blue.300" }}
          >
            {sub}
          </Text>,
        );
      } else {
        nodes.push(
          <Text as="span" key={`t-${i}-${j}`}>
            {sub}
          </Text>,
        );
      }
    });
  }

  return nodes;
}

export default function LearnContentPage() {
  const params = useParams();
  const id = params.id as string;

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
  const currentStory: StoryEntry | undefined = stories[selectedStoryIndex];

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
              selectedIndex={selectedStoryIndex}
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
              storyIndex={selectedStoryIndex}
              sceneIndex={sceneIdx}
              onTextSelect={onTextSelect}
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

function SceneContent({
  scene,
  storyIndex,
  sceneIndex,
  onTextSelect,
}: {
  scene: StoryScene;
  storyIndex: number;
  sceneIndex: number;
  onTextSelect: (storyIndex: number, sceneIndex: number) => void;
}) {
  const hasStatements = scene.statements.length > 0;
  const hasConversations = scene.conversations.length > 0;

  // Hide scenes that carry neither prose nor dialogue (e.g. flashcard-style
  // scenes). The /notebooks/[id] word list is the right place for those.
  if (!hasStatements && !hasConversations) return null;

  return (
    <Box>
      {scene.title && (
        <Heading size="sm" mb={2} color="fg.muted">
          {scene.title}
        </Heading>
      )}

      {hasStatements && (
        <Box
          onMouseUp={() => onTextSelect(storyIndex, sceneIndex)}
          cursor="text"
          lineHeight="tall"
        >
          <VStack align="stretch" gap={3}>
            {scene.statements.map((stmt, i) => (
              <Text key={i} fontSize="md">
                {renderHighlightedText(stmt, scene.definitions)}
              </Text>
            ))}
          </VStack>
        </Box>
      )}

      {hasConversations && (
        <Box
          onMouseUp={() => onTextSelect(storyIndex, sceneIndex)}
          cursor="text"
          mt={hasStatements ? 3 : 0}
        >
          <VStack align="stretch" gap={1}>
            {scene.conversations.map((conv, i) => (
              <Text key={i} fontSize="sm" color="fg.muted">
                <Text as="span" fontWeight="bold" color="fg.default">
                  {conv.speaker}:
                </Text>{" "}
                &ldquo;{renderHighlightedText(conv.quote, scene.definitions)}&rdquo;
              </Text>
            ))}
          </VStack>
        </Box>
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
