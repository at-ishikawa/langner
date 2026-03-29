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
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh">
      {/* Header */}
      <Box bg="white" _dark={{ bg: "gray.800", borderColor: "gray.600" }} borderBottomWidth="1px" borderColor="gray.200">
        <Box px={4} pt={2}>
          <Link href="/">
            <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="xs">
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
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderBottomWidth="1px"
        borderColor="gray.200"
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
            color={tab === "vocabulary" ? "blue.600" : "gray.500"}
            _dark={{ color: tab === "vocabulary" ? "blue.300" : "gray.400" }}
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
              bg="blue.600"
              _dark={{ bg: "blue.300" }}
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
            color={tab === "etymology" ? "blue.600" : "gray.500"}
            _dark={{ color: tab === "etymology" ? "blue.300" : "gray.400" }}
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
              bg="blue.600"
              _dark={{ bg: "blue.300" }}
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
                    _dark={{ bg: "gray.800", borderColor: "gray.600" }}
                    borderWidth="1px"
                    borderColor="gray.200"
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
                      <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }}>
                        {notebook.reviewCount} words
                      </Text>
                      <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
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
                  _dark={{ bg: "gray.800", borderColor: "gray.600" }}
                  borderWidth="1px"
                  borderColor="gray.200"
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
                    <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
                      {notebook.reviewCount} origins
                    </Text>
                  </Box>
                  <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
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
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderTopWidth="1px"
        borderColor="gray.200"
        py={3}
        textAlign="center"
        position="fixed"
        bottom={0}
        left={0}
        right={0}
        maxW="sm"
        mx="auto"
      >
        <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>
          {tab === "vocabulary"
            ? `${vocabularyNotebooks.length} notebooks \u00B7 ${totalVocabWords} words`
            : `${etymologyNotebooks.length} notebooks \u00B7 ${totalEtymologyOrigins} origins`}
        </Text>
      </Box>
    </Box>
  );
}
