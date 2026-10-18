"use client";

import { useEffect, useState } from "react";
import NextLink from "next/link";
import { Box, Button, SimpleGrid, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";
import { PageShell } from "@/components/PageShell";
import { Card } from "@/components/Card";
import { StatTile } from "@/components/StatTile";
import { StatusBadge } from "@/components/StatusBadge";

// Today: the daily entry point. Shows what's actually due (from the same quiz
// options the Quiz hub reads) and gets you into a session in one tap — replacing
// the old static link grid (that role is now the nav).
export default function TodayPage() {
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    quizClient
      .getQuizOptions({ includeUnstudied: false })
      .then((r) => setNotebooks(r.notebooks ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const wordsDue = notebooks.reduce((s, n) => s + n.reviewCount, 0);
  const grammarDue = notebooks.reduce((s, n) => s + n.grammarReviewCount, 0);
  const dueNotebooks = notebooks
    .filter((n) => n.reviewCount > 0)
    .sort((a, b) => b.reviewCount - a.reviewCount);
  const nothingDue = wordsDue === 0 && grammarDue === 0;

  if (loading) {
    return (
      <PageShell title="Today" maxW="xl">
        <Box textAlign="center" py={12}>
          <Spinner size="lg" />
        </Box>
      </PageShell>
    );
  }

  return (
    <PageShell title="Today" maxW="xl">
      <SimpleGrid columns={2} gap={3} mb={5}>
        <StatTile accent value={wordsDue} label="words due" />
        <StatTile value={grammarDue} label="grammar notes" />
      </SimpleGrid>

      <Button asChild colorPalette="brand" size="lg" w="full" mb={3}>
        <NextLink href="/quiz">
          {wordsDue > 0 ? `Review ${wordsDue} due →` : "Start a quiz →"}
        </NextLink>
      </Button>

      <Box textAlign="center" mb={7}>
        <Button asChild variant="plain" size="sm" color="fg.muted" h="auto" py={1}>
          <NextLink href="/history">History — review past days</NextLink>
        </Button>
      </Box>

      {dueNotebooks.length > 0 && (
        <>
          <Text
            fontFamily="mono"
            fontSize="xs"
            letterSpacing="0.1em"
            textTransform="uppercase"
            color="fg.muted"
            mb={2}
          >
            Due by notebook
          </Text>
          <Card overflow="hidden">
            <VStack align="stretch" gap={0}>
              {dueNotebooks.map((n, i) => (
                <Box
                  key={n.notebookId}
                  display="flex"
                  alignItems="center"
                  gap={3}
                  px={4}
                  py={3}
                  borderTopWidth={i === 0 ? "0" : "1px"}
                  borderColor="border"
                >
                  <Text fontWeight="600" fontSize="sm" flex="1" truncate>
                    {n.name}
                  </Text>
                  <StatusBadge status="due">{n.reviewCount} due</StatusBadge>
                </Box>
              ))}
            </VStack>
          </Card>
        </>
      )}

      {nothingDue && (
        <Card px={6} py={8}>
          <VStack gap={3}>
            <Text color="fg.muted" textAlign="center">
              Nothing due right now — nice work.
            </Text>
            <Button asChild variant="outline">
              <NextLink href="/learn">Browse your words</NextLink>
            </Button>
          </VStack>
        </Card>
      )}
    </PageShell>
  );
}
