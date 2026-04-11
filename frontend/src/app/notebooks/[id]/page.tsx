"use client";

import React from "react";
import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "next/navigation";
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
  quizClient,
  type GetNotebookDetailResponse,
  type NotebookWord,
  type StoryEntry,
  type StoryScene,
  type EtymologyOriginPart,
  type EtymologyDefinition,
} from "@/lib/client";
import { LearningStatusBadge } from "@/components/LearningStatusBadge";
import { PdfPreviewModal } from "@/components/PdfPreviewModal";
import { formatReviewDate } from "@/lib/formatReviewDate";

type OriginPartsMap = Map<string, { parts: EtymologyOriginPart[]; etymologyNotebookId: string }>;

type StatusFilter =
  | "all"
  | ""
  | "misunderstood"
  | "understood"
  | "usable"
  | "intuitive"
  | "skipped";

const filterOptions: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "", label: "Learning" },
  { value: "misunderstood", label: "Misunderstood" },
  { value: "understood", label: "Understood" },
  { value: "usable", label: "Usable" },
  { value: "intuitive", label: "Intuitive" },
  { value: "skipped", label: "Skipped" },
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
  if (filter === "skipped") return definitions.filter((w) => w.isSkipped).length;
  return definitions.filter((w) => w.learningStatus === filter && !w.isSkipped).length;
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

// hasReadableContent returns true when any scene carries prose or dialogue —
// i.e., the content reader at /learn/[id] has something to render.
function hasReadableContent(data: GetNotebookDetailResponse): boolean {
  return data.stories.some((story) =>
    story.scenes.some(
      (scene) =>
        scene.statements.length > 0 || scene.conversations.length > 0,
    ),
  );
}

function findProseContext(
  statements: string[],
  expression: string,
): string | null {
  const lower = expression.toLowerCase();
  for (const stmt of statements) {
    if (stmt.toLowerCase().includes(lower)) {
      const idx = stmt.toLowerCase().indexOf(lower);
      const start = Math.max(0, idx - 80);
      const end = Math.min(stmt.length, idx + expression.length + 80);
      let excerpt = stmt.slice(start, end);
      if (start > 0) excerpt = "\u2026" + excerpt;
      if (end < stmt.length) excerpt = excerpt + "\u2026";
      return excerpt;
    }
  }
  return null;
}

function highlightExcerpt(
  excerpt: string,
  expression: string,
): React.ReactNode[] {
  const regex = new RegExp(
    `(${expression.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`,
    "gi",
  );
  const parts = excerpt.split(regex);
  // split() with a capturing group returns matched parts at odd indices
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <Text as="span" key={i} fontWeight="bold" color="blue.600">
        {part}
      </Text>
    ) : (
      <Text as="span" key={i}>
        {part}
      </Text>
    ),
  );
}

async function loadEtymologyData(notebookName: string): Promise<OriginPartsMap> {
  const opts = await quizClient.getQuizOptions({});
  const etymologyNotebooks = (opts.notebooks ?? []).filter(
    (n) => n.kind === "Etymology",
  );
  if (etymologyNotebooks.length === 0) return new Map();

  const results = await Promise.all(
    etymologyNotebooks.map((nb) =>
      notebookClient
        .getEtymologyNotebook({ notebookId: nb.notebookId })
        .then((res) => ({ notebookId: nb.notebookId, definitions: res.definitions ?? [] }))
        .catch(() => ({ notebookId: nb.notebookId, definitions: [] as EtymologyDefinition[] })),
    ),
  );

  const map: OriginPartsMap = new Map();
  for (const { notebookId, definitions } of results) {
    for (const def of definitions) {
      if (
        def.originParts.length > 0 &&
        def.notebookName === notebookName
      ) {
        map.set(def.expression, {
          parts: def.originParts,
          etymologyNotebookId: notebookId,
        });
      }
    }
  }
  return map;
}

export default function NotebookDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const searchParams = useSearchParams();

  const [data, setData] = useState<GetNotebookDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [pickedStory, setPickedStory] = useState<StoryEntry | null>(null);
  const [pdfOpen, setPdfOpen] = useState(false);
  const [originPartsMap, setOriginPartsMap] = useState<OriginPartsMap>(new Map());

  useEffect(() => {
    notebookClient
      .getNotebookDetail({ notebookId: id })
      .then((res) => {
        setData(res);
        // Load etymology data to show origin_parts on word cards
        loadEtymologyData(res.name).then(setOriginPartsMap).catch(() => {
          // Etymology data is optional; ignore errors
        });
      })
      .catch(() => setError("Failed to load notebook"))
      .finally(() => setLoading(false));
  }, [id]);

  // Deep-link to chapter from search params — derived without a setState effect
  const chapter = searchParams.get("chapter");
  const selectedStory =
    pickedStory ??
    (data && chapter
      ? (data.stories.find((s) => s.event === chapter) ?? null)
      : null);

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
            onClick={() => setPickedStory(null)}
          >
            &larr; {data.name}
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
              .filter((w) => filter === "all" || (filter === "skipped" ? w.isSkipped : w.learningStatus === filter && !w.isSkipped))
              .map((word, i) => (
                <WordCard key={i} word={word} originPartsMap={originPartsMap} />
              ))}
          </VStack>
        ) : (
          // Story-style: show scenes
          <VStack align="stretch" gap={2}>
            {selectedStory.scenes
              .filter((scene) => matchCount(scene.definitions, filter) > 0)
              .map((scene, i) => (
                <SceneRow key={i} scene={scene} filter={filter} originPartsMap={originPartsMap} />
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
        <Link href="/learn">
          <Text color="blue.600" fontSize="sm">
            &larr; Back to Learn
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
              {filter === "all"
                ? `${data.totalWordCount} words`
                : `${data.stories.reduce((sum, s) => sum + storyMatchCount(s, filter), 0)} words`}
            </Text>
          </Box>
          <Box display="flex" gap={2}>
            {hasReadableContent(data) && (
              <Link href={`/learn/${id}`}>
                <Button size="sm" variant="outline">
                  Read
                </Button>
              </Link>
            )}
            <Button
              size="sm"
              colorPalette="blue"
              onClick={() => setPdfOpen(true)}
            >
              Export PDF
            </Button>
          </Box>
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
              onClick={() => setPickedStory(story)}
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
                  {filter === "all" ? total : matched} words
                </Text>
                <Text fontSize="xs" color="fg.muted">
                  &rsaquo;
                </Text>
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
  originPartsMap,
}: {
  scene: StoryScene;
  filter: StatusFilter;
  originPartsMap: OriginPartsMap;
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
        <Text fontSize="sm" fontWeight="medium" flex="1" truncate>
          {scene.title ||
            scene.definitions.map((d) => d.expression).join(" · ")}
        </Text>
        <Box display="flex" alignItems="center" gap={2} flexShrink={0}>
          <Text fontSize="xs" color="fg.muted">
            {filter === "all" ? total : matched} words
          </Text>
          <Text fontSize="xs" color="fg.muted">
            {open ? "\u25B2" : "\u25BC"}
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
              .filter((w) => filter === "all" || (filter === "skipped" ? w.isSkipped : w.learningStatus === filter && !w.isSkipped))
              .map((word, i) => {
                const excerpt = findProseContext(
                  scene.statements,
                  word.expression,
                );
                return (
                  <Box key={i}>
                    {excerpt && (
                      <Box
                        borderLeftWidth="2px"
                        borderColor="blue.200"
                        pl={2}
                        mb={1}
                        _dark={{ borderColor: "blue.700" }}
                      >
                        <Text fontSize="xs" color="fg.muted" fontStyle="italic">
                          {highlightExcerpt(excerpt, word.expression)}
                        </Text>
                      </Box>
                    )}
                    <WordCard word={word} originPartsMap={originPartsMap} />
                  </Box>
                );
              })}
          </VStack>
        </Box>
      )}
    </Box>
  );
}

// TODO: Add a "Resume" button for skipped words. The ResumeWord RPC requires a
// note_id, but the NotebookWord proto does not expose note_id. A proto change is
// needed to add note_id to NotebookWord before this feature can be implemented.

function WordCard({
  word,
  originPartsMap,
}: {
  word: NotebookWord;
  originPartsMap: OriginPartsMap;
}) {
  const [open, setOpen] = useState(false);

  const lastLog =
    word.learnedLogs.length > 0
      ? word.learnedLogs[word.learnedLogs.length - 1]
      : null;

  const etymologyData = originPartsMap.get(word.expression);

  return (
    <Box
      borderWidth="1px"
      borderRadius="md"
      overflow="hidden"
      fontSize="sm"
      opacity={word.isSkipped ? 0.6 : 1}
    >
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
          {word.isSkipped ? (
            <Box bg="gray.100" _dark={{ bg: "gray.700" }} px={2} py={0.5} borderRadius="sm">
              <Text fontSize="xs" color="fg.muted" fontStyle="italic">Skipped</Text>
            </Box>
          ) : (
            <LearningStatusBadge status={word.learningStatus} />
          )}
        </Box>
        <Text color="fg.muted" mt={1}>
          {word.meaning || word.definition}
        </Text>
        {etymologyData && (
          <Box mt={2}>
            <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mb={1}>
              Origins:
            </Text>
            <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
              {etymologyData.parts.map((part, j) => (
                <Box key={j} display="flex" alignItems="center" gap={1}>
                  {j > 0 && (
                    <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
                      +
                    </Text>
                  )}
                  <Link
                    href={`/notebooks/etymology/${etymologyData.etymologyNotebookId}?origin=${encodeURIComponent(part.origin)}`}
                  >
                    <Box
                      display="inline-flex"
                      alignItems="center"
                      gap={1}
                      px={2}
                      py={0.5}
                      borderRadius="full"
                      borderWidth="1px"
                      borderColor="blue.600"
                      bg="blue.50"
                      _dark={{ bg: "blue.900", borderColor: "blue.400" }}
                      cursor="pointer"
                      _hover={{ bg: "blue.100" }}
                    >
                      <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">
                        {part.origin}
                      </Text>
                      {part.language && (
                        <Box
                          px={1.5}
                          py={0}
                          borderRadius="full"
                          bg="gray.100"
                          _dark={{ bg: "gray.700", color: "gray.300" }}
                          fontSize="2xs"
                          color="gray.600"
                        >
                          {part.language}
                        </Box>
                      )}
                    </Box>
                  </Link>
                </Box>
              ))}
            </Box>
          </Box>
        )}
        {word.nextReviewDate && (
          <Text fontSize="xs" color="fg.subtle" mt={1}>
            Next review: {formatReviewDate(word.nextReviewDate)}
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
            {word.origin && (
              <Text>
                <Text as="span" fontWeight="bold">
                  Origin:
                </Text>{" "}
                {word.origin}
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
