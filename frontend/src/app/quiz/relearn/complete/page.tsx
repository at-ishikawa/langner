"use client";

import { useRouter } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { PageShell } from "@/components/PageShell";
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
    <PageShell maxW="md">
      <Box textAlign="center" py={8}>
        <Text fontSize="3xl">✅</Text>
        <Heading size="md" mt={2}>Relearn complete</Heading>
        <Text fontSize="sm" mt={2}>
          You cleared {clearedCount} {clearedCount === 1 ? "word" : "words"}.
        </Text>
        <Text fontSize="xs" color="fg.muted" mt={1}>
          Total answers: {totalAnswers}
          {totalAnswers > clearedCount ? " (some came around more than once)" : ""}
        </Text>
        <Text fontSize="xs" color="fg.muted" mt={3}>
          Nothing was saved to your history or schedule.
        </Text>
      </Box>

      <VStack gap={2}>
        <Button colorPalette="brand" w="full" onClick={goStart}>
          Relearn again
        </Button>
        <Button variant="outline" w="full" onClick={goHub}>
          Quiz Hub
        </Button>
      </VStack>
    </PageShell>
  );
}
