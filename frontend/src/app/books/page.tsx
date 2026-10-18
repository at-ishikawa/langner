"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Input,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  notebookClient,
  quizClient,
  messageForRpcError,
  type BookResult,
  type NotebookSummary,
} from "@/lib/client";

// BooksPage lets a reader search the Project Gutenberg catalog (Gutendex),
// import a public-domain book once as a shared notebook, and jump straight into
// the reader. Books are shared across all users, so an already-imported book is
// listed under "Continue reading" (they surface as visible gutenberg- notebooks
// in the quiz options). Highlight → register → quiz all happen in the reader.
export default function BooksPage() {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<BookResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [importingId, setImportingId] = useState<string | null>(null);

  const [myBooks, setMyBooks] = useState<NotebookSummary[]>([]);

  // Already-imported books are shared, owner-less notebooks with a
  // `gutenberg-<id>` id; they show up in the visible quiz options.
  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) =>
        setMyBooks(
          (res.notebooks ?? []).filter((n) =>
            n.notebookId.startsWith("gutenberg-"),
          ),
        ),
      )
      .catch(() => {});
  }, []);

  const runSearch = async () => {
    const q = query.trim();
    if (!q) return;
    setSearching(true);
    setError(null);
    try {
      const res = await notebookClient.searchBooks({ query: q });
      setResults(res.books);
      setSearched(true);
    } catch (err) {
      setError(messageForRpcError(err, "Search failed. Please try again."));
    } finally {
      setSearching(false);
    }
  };

  const importBook = async (book: BookResult) => {
    setImportingId(book.sourceId);
    setError(null);
    try {
      const res = await notebookClient.importBook({ sourceId: book.sourceId });
      router.push(`/learn/${res.notebookId}`);
    } catch (err) {
      setError(
        messageForRpcError(err, `Couldn't import "${book.title}". Try again.`),
      );
      setImportingId(null);
    }
  };

  return (
    <Box maxW="sm" mx="auto" minH="100vh" p={4}>
      <Box mb={2}>
        <Link href="/">
          <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="xs">
            &lt; Home
          </Text>
        </Link>
      </Box>
      <Heading size="md" mb={1} textAlign="center">
        Read a book
      </Heading>
      <Text fontSize="sm" color="fg.muted" textAlign="center" mb={4}>
        Search Project Gutenberg, then read and mine new words.
      </Text>

      <Box display="flex" gap={2} mb={4}>
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void runSearch();
          }}
          placeholder="Title or author…"
          size="sm"
        />
        <Button
          size="sm"
          colorPalette="blue"
          onClick={() => void runSearch()}
          loading={searching}
          flexShrink={0}
        >
          Search
        </Button>
      </Box>

      {error && (
        <Text color="red.500" fontSize="sm" mb={3}>
          {error}
        </Text>
      )}

      {myBooks.length > 0 && !searched && (
        <Box mb={6}>
          <Text fontSize="xs" fontWeight="bold" color="fg.muted" mb={2}>
            Continue reading
          </Text>
          <VStack align="stretch" gap={2}>
            {myBooks.map((n) => (
              <Link key={n.notebookId} href={`/learn/${n.notebookId}`}>
                <Box
                  p={4}
                  borderWidth="1px"
                  borderColor="border"
                  borderRadius="10px"
                  _hover={{ bg: "bg.subtle" }}
                  cursor="pointer"
                  display="flex"
                  alignItems="center"
                  justifyContent="space-between"
                >
                  <Text fontWeight="medium" fontSize="sm">
                    {n.name}
                  </Text>
                  <Text fontSize="sm" color="fg.muted">
                    &rsaquo;
                  </Text>
                </Box>
              </Link>
            ))}
          </VStack>
        </Box>
      )}

      {searching && (
        <Box textAlign="center" py={6}>
          <Spinner />
        </Box>
      )}

      {searched && !searching && results.length === 0 && !error && (
        <Text color="fg.muted" fontSize="sm" textAlign="center">
          No books found for “{query}”.
        </Text>
      )}

      <VStack align="stretch" gap={2}>
        {results.map((book) => (
          <Box
            key={book.sourceId}
            p={4}
            borderWidth="1px"
            borderColor="border"
            borderRadius="10px"
            display="flex"
            alignItems="center"
            justifyContent="space-between"
            gap={3}
          >
            <Box minW={0}>
              <Text fontWeight="medium" fontSize="sm" truncate>
                {book.title}
              </Text>
              <Text fontSize="xs" color="fg.muted" truncate>
                {book.author || "Unknown author"}
                {book.language ? ` · ${book.language}` : ""}
              </Text>
            </Box>
            <Button
              size="xs"
              colorPalette="blue"
              flexShrink={0}
              onClick={() => void importBook(book)}
              loading={importingId === book.sourceId}
              disabled={importingId !== null}
            >
              Import &amp; read
            </Button>
          </Box>
        ))}
      </VStack>
    </Box>
  );
}
