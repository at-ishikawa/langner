"use client";

import React, { useEffect, useState, useCallback } from "react";
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
            Word list
          </Button>
        </Link>
      </Box>

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
                onTextSelect={onTextSelect}
              />
            ))}
          </VStack>
        </Box>
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
