"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Box, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";

type Tab = "vocabulary" | "etymology";

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

  const vocabularyNotebooks = notebooks.filter((n) => n.kind !== "Etymology");
  const etymologyNotebooks = notebooks.filter((n) => n.kind === "Etymology");

  const totalVocabWords = vocabularyNotebooks.reduce(
    (sum, n) => sum + n.reviewCount,
    0,
  );
  const totalEtymologyOrigins = etymologyNotebooks.reduce(
    (sum, n) => sum + n.reviewCount,
    0,
  );

  return (
    <Box maxW="sm" mx="auto" bg="#f8f9fa" minH="100vh">
      {/* Header */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb">
        <Box px={4} pt={2}>
          <Link href="/">
            <Text color="#2563eb" fontSize="xs">
              &lt; Home
            </Text>
          </Link>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">Learn</Heading>
        </Box>
      </Box>

      {/* Tabs */}
      <Box
        bg="white"
        borderBottomWidth="1px"
        borderColor="#e5e7eb"
        display="flex"
      >
        <Box
          flex={1}
          textAlign="center"
          py={2}
          cursor="pointer"
          onClick={() => setTab("vocabulary")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "vocabulary" ? "semibold" : "normal"}
            color={tab === "vocabulary" ? "#2563eb" : "#999"}
          >
            Vocabulary
          </Text>
          {tab === "vocabulary" && (
            <Box
              position="absolute"
              bottom={0}
              left="50%"
              transform="translateX(-50%)"
              w="60%"
              h="3px"
              borderRadius="full"
              bg="#2563eb"
            />
          )}
        </Box>
        <Box
          flex={1}
          textAlign="center"
          py={2}
          cursor="pointer"
          onClick={() => setTab("etymology")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "etymology" ? "semibold" : "normal"}
            color={tab === "etymology" ? "#2563eb" : "#999"}
          >
            Etymology
          </Text>
          {tab === "etymology" && (
            <Box
              position="absolute"
              bottom={0}
              left="50%"
              transform="translateX(-50%)"
              w="60%"
              h="3px"
              borderRadius="full"
              bg="#2563eb"
            />
          )}
        </Box>
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
                  href={`/notebooks/${notebook.notebookId}`}
                >
                  <Box
                    p={4}
                    bg="white"
                    borderWidth="1px"
                    borderColor="#e5e7eb"
                    borderRadius="10px"
                    _hover={{ bg: "gray.50" }}
                    cursor="pointer"
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                  >
                    <Text fontWeight="medium" fontSize="sm">
                      {notebook.name}
                    </Text>
                    <Box display="flex" alignItems="center" gap={2}>
                      <Text fontSize="xs" color="#666">
                        {notebook.reviewCount} words
                      </Text>
                      <Text fontSize="sm" color="#999">
                        &rsaquo;
                      </Text>
                    </Box>
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
                  bg="white"
                  borderWidth="1px"
                  borderColor="#e5e7eb"
                  borderRadius="10px"
                  _hover={{ bg: "gray.50" }}
                  cursor="pointer"
                  display="flex"
                  alignItems="center"
                  justifyContent="space-between"
                >
                  <Box>
                    <Text fontWeight="medium" fontSize="sm">
                      {notebook.name}
                    </Text>
                    <Text fontSize="xs" color="#999">
                      {notebook.reviewCount} origins
                    </Text>
                  </Box>
                  <Text fontSize="sm" color="#999">
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
        bg="white"
        borderTopWidth="1px"
        borderColor="#e5e7eb"
        py={3}
        textAlign="center"
        position="fixed"
        bottom={0}
        left={0}
        right={0}
        maxW="sm"
        mx="auto"
      >
        <Text fontSize="sm" color="#666">
          {tab === "vocabulary"
            ? `${vocabularyNotebooks.length} notebooks \u00B7 ${totalVocabWords} words`
            : `${etymologyNotebooks.length} notebooks \u00B7 ${totalEtymologyOrigins} origins`}
        </Text>
      </Box>
    </Box>
  );
}
