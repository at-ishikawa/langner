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
  quizClient,
  type GetNotebookDetailResponse,
  type GrammarMistake,
  type StoryEntry,
} from "@/lib/client";

// JournalDetailPage shows a journal's story prose plus its grammar-mistake
// review list. Each mistake carries a deliberate "Exclude from quizzes" /
// "Resume" control (the ONLY thing that writes the grammar skipped_at marker,
// via ExcludeGrammarMistake / ResumeGrammarMistake) — see quiz-ui-invariants
// U1/U2. Nothing else on this page touches the exclude marker.

// StoryProse renders a journal entry's scenes as plain prose (statements +
// dialogue). Mirrors the /learn/[id] reader's structure without its
// word-lookup / highlight machinery, which a journal review view doesn't need.
function StoryProse({ story }: { story: StoryEntry }) {
  return (
    <Box>
      {story.event && (
        <Heading size="sm" mb={2} color="fg.muted">
          {story.event}
        </Heading>
      )}
      <VStack align="stretch" gap={4}>
        {story.scenes.map((scene, si) => {
          const hasStatements = scene.statements.length > 0;
          const hasConversations = scene.conversations.length > 0;
          if (!hasStatements && !hasConversations) return null;
          return (
            <Box key={si}>
              {scene.title && (
                <Heading size="xs" mb={1} color="fg.muted">
                  {scene.title}
                </Heading>
              )}
              {hasStatements && (
                <VStack align="stretch" gap={3} lineHeight="tall">
                  {scene.statements.map((stmt, i) => (
                    <Text key={i} fontSize="md">
                      {stmt}
                    </Text>
                  ))}
                </VStack>
              )}
              {hasConversations && (
                <VStack align="stretch" gap={1} mt={hasStatements ? 3 : 0}>
                  {scene.conversations.map((conv, i) => (
                    <Text key={i} fontSize="sm" color="fg.muted">
                      <Text as="span" fontWeight="bold" color="fg.default">
                        {conv.speaker}:
                      </Text>{" "}
                      &ldquo;{conv.quote}&rdquo;
                    </Text>
                  ))}
                </VStack>
              )}
            </Box>
          );
        })}
      </VStack>
    </Box>
  );
}

// MistakeRow renders one grammar correction as a review row and owns the
// deliberate exclude/resume action. `excluded` is optimistic: it flips on
// click and reverts if the RPC fails. Only an excluded row is labelled
// "Excluded" (invariant U1).
function MistakeRow({
  notebookId,
  mistake,
}: {
  notebookId: string;
  mistake: GrammarMistake;
}) {
  const [excluded, setExcluded] = useState(mistake.isExcluded);
  const [busy, setBusy] = useState(false);

  async function toggle() {
    if (busy) return;
    const next = !excluded;
    setExcluded(next);
    setBusy(true);
    try {
      if (next) {
        await quizClient.excludeGrammarMistake({ notebookId, senseId: mistake.senseId });
      } else {
        await quizClient.resumeGrammarMistake({ notebookId, senseId: mistake.senseId });
      }
    } catch {
      setExcluded(!next); // revert on failure
    } finally {
      setBusy(false);
    }
  }

  return (
    <Box
      borderWidth="1px"
      borderColor="gray.200"
      _dark={{ bg: "gray.800", borderColor: "gray.600" }}
      bg="white"
      borderRadius="md"
      p={3}
      opacity={excluded ? 0.7 : 1}
    >
      <Box display="flex" alignItems="center" gap={2} mb={2} flexWrap="wrap">
        {excluded && (
          <Box bg="gray.100" _dark={{ bg: "gray.700" }} px={2} py={0.5} borderRadius="full">
            <Text fontSize="xs" fontWeight="medium" color="fg.muted">
              Excluded
            </Text>
          </Box>
        )}
        {mistake.category && (
          <Text fontSize="xs" color="fg.muted" ml={excluded ? 0 : "auto"}>
            {mistake.category}
          </Text>
        )}
      </Box>

      {/* Mistake (struck) vs correction. */}
      <Box display="flex" gap={2} mb={1}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          Mistake
        </Text>
        <Text
          fontSize="sm"
          color="fg.muted"
          textDecoration="line-through"
          wordBreak="break-word"
        >
          {mistake.incorrect}
        </Text>
      </Box>
      <Box display="flex" gap={2} mb={mistake.reason ? 2 : 0}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          Correct
        </Text>
        <Text
          fontSize="sm"
          color="green.700"
          _dark={{ color: "green.300" }}
          fontWeight="medium"
          wordBreak="break-word"
        >
          {mistake.correct}
        </Text>
      </Box>

      {mistake.reason && (
        <Box mb={3}>
          <Text fontSize="xs" color="fg.muted">
            Why
          </Text>
          <Text fontSize="sm" color="fg.muted" fontStyle="italic">
            {mistake.reason}
          </Text>
        </Box>
      )}

      {/* Deliberate exclude/resume — the only writer of the grammar skip
          marker on this surface (quiz-ui-invariants U1/U2). */}
      <Box>
        {excluded ? (
          <Button size="sm" variant="outline" colorPalette="blue" onClick={toggle} disabled={busy}>
            Resume
          </Button>
        ) : (
          <Button size="sm" variant="outline" colorPalette="gray" onClick={toggle} disabled={busy}>
            Exclude from quizzes
          </Button>
        )}
      </Box>
    </Box>
  );
}

// groupMistakesByTitle preserves the order in which titles first appear.
function groupMistakesByTitle(
  mistakes: GrammarMistake[],
): { title: string; mistakes: GrammarMistake[] }[] {
  const out: { title: string; mistakes: GrammarMistake[] }[] = [];
  const indexByTitle = new Map<string, number>();
  for (const m of mistakes) {
    const title = m.title || "Untitled entry";
    const idx = indexByTitle.get(title);
    if (idx === undefined) {
      indexByTitle.set(title, out.length);
      out.push({ title, mistakes: [m] });
      continue;
    }
    out[idx].mistakes.push(m);
  }
  return out;
}

export default function JournalDetailPage() {
  const params = useParams();
  const id = params.id as string;

  const [data, setData] = useState<GetNotebookDetailResponse | null>(null);
  const [mistakes, setMistakes] = useState<GrammarMistake[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      notebookClient.getNotebookDetail({ notebookId: id }),
      quizClient
        .listGrammarMistakes({ notebookId: id, sectionTitles: [] })
        .catch(() => null), // grammar list is optional; story still renders
    ])
      .then(([detail, grammar]) => {
        setData(detail);
        setMistakes(grammar?.mistakes ?? []);
      })
      .catch(() => setError("Failed to load journal"))
      .finally(() => setLoading(false));
  }, [id]);

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
        <Box mb={2}>
          <Link href="/learn">
            <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="sm">
              &larr; Back to Learn
            </Text>
          </Link>
        </Box>
        <Text color="red.500">{error ?? "Journal not found"}</Text>
      </Box>
    );
  }

  const grouped = groupMistakesByTitle(mistakes);

  return (
    <Box p={4} maxW="3xl" mx="auto">
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

      {/* Story prose */}
      {data.stories.length > 0 && (
        <VStack align="stretch" gap={6} mb={8}>
          {data.stories.map((story, i) => (
            <StoryProse key={i} story={story} />
          ))}
        </VStack>
      )}

      {/* Grammar mistakes review */}
      <Heading size="md" mb={3}>
        Grammar mistakes
      </Heading>
      {grouped.length === 0 ? (
        <Text color="fg.muted">No grammar mistakes recorded for this journal.</Text>
      ) : (
        <VStack align="stretch" gap={5}>
          {grouped.map((group, gi) => (
            <Box key={gi}>
              <Text fontSize="sm" fontWeight="medium" color="fg.muted" mb={2}>
                {group.title}
              </Text>
              <VStack align="stretch" gap={2}>
                {group.mistakes.map((m) => (
                  <MistakeRow key={m.senseId} notebookId={id} mistake={m} />
                ))}
              </VStack>
            </Box>
          ))}
        </VStack>
      )}
    </Box>
  );
}
