"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { useQuizStore, WordDetail } from "@/store/quizStore";
import { formatReviewDate } from "@/lib/formatReviewDate";

interface ResultItem {
  key: string;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  wordDetail?: WordDetail;
  nextReviewDate?: string;
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
        nextReviewDate: r.nextReviewDate,
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
        nextReviewDate: r.nextReviewDate,
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
        nextReviewDate: r.nextReviewDate,
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

      {incorrectResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="red.600" _dark={{ color: "red.300" }} mb={2}>
            Incorrect
          </Heading>
          <VStack align="stretch" gap={2}>
            {incorrectResults.map((r) => (
              <ResultCard key={r.key} item={r} />
            ))}
          </VStack>
        </Box>
      )}

      {correctResults.length > 0 && (
        <Box mb={6}>
          <Heading size="md" color="green.600" _dark={{ color: "green.300" }} mb={2}>
            Correct
          </Heading>
          <VStack align="stretch" gap={2}>
            {correctResults.map((r) => (
              <ResultCard key={r.key} item={r} />
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

function ResultCard({ item }: { item: ResultItem }) {
  const [skipped, setSkipped] = useState(false);
  const [overridden, setOverridden] = useState(false);
  const [displayCorrect, setDisplayCorrect] = useState(item.correct);

  return (
    <Box p={2} borderWidth="1px" borderRadius="md">
      <Text fontWeight="bold">{item.entry}</Text>
      <Text fontSize="sm">{item.meaning}</Text>
      {item.contexts?.map((ctx, i) => (
        <Text key={i} fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
          {ctx}
        </Text>
      ))}
      {item.wordDetail && (
        <Box mt={1} fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }}>
          {item.wordDetail.partOfSpeech && (
            <Text><Text as="span" fontWeight="bold">Part of speech:</Text> {item.wordDetail.partOfSpeech}</Text>
          )}
          {item.wordDetail.pronunciation && (
            <Text><Text as="span" fontWeight="bold">Pronunciation:</Text> {item.wordDetail.pronunciation}</Text>
          )}
          {item.wordDetail.origin && (
            <Text><Text as="span" fontWeight="bold">Origin:</Text> {item.wordDetail.origin}</Text>
          )}
          {item.wordDetail.synonyms && item.wordDetail.synonyms.length > 0 && (
            <Text><Text as="span" fontWeight="bold">Synonyms:</Text> {item.wordDetail.synonyms.join(", ")}</Text>
          )}
          {item.wordDetail.antonyms && item.wordDetail.antonyms.length > 0 && (
            <Text><Text as="span" fontWeight="bold">Antonyms:</Text> {item.wordDetail.antonyms.join(", ")}</Text>
          )}
          {item.wordDetail.memo && (
            <Text><Text as="span" fontWeight="bold">Memo:</Text> {item.wordDetail.memo}</Text>
          )}
        </Box>
      )}

      {/* Review date */}
      {item.nextReviewDate && (
        <Box
          mt={2}
          bg="blue.50"
          _dark={{ bg: "blue.900/20", borderColor: "blue.700" }}
          borderWidth="1px"
          borderColor="blue.200"
          borderRadius="md"
          p={2}
        >
          <Text fontSize="xs" fontWeight="medium">
            Next review: {formatReviewDate(item.nextReviewDate)}
          </Text>
        </Box>
      )}

      {/* Override button */}
      {!overridden && !skipped && (
        <Button
          w="full"
          mt={2}
          size="sm"
          variant="outline"
          colorPalette={displayCorrect ? "red" : "blue"}
          onClick={() => {
            setOverridden(true);
            setDisplayCorrect(!displayCorrect);
          }}
        >
          {displayCorrect ? "Mark as Incorrect" : "Mark as Correct"}
        </Button>
      )}

      {overridden && (
        <Text mt={2} fontSize="xs" color="fg.muted" fontStyle="italic">
          {displayCorrect ? "Marked as correct" : "Marked as incorrect"}{" "}
          <Text
            as="span"
            color="blue.600"
            cursor="pointer"
            textDecoration="underline"
            onClick={() => {
              setOverridden(false);
              setDisplayCorrect(item.correct);
            }}
          >
            Undo
          </Text>
        </Text>
      )}

      {/* Skip button or Skipped label */}
      {skipped ? (
        <Text mt={2} fontSize="sm" color="fg.muted" fontStyle="italic">
          Skipped
        </Text>
      ) : !overridden ? (
        <Button
          w="full"
          mt={2}
          size="sm"
          variant="outline"
          colorPalette="gray"
          onClick={() => setSkipped(true)}
        >
          Skip
        </Button>
      ) : null}
    </Box>
  );
}
