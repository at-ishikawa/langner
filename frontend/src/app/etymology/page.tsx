"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Box, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";

export default function EtymologyListPage() {
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) => {
        setNotebooks(
          (res.notebooks ?? []).filter((n) => n.kind === "Etymology"),
        );
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

  return (
    <Box p={4} maxW="sm" mx="auto">
      <Box mb={2}>
        <Link href="/">
          <Text color="blue.600" fontSize="sm" _dark={{ color: "blue.300" }}>
            &larr; Back
          </Text>
        </Link>
      </Box>
      <Heading size="lg" mb={4}>
        Etymology
      </Heading>

      {notebooks.length === 0 ? (
        <Text color="fg.muted">No etymology notebooks found.</Text>
      ) : (
        <VStack align="stretch" gap={3}>
          {notebooks.map((notebook) => (
            <Link
              key={notebook.notebookId}
              href={`/notebooks/etymology/${notebook.notebookId}`}
            >
              <Box
                p={4}
                borderWidth="1px"
                borderRadius="md"
                _hover={{ bg: "bg.muted" }}
                cursor="pointer"
              >
                <Text fontWeight="medium">{notebook.name}</Text>
                <Text fontSize="sm" color="fg.muted">
                  {notebook.etymologyReviewCount} origins
                </Text>
              </Box>
            </Link>
          ))}
        </VStack>
      )}
    </Box>
  );
}
