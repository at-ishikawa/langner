"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Box, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";

type Tab = "vocabulary" | "journals" | "etymology";

const TAB_LABELS: { key: Tab; label: string }[] = [
  { key: "vocabulary", label: "Vocabulary" },
  { key: "journals", label: "Journals" },
  { key: "etymology", label: "Word roots" },
];

export default function LearnHubPage() {
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("vocabulary");

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) => {
        setNotebooks(res.notebooks ?? []);
      })
      .catch(() => setError("Failed to load notebooks"))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <Box p={4} maxW="sm" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error) {
    return (
      <Box p={4} maxW="sm" mx="auto">
        <Text color="red.500">{error}</Text>
      </Box>
    );
  }

  // Vocabulary keeps only true vocabulary notebooks — journals (kind
  // "Journal") and their grammar-quiz twin (kind "Grammar") get their own tab.
  const vocabularyNotebooks = notebooks.filter(
    (n) => n.kind !== "Etymology" && n.kind !== "Journal" && n.kind !== "Grammar",
  );
  const etymologyNotebooks = notebooks.filter((n) => n.kind === "Etymology");

  // A journal surfaces twice in the options (once as "Journal", once as
  // "Grammar" with the same notebookId); list each journal once, keyed by id.
  const journalNotebooks = Array.from(
    notebooks
      .filter((n) => n.kind === "Journal")
      .reduce((map, n) => map.set(n.notebookId, n), new Map<string, NotebookSummary>())
      .values(),
  );

  const totalVocabWords = vocabularyNotebooks.reduce(
    (sum, n) => sum + n.reviewCount,
    0,
  );
  const totalEtymologyOrigins = etymologyNotebooks.reduce(
    (sum, n) => sum + n.reviewCount,
    0,
  );

  return (
    <Box maxW="sm" mx="auto" bg="bg" minH="100vh">
      {/* Header — left-aligned editorial title, consistent with every other
          content page (Today / Analytics / Books / History / Settings). */}
      <Box px={4} pt={6} pb={3}>
        <Heading size="lg">Learn</Heading>
      </Box>

      {/* Tabs */}
      <Box
        bg="panel"
        borderBottomWidth="1px"
        borderColor="border"
        display="flex"
      >
        {TAB_LABELS.map(({ key, label }) => (
          <Box
            key={key}
            flex={1}
            textAlign="center"
            py={2}
            cursor="pointer"
            onClick={() => setTab(key)}
            position="relative"
          >
            <Text
              fontSize="sm"
              fontWeight={tab === key ? "semibold" : "normal"}
              color={tab === key ? "brand.fg" : "fg.muted"}
            >
              {label}
            </Text>
            {tab === key && (
              <Box
                position="absolute"
                bottom={0}
                left="50%"
                transform="translateX(-50%)"
                w="60%"
                h="3px"
                borderRadius="full"
                bg="brand.solid"
              />
            )}
          </Box>
        ))}
      </Box>

      {/* Content */}
      <Box p={4}>
        {tab === "vocabulary" ? (
          vocabularyNotebooks.length === 0 ? (
            <Text color="fg.muted" textAlign="center">
              No notebooks found.
            </Text>
          ) : (
            <VStack align="stretch" gap={2}>
              {vocabularyNotebooks.map((notebook) => (
                <Link
                  key={notebook.notebookId}
                  href={
                    notebook.hasContent
                      ? `/learn/${notebook.notebookId}`
                      : `/notebooks/${notebook.notebookId}`
                  }
                >
                  <Box
                    p={4}
                    bg="panel"
                    borderWidth="1px"
                    borderColor="border"
                    borderRadius="12px"
                    _hover={{ bg: "bg.subtle" }}
                    cursor="pointer"
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                  >
                    <Text fontWeight="medium" fontSize="sm">
                      {notebook.name}
                    </Text>
                    <Box display="flex" alignItems="center" gap={2}>
                      <Text fontSize="xs" color="fg.muted">
                        <Text
                          as="span"
                          fontFamily="mono"
                          css={{ fontVariantNumeric: "tabular-nums" }}
                        >
                          {notebook.reviewCount}
                        </Text>{" "}
                        words
                      </Text>
                      <Text fontSize="sm" color="fg.muted">
                        &rsaquo;
                      </Text>
                    </Box>
                  </Box>
                </Link>
              ))}
            </VStack>
          )
        ) : tab === "journals" ? (
          journalNotebooks.length === 0 ? (
            <Text color="fg.muted" textAlign="center">
              No journals found.
            </Text>
          ) : (
            <VStack align="stretch" gap={2}>
              {journalNotebooks.map((notebook) => (
                <Link
                  key={notebook.notebookId}
                  href={`/journals/${notebook.notebookId}`}
                >
                  <Box
                    p={4}
                    bg="panel"
                    borderWidth="1px"
                    borderColor="border"
                    borderRadius="12px"
                    _hover={{ bg: "bg.subtle" }}
                    cursor="pointer"
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                  >
                    <Text fontWeight="medium" fontSize="sm">
                      {notebook.name}
                    </Text>
                    <Text fontSize="sm" color="fg.muted">
                      &rsaquo;
                    </Text>
                  </Box>
                </Link>
              ))}
            </VStack>
          )
        ) : etymologyNotebooks.length === 0 ? (
          <Text color="fg.muted" textAlign="center">
            No etymology notebooks found.
          </Text>
        ) : (
          <VStack align="stretch" gap={2}>
            {etymologyNotebooks.map((notebook) => (
              <Link
                key={notebook.notebookId}
                href={`/notebooks/etymology/${notebook.notebookId}`}
              >
                <Box
                  p={4}
                  bg="panel"
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="12px"
                  _hover={{ bg: "bg.subtle" }}
                  cursor="pointer"
                  display="flex"
                  alignItems="center"
                  justifyContent="space-between"
                >
                  <Box>
                    <Text fontWeight="medium" fontSize="sm">
                      {notebook.name}
                    </Text>
                    <Text fontSize="xs" color="fg.muted">
                      <Text
                        as="span"
                        fontFamily="mono"
                        css={{ fontVariantNumeric: "tabular-nums" }}
                      >
                        {notebook.reviewCount}
                      </Text>{" "}
                      origins
                    </Text>
                  </Box>
                  <Text fontSize="sm" color="fg.muted">
                    &rsaquo;
                  </Text>
                </Box>
              </Link>
            ))}
          </VStack>
        )}
      </Box>

      {/* Summary footer */}
      <Box
        bg="panel"
        borderTopWidth="1px"
        borderColor="border"
        py={3}
        textAlign="center"
        position="fixed"
        bottom={0}
        left={0}
        right={0}
        maxW="sm"
        mx="auto"
      >
        <Text fontSize="sm" color="fg.muted">
          {tab === "vocabulary"
            ? `${vocabularyNotebooks.length} notebooks \u00B7 ${totalVocabWords} words`
            : tab === "journals"
              ? `${journalNotebooks.length} journals`
              : `${etymologyNotebooks.length} notebooks \u00B7 ${totalEtymologyOrigins} origins`}
        </Text>
      </Box>
    </Box>
  );
}
