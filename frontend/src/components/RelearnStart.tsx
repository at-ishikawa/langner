"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type RelearnCard } from "@/lib/client";
import { useRelearnStore } from "@/store/relearnStore";

const WINDOW_OPTIONS = [
  { hours: 6, label: "6 hours" },
  { hours: 12, label: "12 hours" },
  { hours: 24, label: "24 hours" },
  { hours: 48, label: "48 hours" },
];

// RelearnStart is the Relearn tab's content: pick a look-back window, see how
// many words are pooled, and start. It renders inline under the Quiz hub's tab
// row (no page chrome of its own), so the Relearn tab behaves like the
// Vocabulary / Etymology tabs.
export default function RelearnStart() {
  const router = useRouter();
  const [windowHours, setWindowHours] = useState(24);
  const [cards, setCards] = useState<RelearnCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const seedQueue = useRelearnStore((s) => s.seedQueue);

  const loadPool = useCallback(async (hours: number) => {
    setLoading(true);
    setError(null);
    try {
      const res = await quizClient.startRelearnQuiz({ windowHours: hours });
      setCards(res.cards ?? []);
    } catch {
      setError("Failed to load the relearn pool. Please try again.");
      setCards([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadPool(windowHours);
  }, [windowHours, loadPool]);

  const handleStart = () => {
    if (cards.length === 0) return;
    seedQueue(cards);
    router.push("/quiz/relearn/session");
  };

  return (
    <Box p={4}>
      <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }} mb={4}>
        Re-drill the words you recently got wrong. Nothing here is saved to your history or schedule.
      </Text>

      <Text fontSize="sm" fontWeight="semibold" mb={2}>
        Look back over the last:
      </Text>
      <VStack align="stretch" gap={2} mb={4}>
        {WINDOW_OPTIONS.map((opt) => (
          <Box
            key={opt.hours}
            as="button"
            onClick={() => setWindowHours(opt.hours)}
            textAlign="left"
            px={3}
            py={2}
            borderWidth="1px"
            borderRadius="md"
            borderColor={windowHours === opt.hours ? "purple.500" : "gray.200"}
            bg={windowHours === opt.hours ? "purple.50" : "white"}
            _dark={{
              bg: windowHours === opt.hours ? "purple.900" : "gray.800",
              borderColor: windowHours === opt.hours ? "purple.300" : "gray.600",
            }}
          >
            <Text fontSize="sm">
              {opt.label}
              {opt.hours === 24 ? "  (default)" : ""}
            </Text>
          </Box>
        ))}
      </VStack>

      {loading ? (
        <Box textAlign="center" py={4} aria-live="polite">
          <Spinner size="sm" />
        </Box>
      ) : error ? (
        <Text color="red.500" fontSize="sm" py={2} role="alert">
          {error}
        </Text>
      ) : cards.length === 0 ? (
        <Box textAlign="center" py={4}>
          <Text fontSize="2xl">🎉</Text>
          <Text fontSize="sm" fontWeight="medium" mt={1}>
            Nothing to relearn.
          </Text>
          <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
            You&apos;re all caught up for this window. Try widening it above.
          </Text>
        </Box>
      ) : (
        <Text fontSize="sm" fontWeight="medium" py={2} aria-live="polite">
          {cards.length} {cards.length === 1 ? "word" : "words"} to relearn
        </Text>
      )}

      <Button
        colorPalette="purple"
        w="full"
        mt={2}
        onClick={handleStart}
        disabled={loading || cards.length === 0}
      >
        Start
      </Button>
    </Box>
  );
}
