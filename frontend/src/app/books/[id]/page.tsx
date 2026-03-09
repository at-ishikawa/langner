"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  Box,
  Button,
  Heading,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  notebookClient,
  type GetNotebookDetailResponse,
  type StoryEntry,
  type StoryScene,
  type NotebookWord,
  type WordDefinition,
} from "@/lib/client";

function renderBookText(
  statement: string,
  definitions: NotebookWord[],
): React.ReactNode[] {
  // Highlight {{ word }} markers (same pattern as renderQuote in notebooks)
  const markerParts = statement.split(/\{\{\s*([^}]+?)\s*\}\}/);
  const nodes: React.ReactNode[] = [];

  for (let i = 0; i < markerParts.length; i++) {
    if (i % 2 === 1) {
      // This is a marked word
      nodes.push(
        <Text as="span" key={`m-${i}`} fontWeight="bold" color="blue.600">
          {markerParts[i].trim()}
        </Text>,
      );
    } else {
      // Plain text segment - highlight any defined words within it
      const text = markerParts[i];
      if (definitions.length === 0) {
        nodes.push(
          <Text as="span" key={`t-${i}`}>
            {text}
          </Text>,
        );
        continue;
      }

      // Build a regex to match any defined expression in the text
      const expressions = definitions
        .map((d) => d.expression)
        .filter((e) => e.length > 0)
        .sort((a, b) => b.length - a.length); // Longer first

      if (expressions.length === 0) {
        nodes.push(
          <Text as="span" key={`t-${i}`}>
            {text}
          </Text>,
        );
        continue;
      }

      const escapedExpressions = expressions.map((e) =>
        e.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
      );
      const regex = new RegExp(`\\b(${escapedExpressions.join("|")})\\b`, "gi");
      const segments = text.split(regex);

      segments.forEach((seg, j) => {
        const isDefinedWord = expressions.some(
          (e) => e.toLowerCase() === seg.toLowerCase(),
        );
        if (isDefinedWord) {
          nodes.push(
            <Text
              as="span"
              key={`t-${i}-${j}`}
              fontWeight="bold"
              color="blue.600"
            >
              {seg}
            </Text>,
          );
        } else {
          nodes.push(
            <Text as="span" key={`t-${i}-${j}`}>
              {seg}
            </Text>,
          );
        }
      });
    }
  }

  return nodes;
}

interface LookupState {
  word: string;
  context: string;
  storyIndex: number;
  sceneIndex: number;
  definitions: WordDefinition[];
  source: string;
  loading: boolean;
  error: string | null;
  saved: boolean;
  saving: boolean;
  savedDefinition: NotebookWord | null;
  deleting: boolean;
  deleted: boolean;
}

export default function BookReaderPage() {
  const params = useParams();
  const id = params.id as string;

  const [data, setData] = useState<GetNotebookDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedStoryIndex, setSelectedStoryIndex] = useState(0);
  const [lookup, setLookup] = useState<LookupState | null>(null);
  const popupRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    notebookClient
      .getNotebookDetail({ notebookId: id })
      .then((res) => setData(res))
      .catch(() => setError("Failed to load book"))
      .finally(() => setLoading(false));
  }, [id]);

  // Close popup on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (popupRef.current && !popupRef.current.contains(e.target as Node)) {
        setLookup(null);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleTextSelect = useCallback(
    (storyIndex: number, sceneIndex: number) => {
      const selection = window.getSelection();
      const selectedText = selection?.toString().trim();
      if (!selectedText || selectedText.length === 0) {
        return;
      }
      // Skip very long selections (likely not a word lookup)
      if (selectedText.length > 100) {
        return;
      }

      // Get surrounding context
      const range = selection?.getRangeAt(0);
      const container = range?.startContainer.parentElement;
      const context = container?.textContent?.trim() ?? "";

      // Check if this word is already saved in this scene
      const currentScene = data?.stories[storyIndex]?.scenes[sceneIndex];
      const existing = currentScene?.definitions.find(
        (d) => d.expression.toLowerCase() === selectedText.toLowerCase(),
      );

      if (existing) {
        setLookup({
          word: selectedText,
          context,
          storyIndex,
          sceneIndex,
          definitions: [],
          source: "",
          loading: false,
          error: null,
          saved: false,
          saving: false,
          savedDefinition: existing,
          deleting: false,
          deleted: false,
        });
        return;
      }

      setLookup({
        word: selectedText,
        context,
        storyIndex,
        sceneIndex,
        definitions: [],
        source: "",
        loading: true,
        error: null,
        saved: false,
        saving: false,
        savedDefinition: null,
        deleting: false,
        deleted: false,
      });

      notebookClient
        .lookupWord({
          word: selectedText,
          notebookId: id,
          context,
        })
        .then((res) => {
          setLookup((prev) => {
            if (!prev || prev.word !== selectedText) return prev;
            return {
              ...prev,
              definitions: res.definitions,
              source: res.source,
              loading: false,
            };
          });
        })
        .catch(() => {
          setLookup((prev) => {
            if (!prev || prev.word !== selectedText) return prev;
            return {
              ...prev,
              loading: false,
              error: "Failed to look up word",
            };
          });
        });
    },
    [id, data],
  );

  const handleSaveDefinition = useCallback(
    (defIndex: number) => {
      if (!lookup || !data) return;

      const def = lookup.definitions[defIndex];
      if (!def) return;

      const story = data.stories[lookup.storyIndex];
      if (!story) return;

      setLookup((prev) => (prev ? { ...prev, saving: true } : null));

      notebookClient
        .registerDefinition({
          notebookId: id,
          notebookFile: story.event || "",
          sceneIndex: lookup.sceneIndex,
          expression: lookup.word,
          meaning: def.definition,
          partOfSpeech: def.partOfSpeech,
          examples: def.examples,
        })
        .then(() => {
          setLookup((prev) =>
            prev ? { ...prev, saving: false, saved: true } : null,
          );
        })
        .catch(() => {
          setLookup((prev) =>
            prev
              ? { ...prev, saving: false, error: "Failed to save definition" }
              : null,
          );
        });
    },
    [lookup, data, id],
  );

  const handleDelete = useCallback(() => {
    if (!lookup || !data) return;
    const story = data.stories[lookup.storyIndex];
    if (!story) return;

    setLookup((prev) => (prev ? { ...prev, deleting: true } : null));

    notebookClient
      .deleteDefinition({
        notebookId: id,
        notebookFile: story.event || "",
        sceneIndex: lookup.sceneIndex,
        expression: lookup.word,
      })
      .then(() => {
        setLookup((prev) =>
          prev ? { ...prev, deleting: false, deleted: true } : null,
        );
        // Remove definition from local state so highlight disappears
        setData((prev) => {
          if (!prev) return prev;
          const stories = prev.stories.map((s, si) => {
            if (si !== lookup.storyIndex) return s;
            return {
              ...s,
              scenes: s.scenes.map((sc, sci) => {
                if (sci !== lookup.sceneIndex) return sc;
                return {
                  ...sc,
                  definitions: sc.definitions.filter(
                    (d) =>
                      d.expression.toLowerCase() !==
                      lookup.word.toLowerCase(),
                  ),
                };
              }),
            };
          });
          return { ...prev, stories };
        });
      })
      .catch(() => {
        setLookup((prev) =>
          prev
            ? { ...prev, deleting: false, error: "Failed to delete definition" }
            : null,
        );
      });
  }, [lookup, data, id]);

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
        <Text color="red.500">{error ?? "Book not found"}</Text>
      </Box>
    );
  }

  const stories = data.stories;
  const currentStory: StoryEntry | undefined = stories[selectedStoryIndex];

  return (
    <Box p={4} maxW="3xl" mx="auto" position="relative">
      {/* Header navigation */}
      <Box mb={2}>
        <Link href="/books">
          <Text color="blue.600" fontSize="sm">
            &larr; Back to books
          </Text>
        </Link>
      </Box>

      <Box
        mb={4}
        display="flex"
        justifyContent="space-between"
        alignItems="flex-start"
        gap={2}
      >
        <Heading size="lg">{data.name}</Heading>
        <Link
          href={`/notebooks/${id}?chapter=${encodeURIComponent(currentStory?.event ?? "")}`}
        >
          <Button size="sm" variant="outline">
            View Notebook
          </Button>
        </Link>
      </Box>

      {/* Chapter navigation */}
      {stories.length > 1 && (
        <Box mb={4} overflowX="auto">
          <Box display="flex" gap={2} pb={2}>
            {stories.map((story, i) => (
              <Button
                key={i}
                size="sm"
                variant={i === selectedStoryIndex ? "solid" : "outline"}
                onClick={() => setSelectedStoryIndex(i)}
                flexShrink={0}
              >
                {story.event || `Chapter ${i + 1}`}
              </Button>
            ))}
          </Box>
        </Box>
      )}

      {/* Chapter content */}
      {currentStory && (
        <Box>
          <Heading size="md" mb={4}>
            {currentStory.event}
          </Heading>

          <VStack align="stretch" gap={6}>
            {currentStory.scenes.map((scene, sceneIdx) => (
              <SceneContent
                key={sceneIdx}
                scene={scene}
                storyIndex={selectedStoryIndex}
                sceneIndex={sceneIdx}
                onTextSelect={handleTextSelect}
              />
            ))}
          </VStack>
        </Box>
      )}

      {/* Word lookup popup */}
      {lookup && (
        <Box
          ref={popupRef}
          position="fixed"
          bottom={0}
          left={0}
          right={0}
          bg="bg.panel"
          borderTopWidth="2px"
          borderColor="blue.400"
          p={4}
          maxH="50vh"
          overflowY="auto"
          zIndex={100}
          boxShadow="lg"
        >
          {lookup.savedDefinition && !lookup.deleted ? (
            <Box>
              <Box
                display="flex"
                alignItems="center"
                gap={2}
                mb={3}
                justifyContent="space-between"
              >
                <Box display="flex" alignItems="center" gap={2}>
                  <Heading size="sm">{lookup.word}</Heading>
                  <Text
                    fontSize="xs"
                    px={2}
                    py={0.5}
                    bg="green.100"
                    color="green.700"
                    borderRadius="full"
                    _dark={{ bg: "green.900", color: "green.200" }}
                  >
                    Saved
                  </Text>
                </Box>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => setLookup(null)}
                >
                  Close
                </Button>
              </Box>
              {lookup.savedDefinition.partOfSpeech && (
                <Text fontSize="xs" color="fg.muted" mb={1}>
                  {lookup.savedDefinition.partOfSpeech}
                </Text>
              )}
              <Text fontSize="sm" mb={2}>
                {lookup.savedDefinition.meaning ||
                  lookup.savedDefinition.definition}
              </Text>
              {lookup.savedDefinition.examples.length > 0 && (
                <Box mb={2}>
                  {lookup.savedDefinition.examples.map((ex, i) => (
                    <Text key={i} fontSize="xs" color="fg.muted" pl={2}>
                      {ex}
                    </Text>
                  ))}
                </Box>
              )}
              {lookup.error && (
                <Text color="red.500" fontSize="sm" mb={2}>
                  {lookup.error}
                </Text>
              )}
              <Button
                size="xs"
                colorPalette="red"
                variant="outline"
                onClick={handleDelete}
                disabled={lookup.deleting}
              >
                {lookup.deleting ? "Deleting..." : "Delete definition"}
              </Button>
            </Box>
          ) : lookup.deleted ? (
            <Box>
              <Text color="fg.muted" fontSize="sm">
                Definition deleted.
              </Text>
              <Button
                size="xs"
                variant="ghost"
                mt={2}
                onClick={() => setLookup(null)}
              >
                Close
              </Button>
            </Box>
          ) : (
            <Box>
              <Box
                display="flex"
                justifyContent="space-between"
                alignItems="center"
                mb={3}
              >
                <Heading size="sm">
                  {lookup.word}
                  {lookup.source && (
                    <Text
                      as="span"
                      fontWeight="normal"
                      fontSize="xs"
                      color="fg.muted"
                      ml={2}
                    >
                      ({lookup.source})
                    </Text>
                  )}
                </Heading>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => setLookup(null)}
                >
                  Close
                </Button>
              </Box>

              {lookup.loading && (
                <Box textAlign="center" py={4}>
                  <Spinner size="sm" />
                  <Text fontSize="sm" color="fg.muted" mt={2}>
                    Looking up...
                  </Text>
                </Box>
              )}

              {lookup.error && (
                <Text color="red.500" fontSize="sm">
                  {lookup.error}
                </Text>
              )}

              {lookup.saved && (
                <Text color="green.600" fontSize="sm" mb={2}>
                  Definition saved.
                </Text>
              )}

              {!lookup.loading &&
                lookup.definitions.length === 0 &&
                !lookup.error && (
                  <Text color="fg.muted" fontSize="sm">
                    No definitions found.
                  </Text>
                )}

              <VStack align="stretch" gap={3}>
                {lookup.definitions.map((def, i) => (
                  <Box
                    key={i}
                    p={3}
                    borderWidth="1px"
                    borderRadius="md"
                    fontSize="sm"
                  >
                    {def.partOfSpeech && (
                      <Text fontSize="xs" color="fg.muted" mb={1}>
                        {def.partOfSpeech}
                      </Text>
                    )}
                    <Text mb={1}>{def.definition}</Text>
                    {def.pronunciation && (
                      <Text fontSize="xs" color="fg.muted" mb={1}>
                        /{def.pronunciation}/
                      </Text>
                    )}
                    {def.examples.length > 0 && (
                      <Box mt={1}>
                        {def.examples.map((ex, j) => (
                          <Text key={j} fontSize="xs" color="fg.muted" pl={2}>
                            {ex}
                          </Text>
                        ))}
                      </Box>
                    )}
                    {def.synonyms.length > 0 && (
                      <Text fontSize="xs" color="fg.muted" mt={1}>
                        Synonyms: {def.synonyms.join(", ")}
                      </Text>
                    )}
                    {!lookup.saved && (
                      <Button
                        size="xs"
                        colorPalette="blue"
                        mt={2}
                        onClick={() => handleSaveDefinition(i)}
                        disabled={lookup.saving}
                      >
                        {lookup.saving ? "Saving..." : "Save"}
                      </Button>
                    )}
                  </Box>
                ))}
              </VStack>
            </Box>
          )}
        </Box>
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

  return (
    <Box>
      {scene.title && (
        <Heading size="sm" mb={2} color="fg.muted">
          {scene.title}
        </Heading>
      )}

      {/* Book text (statements) */}
      {hasStatements && (
        <Box
          onMouseUp={() => onTextSelect(storyIndex, sceneIndex)}
          cursor="text"
          lineHeight="tall"
        >
          <VStack align="stretch" gap={3}>
            {scene.statements.map((stmt, i) => (
              <Text key={i} fontSize="md">
                {renderBookText(stmt, scene.definitions)}
              </Text>
            ))}
          </VStack>
        </Box>
      )}

      {/* Conversations (if any) */}
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
                &ldquo;{renderBookText(conv.quote, scene.definitions)}&rdquo;
              </Text>
            ))}
          </VStack>
        </Box>
      )}
    </Box>
  );
}
