"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Text, VStack } from "@chakra-ui/react";
import { PageShell } from "@/components/PageShell";
import { useGrammarStore } from "@/store/grammarStore";
import { GrammarResultsGroupedList } from "@/components/GrammarResultsGroupedList";
import { useGrammarResultActions } from "@/lib/useGrammarResultActions";

// Grammar session summary. Every graded blank across all posts is grouped into
// Incorrect / Correct / Excluded with the labelled+diffed grammar card and the
// same Mark-correct / Undo / Exclude footer.
export default function GrammarCompletePage() {
  const router = useRouter();
  const results = useGrammarStore((s) => s.results);
  const reset = useGrammarStore((s) => s.reset);

  useEffect(() => {
    if (results.length === 0) router.replace("/quiz?tab=grammar");
  }, [results.length, router]);

  const { handleOverride, handleUndo, handleSkip, handleResume } = useGrammarResultActions();

  if (results.length === 0) return null;

  const correctCount = results.filter((r) => r.correct && !r.isSkipped).length;
  const incorrectCount = results.filter((r) => !r.correct && !r.isSkipped).length;

  const handleBackToStart = () => {
    reset();
    router.push("/quiz?tab=grammar");
  };

  return (
    <PageShell title="Session summary" maxW="xl" stickyFooter>
      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Answered: {results.length} corrections</Text>
        <Text color="green.fg" fontWeight="bold">
          Correct: {correctCount}
        </Text>
        <Text color="red.fg" fontWeight="bold">
          Incorrect: {incorrectCount}
        </Text>
      </VStack>

      <GrammarResultsGroupedList
        results={results}
        onOverride={handleOverride}
        onUndo={handleUndo}
        onSkip={handleSkip}
        onResume={handleResume}
      />

      <Box
        position="sticky"
        bottom={0}
        bg="bg"
        borderTopWidth="1px"
        borderTopColor="border"
        mx={-4}
        px={4}
        py={3}
        mt={6}
      >
        <Button w="full" colorPalette="brand" onClick={handleBackToStart}>
          Done
        </Button>
      </Box>
    </PageShell>
  );
}
