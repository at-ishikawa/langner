"use client";

import { useEffect, useState } from "react";
import NextLink from "next/link";
import { Box, Button, SimpleGrid, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";
import { PageShell } from "@/components/PageShell";
import { Card } from "@/components/Card";
import { StatTile } from "@/components/StatTile";

// Today: the daily entry point. Shows what's actually due (from the same quiz
// options the Quiz hub reads) and gets you into a session in one tap. Primary
// navigation lives in the bottom tab bar, so this screen stays focused on "what
// should I do right now".
export default function TodayPage() {
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    quizClient
      .getQuizOptions({ includeUnstudied: false })
      .then((r) => setNotebooks(mergeByNotebook(r.notebooks ?? [])))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const wordsDue = notebooks.reduce((s, n) => s + n.reviewCount, 0);
  const grammarDue = notebooks.reduce((s, n) => s + n.grammarReviewCount, 0);
  const dueNotebooks = notebooks
    .filter((n) => n.reviewCount > 0)
    .sort((a, b) => b.reviewCount - a.reviewCount);
  const books = notebooks.filter((n) => n.notebookId.startsWith("gutenberg-"));
  const nothingDue = wordsDue === 0 && grammarDue === 0;

  if (loading) {
    return (
      <PageShell title="Today" maxW="xl">
        <Box textAlign="center" py={16}>
          <Spinner size="lg" color="brand.solid" />
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

      <Box textAlign="center" mb={6}>
        <Button asChild variant="plain" size="sm" color="fg.muted" h="auto" py={1}>
          <NextLink href="/history">History — review past days</NextLink>
        </Button>
      </Box>

      {dueNotebooks.length > 0 && (
        <Box mb={6}>
          <SectionLabel>Due by notebook</SectionLabel>
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
                  <Text fontWeight="500" fontSize="sm" flex="1" truncate>
                    {n.name}
                  </Text>
                  <Text
                    fontFamily="mono"
                    fontSize="sm"
                    color="brand.fg"
                    css={{ fontVariantNumeric: "tabular-nums" }}
                  >
                    {n.reviewCount}
                  </Text>
                </Box>
              ))}
            </VStack>
          </Card>
        </Box>
      )}

      {books.length > 0 && (
        <Box mb={6}>
          <SectionLabel>Continue reading</SectionLabel>
          <VStack align="stretch" gap={2}>
            {books.map((n) => (
              <Button
                key={n.notebookId}
                asChild
                variant="outline"
                justifyContent="space-between"
                h="auto"
                py={3}
                px={4}
              >
                <NextLink href={`/learn/${n.notebookId}`}>
                  <Text fontWeight="500" fontSize="sm" truncate>
                    {n.name}
                  </Text>
                  <Text aria-hidden color="fg.muted">
                    ›
                  </Text>
                </NextLink>
              </Button>
            ))}
          </VStack>
        </Box>
      )}

      {nothingDue && (
        <Card px={6} py={10} mb={6}>
          <VStack gap={4}>
            <Text
              color="fg.muted"
              textAlign="center"
              fontFamily="heading"
              fontSize="lg"
              fontStyle="italic"
            >
              Nothing due right now — nice work.
            </Text>
            <Box display="flex" gap={3} flexWrap="wrap" justifyContent="center">
              <Button asChild variant="outline">
                <NextLink href="/books">Find a book</NextLink>
              </Button>
              <Button asChild variant="outline">
                <NextLink href="/learn">Browse your words</NextLink>
              </Button>
            </Box>
          </VStack>
        </Card>
      )}
    </PageShell>
  );
}

// getQuizOptions returns the SAME notebookId as more than one row on purpose —
// a journal surfaces once as its vocabulary/Journal row and again as a separate
// "Grammar" row so the Quiz hub can split them across tabs (it collapses them
// with a kind filter). This dashboard doesn't split by kind: it wants one row
// and one count per notebook, so it folds the rows together, taking the MAX of
// each count field (never the sum — a count duplicated across rows must stay
// itself, while a vocab row's reviewCount and a grammar row's grammarReviewCount
// each survive). Without this, keys collide and "words due" double-counts.
function mergeByNotebook(list: NotebookSummary[]): NotebookSummary[] {
  const byId = new Map<string, NotebookSummary>();
  for (const n of list) {
    const prev = byId.get(n.notebookId);
    if (!prev) {
      byId.set(n.notebookId, n);
      continue;
    }
    byId.set(n.notebookId, {
      ...prev,
      name: prev.name || n.name,
      kind: prev.kind || n.kind,
      hasContent: prev.hasContent || n.hasContent,
      reviewCount: Math.max(prev.reviewCount, n.reviewCount),
      reverseReviewCount: Math.max(prev.reverseReviewCount, n.reverseReviewCount),
      etymologyReviewCount: Math.max(prev.etymologyReviewCount, n.etymologyReviewCount),
      etymologyReverseReviewCount: Math.max(
        prev.etymologyReverseReviewCount,
        n.etymologyReverseReviewCount,
      ),
      grammarReviewCount: Math.max(prev.grammarReviewCount, n.grammarReviewCount),
      vocabularyCount: Math.max(prev.vocabularyCount, n.vocabularyCount),
    });
  }
  return Array.from(byId.values());
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <Text
      fontFamily="mono"
      fontSize="xs"
      letterSpacing="0.12em"
      textTransform="uppercase"
      color="fg.muted"
      mb={2}
    >
      {children}
    </Text>
  );
}
