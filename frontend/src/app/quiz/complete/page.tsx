"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { useQuizStore } from "@/store/quizStore";

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reset = useQuizStore((s) => s.reset);

  useEffect(() => {
    if (results.length === 0) {
      router.push("/");
    }
  }, [results, router]);

  if (results.length === 0) {
    return null;
  }

  const correctResults = results.filter((r) => r.correct);
  const incorrectResults = results.filter((r) => !r.correct);

  const handleBackToStart = () => {
    reset();
    router.push("/");
  };

  return (
    <Box p={4} maxW="md" mx="auto">
      <Heading size="lg" mb={4}>
        Session Complete
      </Heading>

      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Total: {results.length} words</Text>
        <Text color="green.600" fontWeight="bold">
          Correct: {correctResults.length}
        </Text>
        <Text color="red.600" fontWeight="bold">
          Incorrect: {incorrectResults.length}
        </Text>
      </VStack>

      {correctResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.600" mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correctResults.map((r) => (
              <Box key={r.entry} p={2} borderWidth="1px" borderRadius="md">
                <Text fontWeight="bold">{r.entry}</Text>
                <Text fontSize="sm">{r.meaning}</Text>
              </Box>
            ))}
          </VStack>
        </Box>
      )}

      {incorrectResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.600" mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrectResults.map((r) => (
              <Box key={r.entry} p={2} borderWidth="1px" borderRadius="md">
                <Text fontWeight="bold">{r.entry}</Text>
                <Text fontSize="sm">{r.meaning}</Text>
              </Box>
            ))}
          </VStack>
        </Box>
      )}

      <Button w="full" colorPalette="blue" onClick={handleBackToStart}>
        Back to Start
      </Button>
    </Box>
  );
}
