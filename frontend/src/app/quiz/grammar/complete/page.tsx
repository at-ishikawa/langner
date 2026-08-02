"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
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
    <Box p={4} maxW="sm" mx="auto">
      <Heading size="lg" mb={4}>
        Grammar Session Complete
      </Heading>

      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Total: {results.length} corrections</Text>
        <Text color="green.600" _dark={{ color: "green.300" }} fontWeight="bold">
          Correct: {correctCount}
        </Text>
        <Text color="red.600" _dark={{ color: "red.300" }} fontWeight="bold">
          Incorrect: {incorrectCount}
        </Text>
      </VStack>

      <Box pb={20}>
        <GrammarResultsGroupedList
          results={results}
          onOverride={handleOverride}
          onUndo={handleUndo}
          onSkip={handleSkip}
          onResume={handleResume}
        />
      </Box>

      <Box
        position="sticky"
        bottom={0}
        bg="white"
        _dark={{ bg: "gray.900", borderTopColor: "gray.700" }}
        borderTopWidth="1px"
        borderTopColor="gray.200"
        mx={-4}
        px={4}
        py={3}
      >
        <Button w="full" colorPalette="blue" onClick={handleBackToStart}>
          Back to Start
        </Button>
      </Box>
    </Box>
  );
}
