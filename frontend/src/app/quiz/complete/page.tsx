"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { useQuizStore, WordDetail } from "@/store/quizStore";

interface ResultItem {
  key: string;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  wordDetail?: WordDetail;
}

export default function SessionCompletePage() {
  const router = useRouter();
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const reset = useQuizStore((s) => s.reset);

  const allResults = useMemo((): ResultItem[] => {
    if (results.length > 0) {
      return results.map((r) => ({
        key: r.noteId.toString(),
        entry: r.entry,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        wordDetail: r.wordDetail,
      }));
    }
    if (reverseResults.length > 0) {
      return reverseResults.map((r) => ({
        key: r.noteId.toString(),
        entry: r.expression,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        wordDetail: r.wordDetail,
      }));
    }
    if (freeformResults.length > 0) {
      return freeformResults.map((r, i) => ({
        key: `freeform-${i}`,
        entry: r.word,
        meaning: r.meaning,
        correct: r.correct,
        contexts: r.contexts,
        wordDetail: r.wordDetail,
      }));
    }
    return [];
  }, [results, reverseResults, freeformResults]);

  useEffect(() => {
    if (allResults.length === 0) {
      router.push("/");
    }
  }, [allResults, router]);

  if (allResults.length === 0) {
    return null;
  }

  const correctResults = allResults.filter((r) => r.correct);
  const incorrectResults = allResults.filter((r) => !r.correct);

  const handleBackToStart = () => {
    reset();
    router.push("/");
  };

  const renderResultCard = (r: ResultItem) => (
    <Box key={r.key} p={2} borderWidth="1px" borderRadius="md">
      <Text fontWeight="bold">{r.entry}</Text>
      <Text fontSize="sm">{r.meaning}</Text>
      {r.contexts?.map((ctx, i) => (
        <Text key={i} fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
          {ctx}
        </Text>
      ))}
      {r.wordDetail && (
        <Box mt={1} fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }}>
          {r.wordDetail.partOfSpeech && (
            <Text><Text as="span" fontWeight="bold">Part of speech:</Text> {r.wordDetail.partOfSpeech}</Text>
          )}
          {r.wordDetail.pronunciation && (
            <Text><Text as="span" fontWeight="bold">Pronunciation:</Text> {r.wordDetail.pronunciation}</Text>
          )}
          {r.wordDetail.origin && (
            <Text><Text as="span" fontWeight="bold">Origin:</Text> {r.wordDetail.origin}</Text>
          )}
          {r.wordDetail.synonyms && r.wordDetail.synonyms.length > 0 && (
            <Text><Text as="span" fontWeight="bold">Synonyms:</Text> {r.wordDetail.synonyms.join(", ")}</Text>
          )}
          {r.wordDetail.antonyms && r.wordDetail.antonyms.length > 0 && (
            <Text><Text as="span" fontWeight="bold">Antonyms:</Text> {r.wordDetail.antonyms.join(", ")}</Text>
          )}
          {r.wordDetail.memo && (
            <Text><Text as="span" fontWeight="bold">Memo:</Text> {r.wordDetail.memo}</Text>
          )}
        </Box>
      )}
    </Box>
  );

  return (
    <Box p={4} maxW="md" mx="auto">
      <Heading size="lg" mb={4}>
        Session Complete
      </Heading>

      <VStack align="stretch" gap={3} mb={6}>
        <Text fontWeight="bold">Total: {allResults.length} words</Text>
        <Text color="green.600" _dark={{ color: "green.300" }} fontWeight="bold">
          Correct: {correctResults.length}
        </Text>
        <Text color="red.600" _dark={{ color: "red.300" }} fontWeight="bold">
          Incorrect: {incorrectResults.length}
        </Text>
      </VStack>

      {correctResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.600" _dark={{ color: "green.300" }} mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correctResults.map(renderResultCard)}
          </VStack>
        </Box>
      )}

      {incorrectResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.600" _dark={{ color: "red.300" }} mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrectResults.map(renderResultCard)}
          </VStack>
        </Box>
      )}

      <Button w="full" colorPalette="blue" onClick={handleBackToStart}>
        Back to Start
      </Button>
    </Box>
  );
}
