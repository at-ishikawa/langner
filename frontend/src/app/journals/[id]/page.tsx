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
} from "@/lib/client";
import { SceneContent } from "@/components/SceneContent";

// JournalDetailPage shows a journal's story content with each entry's
// grammar mistakes attached directly beneath the scene they came from, so on
// a long journal you can always tell which mistake belongs to which entry.
// The story is rendered with the shared SceneContent reader (identical to the
// vocabulary /learn/[id] page) minus its word-lookup wiring.
//
// Each mistake carries a deliberate "Exclude from quizzes" / "Resume" control
// (the ONLY thing that writes the grammar skipped_at marker, via
// ExcludeGrammarMistake / ResumeGrammarMistake) — see quiz-ui-invariants
// U1/U2. Nothing else on this page touches the exclude marker.

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

// MistakeList renders a stack of MistakeRow cards for the mistakes attached to
// one scene/entry. Renders nothing when there are none.
function MistakeList({
  notebookId,
  mistakes,
}: {
  notebookId: string;
  mistakes: GrammarMistake[];
}) {
  if (mistakes.length === 0) return null;
  return (
    <VStack align="stretch" gap={2} mt={3}>
      {mistakes.map((m) => (
        <MistakeRow key={m.senseId} notebookId={notebookId} mistake={m} />
      ))}
    </VStack>
  );
}

// placeMistakes assigns every grammar mistake to the story entry / scene it
// came from so each renders adjacent to its source (quiz-ui-invariants U3).
// Both keys derive from the backend story loop variable `sn.Event`: a
// mistake's `title` equals the entry's `event`, and its `entryId` equals
// `${event}#${sceneIndex}` for the specific scene. Mistakes matching a scene
// render under that scene; mistakes matching the entry title but no rendered
// scene index fall to the entry level; anything left over (matching no
// rendered entry — rare) goes to the "Other" bucket so nothing is dropped.
interface StoryPlacement {
  bySceneIndex: Map<number, GrammarMistake[]>;
  entryLevel: GrammarMistake[];
}

function placeMistakes(
  stories: GetNotebookDetailResponse["stories"],
  mistakes: GrammarMistake[],
): { placements: StoryPlacement[]; other: GrammarMistake[] } {
  const used = new Set<string>();
  const placements = stories.map((story) => {
    const entryMistakes = mistakes.filter(
      (m) => m.title === story.event && !used.has(m.senseId),
    );
    const bySceneIndex = new Map<number, GrammarMistake[]>();
    story.scenes.forEach((_, sceneIndex) => {
      const key = `${story.event}#${sceneIndex}`;
      const sceneMistakes = entryMistakes.filter((m) => m.entryId === key);
      if (sceneMistakes.length === 0) return;
      bySceneIndex.set(sceneIndex, sceneMistakes);
      sceneMistakes.forEach((m) => used.add(m.senseId));
    });
    const entryLevel = entryMistakes.filter((m) => !used.has(m.senseId));
    entryLevel.forEach((m) => used.add(m.senseId));
    return { bySceneIndex, entryLevel };
  });
  const other = mistakes.filter((m) => !used.has(m.senseId));
  return { placements, other };
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

  const { placements, other } = placeMistakes(data.stories, mistakes);

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

      {/* Story content, each entry's grammar mistakes attached beneath the
          scene they came from. */}
      <VStack align="stretch" gap={8}>
        {data.stories.map((story, storyIndex) => {
          const placement = placements[storyIndex];
          return (
            <Box key={storyIndex}>
              {story.event && (
                <Heading size="md" mb={3}>
                  {story.event}
                </Heading>
              )}
              <VStack align="stretch" gap={6}>
                {story.scenes.map((scene, sceneIndex) => (
                  <Box key={sceneIndex}>
                    <SceneContent scene={scene} />
                    <MistakeList
                      notebookId={id}
                      mistakes={placement.bySceneIndex.get(sceneIndex) ?? []}
                    />
                  </Box>
                ))}
              </VStack>
              {/* Mistakes tied to this entry but not to a rendered scene. */}
              <MistakeList notebookId={id} mistakes={placement.entryLevel} />
            </Box>
          );
        })}
      </VStack>

      {/* Any mistake whose entry no longer exists in the story — rare, but
          surfaced here so nothing is silently dropped. */}
      {other.length > 0 && (
        <Box mt={8}>
          <Heading size="md" mb={3}>
            Other grammar mistakes
          </Heading>
          <MistakeList notebookId={id} mistakes={other} />
        </Box>
      )}
    </Box>
  );
}
