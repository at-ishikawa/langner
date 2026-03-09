"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Box, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import { quizClient, type NotebookSummary } from "@/lib/client";

export default function BooksListPage() {
  const [books, setBooks] = useState<NotebookSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) => {
        setBooks((res.notebooks ?? []).filter((n) => n.kind === "Books"));
      })
      .catch(() => setError("Failed to load books"))
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
      <Box mb={2}>
        <Link href="/notebooks">
          <Text color="blue.600" fontSize="sm">
            &larr; Back to notebooks
          </Text>
        </Link>
      </Box>

      <Heading size="lg" mb={4}>
        Books
      </Heading>

      {books.length === 0 ? (
        <Text color="fg.muted">No books found.</Text>
      ) : (
        <VStack align="stretch" gap={3}>
          {books.map((book) => (
            <Link key={book.notebookId} href={`/books/${book.notebookId}`}>
              <Box
                p={4}
                borderWidth="1px"
                borderRadius="md"
                _hover={{ bg: "bg.muted" }}
                cursor="pointer"
              >
                <Text fontWeight="medium">{book.name}</Text>
              </Box>
            </Link>
          ))}
        </VStack>
      )}
    </Box>
  );
}
