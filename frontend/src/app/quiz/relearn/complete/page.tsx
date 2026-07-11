"use client";

import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { useRelearnStore } from "@/store/relearnStore";

export default function RelearnCompletePage() {
  const router = useRouter();
  const clearedCount = useRelearnStore((s) => s.clearedCount);
  const totalAnswers = useRelearnStore((s) => s.totalAnswers);
  const reset = useRelearnStore((s) => s.reset);

  const goStart = () => {
    reset();
    router.push("/quiz?tab=relearn");
  };
  const goHub = () => {
    reset();
    router.push("/quiz");
  };

  return (
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh" p={4}>
      <Box textAlign="center" py={8}>
        <Text fontSize="3xl">✅</Text>
        <Heading size="md" mt={2}>Relearn complete</Heading>
        <Text fontSize="sm" mt={2}>
          You cleared {clearedCount} {clearedCount === 1 ? "word" : "words"}.
        </Text>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mt={1}>
          Total answers: {totalAnswers}
          {totalAnswers > clearedCount ? " (some came around more than once)" : ""}
        </Text>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mt={3}>
          Nothing was saved to your history or schedule.
        </Text>
      </Box>

      <VStack gap={2}>
        <Button colorPalette="purple" w="full" onClick={goStart}>
          Relearn again
        </Button>
        <Button variant="outline" w="full" onClick={goHub}>
          Quiz Hub
        </Button>
      </VStack>
    </Box>
  );
}
