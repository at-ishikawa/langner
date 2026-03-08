"use client";

import { useEffect, useState } from "react";
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
import { LearningStatusBadge } from "@/components/LearningStatusBadge";
import { PdfPreviewModal } from "@/components/PdfPreviewModal";

type StatusFilter =
  | "all"
  | ""
  | "misunderstood"
  | "understood"
  | "usable"
  | "intuitive";

const filterOptions: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "", label: "Learning" },
  { value: "misunderstood", label: "Misunderstood" },
  { value: "understood", label: "Understood" },
  { value: "usable", label: "Usable" },
  { value: "intuitive", label: "Intuitive" },
];

function renderQuote(quote: string) {
  const parts = quote.split(/\{\{\s*([^}]+?)\s*\}\}/);
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <Text as="span" key={i} fontWeight="bold" color="blue.600">
        {part.trim()}
      </Text>
    ) : (
      <Text as="span" key={i}>
        {part}
      </Text>
    ),
  );
}

function matchCount(definitions: NotebookWord[], filter: StatusFilter): number {
  if (filter === "all") return definitions.length;
  return definitions.filter((w) => w.learningStatus === filter).length;
}

function storyMatchCount(story: StoryEntry, filter: StatusFilter): number {
  return story.scenes.reduce(
    (sum, scene) => sum + matchCount(scene.definitions, filter),
    0,
  );
}

// Flashcard notebooks have a single unnamed scene — skip that level
function isFlatStory(story: StoryEntry): boolean {
  return story.scenes.length === 1 && !story.scenes[0].title;
}

export default function NotebookDetailPage() {
  const params = useParams();
  const id = params.id as string;

  const [data, setData] = useState<GetNotebookDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [selectedStory, setSelectedStory] = useState<StoryEntry | null>(null);
  const [pdfOpen, setPdfOpen] = useState(false);

  useEffect(() => {
    notebookClient
      .getNotebookDetail({ notebookId: id })
      .then((res) => setData(res))
      .catch(() => setError("Failed to load notebook"))
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) {
    return (
      <Box p={4} maxW="2xl" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error || !data) {
    return (
      <Box p={4} maxW="2xl" mx="auto">
        <Text color="red.500">{error ?? "Notebook not found"}</Text>
      </Box>
    );
  }

  const visibleStories = data.stories.filter(
    (story) => storyMatchCount(story, filter) > 0,
  );

  // Story detail view
  if (selectedStory) {
    return (
      <Box p={4} maxW="2xl" mx="auto">
        <Box mb={2}>
          <Text
            color="blue.600"
            fontSize="sm"
            cursor="pointer"
            onClick={() => setSelectedStory(null)}
          >
            ← {data.name}
          </Text>
        </Box>

        <Box mb={4}>
          <Heading size="md">{selectedStory.event}</Heading>
          {selectedStory.metadata &&
            (selectedStory.metadata.series ||
              selectedStory.metadata.season > 0) && (
              <Text fontSize="xs" color="fg.muted">
                {selectedStory.metadata.series}
                {selectedStory.metadata.season > 0 &&
                  ` S${String(selectedStory.metadata.season).padStart(2, "0")}E${String(selectedStory.metadata.episode).padStart(2, "0")}`}
              </Text>
            )}
        </Box>

        <Box mb={4}>
          <select
            style={{
              width: "100%",
              padding: "0.5rem",
              border: "1px solid",
              borderRadius: "0.375rem",
              fontSize: "0.875rem",
            }}
            value={filter}
            onChange={(e) => setFilter(e.target.value as StatusFilter)}
          >
            {filterOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </Box>

        {isFlatStory(selectedStory) ? (
          // Flashcard-style: no scene level, show words directly
          <VStack align="stretch" gap={2}>
            {selectedStory.scenes[0].definitions
              .filter((w) => filter === "all" || w.learningStatus === filter)
              .map((word, i) => (
                <WordCard key={i} word={word} />
              ))}
          </VStack>
        ) : (
          // Story-style: show scenes
          <VStack align="stretch" gap={2}>
            {selectedStory.scenes
              .filter((scene) => matchCount(scene.definitions, filter) > 0)
              .map((scene, i) => (
                <SceneRow key={i} scene={scene} filter={filter} />
              ))}
          </VStack>
        )}
      </Box>
    );
  }

  // Story list view
  return (
    <Box p={4} maxW="2xl" mx="auto">
      <Box mb={2}>
        <Link href="/notebooks">
          <Text color="blue.600" fontSize="sm">
            ← Back to notebooks
          </Text>
        </Link>
      </Box>

      <Box mb={4}>
        <Box
          display="flex"
          justifyContent="space-between"
          alignItems="flex-start"
          gap={2}
        >
          <Box flex="1">
            <Heading size="lg">{data.name}</Heading>
            <Text fontSize="sm" color="fg.muted">
              {data.totalWordCount} words
            </Text>
          </Box>
          <Button
            size="sm"
            colorPalette="blue"
            onClick={() => setPdfOpen(true)}
          >
            Export PDF
          </Button>
        </Box>
        <Box mt={3}>
          <select
            style={{
              width: "100%",
              padding: "0.5rem",
              border: "1px solid",
              borderRadius: "0.375rem",
              fontSize: "0.875rem",
            }}
            value={filter}
            onChange={(e) => setFilter(e.target.value as StatusFilter)}
          >
            {filterOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </Box>
      </Box>

      <VStack align="stretch" gap={2}>
        {visibleStories.map((story, i) => {
          const total = storyMatchCount(story, "all");
          const matched = storyMatchCount(story, filter);
          return (
            <Box
              key={i}
              p={3}
              borderWidth="1px"
              borderRadius="md"
              cursor="pointer"
              _hover={{ bg: "bg.muted" }}
              onClick={() => setSelectedStory(story)}
              display="flex"
              justifyContent="space-between"
              alignItems="center"
              gap={2}
            >
              <Box flex="1" minW={0}>
                <Text fontWeight="medium" truncate>
                  {story.event}
                </Text>
                {story.metadata &&
                  (story.metadata.series ||
                    story.metadata.season > 0) && (
                    <Text fontSize="xs" color="fg.muted">
                      {story.metadata.series}
                      {story.metadata.season > 0 &&
                        ` S${String(story.metadata.season).padStart(2, "0")}E${String(story.metadata.episode).padStart(2, "0")}`}
                    </Text>
                  )}
              </Box>
              <Box flexShrink={0} textAlign="right">
                <Text fontSize="xs" color="fg.muted">
                  {filter === "all" ? total : `${matched}/${total}`} words
                </Text>
                <Text fontSize="xs" color="fg.muted">›</Text>
              </Box>
            </Box>
          );
        })}
      </VStack>

      {visibleStories.length === 0 && (
        <Text color="fg.muted" textAlign="center" mt={4}>
          No words match the selected filter.
        </Text>
      )}

      <PdfPreviewModal
        notebookId={id}
        isOpen={pdfOpen}
        onClose={() => setPdfOpen(false)}
      />
    </Box>
  );
}

function SceneRow({
  scene,
  filter,
}: {
  scene: StoryScene;
  filter: StatusFilter;
}) {
  const [open, setOpen] = useState(false);
  const total = matchCount(scene.definitions, "all");
  const matched = matchCount(scene.definitions, filter);

  return (
    <Box borderWidth="1px" borderRadius="md" overflow="hidden">
      <Box
        p={3}
        cursor="pointer"
        _hover={{ bg: "bg.muted" }}
        onClick={() => setOpen(!open)}
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        gap={2}
      >
        <Text fontSize="sm" fontWeight="medium" flex="1">
          {scene.title}
        </Text>
        <Box display="flex" alignItems="center" gap={2} flexShrink={0}>
          <Text fontSize="xs" color="fg.muted">
            {filter === "all" ? total : `${matched}/${total}`} words
          </Text>
          <Text fontSize="xs" color="fg.muted">
            {open ? "▲" : "▼"}
          </Text>
        </Box>
      </Box>

      {open && (
        <Box borderTopWidth="1px" p={3}>
          {scene.conversations.length > 0 && (
            <VStack align="stretch" gap={1} mb={3}>
              {scene.conversations.map((conv, i) => (
                <Text key={i} fontSize="sm" color="fg.muted">
                  <Text as="span" fontWeight="bold" color="fg.default">
                    {conv.speaker}:
                  </Text>{" "}
                  &ldquo;{renderQuote(conv.quote)}&rdquo;
                </Text>
              ))}
            </VStack>
          )}
          <VStack align="stretch" gap={2}>
            {scene.definitions
              .filter((w) => filter === "all" || w.learningStatus === filter)
              .map((word, i) => (
                <WordCard key={i} word={word} />
              ))}
          </VStack>
        </Box>
      )}
    </Box>
  );
}

function WordCard({ word }: { word: NotebookWord }) {
  const [open, setOpen] = useState(false);

  const lastLog =
    word.learnedLogs.length > 0
      ? word.learnedLogs[word.learnedLogs.length - 1]
      : null;

  return (
    <Box borderWidth="1px" borderRadius="md" overflow="hidden" fontSize="sm">
      <Box
        p={3}
        cursor="pointer"
        _hover={{ bg: "bg.muted" }}
        onClick={() => setOpen(!open)}
      >
        <Box
          display="flex"
          justifyContent="space-between"
          alignItems="flex-start"
          gap={2}
        >
          <Text fontWeight="semibold" flex="1">
            {word.expression}
          </Text>
          <LearningStatusBadge status={word.learningStatus} />
        </Box>
        <Text color="fg.muted" mt={1}>
          {word.meaning || word.definition}
        </Text>
        {word.nextReviewDate && (
          <Text fontSize="xs" color="fg.subtle" mt={1}>
            Next review: {word.nextReviewDate}
          </Text>
        )}
      </Box>

      {open && (
        <Box p={3} pt={0} borderTopWidth="1px" bg="bg.subtle">
          <VStack align="stretch" gap={2} fontSize="sm" pt={3}>
            {word.partOfSpeech && (
              <Text>
                <Text as="span" fontWeight="bold">
                  Part of speech:
                </Text>{" "}
                {word.partOfSpeech}
              </Text>
            )}
            {word.pronunciation && (
              <Text>
                <Text as="span" fontWeight="bold">
                  Pronunciation:
                </Text>{" "}
                {word.pronunciation}
              </Text>
            )}
            {word.examples.length > 0 && (
              <Box>
                <Text fontWeight="bold">Examples:</Text>
                {word.examples.map((ex, i) => (
                  <Text key={i} pl={2} color="fg.muted">
                    {ex}
                  </Text>
                ))}
              </Box>
            )}
            {word.synonyms.length > 0 && (
              <Text>
                <Text as="span" fontWeight="bold">
                  Synonyms:
                </Text>{" "}
                {word.synonyms.join(", ")}
              </Text>
            )}
            {word.learnedLogs.length > 0 && (
              <Box>
                <Text fontWeight="bold" mb={1}>
                  Learning History:
                </Text>
                <VStack align="stretch" gap={1}>
                  {word.learnedLogs.map((log, i) => (
                    <Box
                      key={i}
                      display="flex"
                      gap={2}
                      alignItems="center"
                      fontSize="xs"
                    >
                      <Text color="fg.muted" minW="80px">
                        {log.learnedAt || "-"}
                      </Text>
                      <LearningStatusBadge status={log.status} />
                      <Text color="fg.muted">Q:{log.quality}</Text>
                      <Text color="fg.muted">{log.intervalDays}d</Text>
                    </Box>
                  ))}
                </VStack>
              </Box>
            )}
            {lastLog && (
              <Text fontSize="xs" color="fg.muted">
                Last studied: {lastLog.learnedAt || "-"}
              </Text>
            )}
          </VStack>
        </Box>
      )}
    </Box>
  );
}
