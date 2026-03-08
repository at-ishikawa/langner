"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Box, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";

export default function NotebookListPage() {
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
      <Box p={4} maxW="md" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error) {
    return (
      <Box p={4} maxW="md" mx="auto">
        <Text color="red.500">{error}</Text>
      </Box>
    );
  }

  return (
    <Box p={4} maxW="md" mx="auto">
      <Heading size="lg" mb={4}>
        Notebooks
      </Heading>

      {notebooks.length === 0 ? (
        <Text color="fg.muted">No notebooks found.</Text>
      ) : (
        <VStack align="stretch" gap={3}>
          {notebooks.map((notebook) => (
            <Link
              key={notebook.notebookId}
              href={`/notebooks/${notebook.notebookId}`}
            >
              <Box
                p={4}
                borderWidth="1px"
                borderRadius="md"
                _hover={{ bg: "bg.muted" }}
                cursor="pointer"
              >
                <Text fontWeight="medium">{notebook.name}</Text>
              </Box>
            </Link>
          ))}
        </VStack>
      )}
    </Box>
  );
}
