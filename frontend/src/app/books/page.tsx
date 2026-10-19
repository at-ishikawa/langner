"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Image,
  Input,
  SimpleGrid,
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
import { Card } from "@/components/Card";

// BooksPage is the Project Gutenberg discovery surface (reached from the home
// page). It BROWSES the popular catalog on load — no search required — and
// paginates it; typing a title/author switches to a free-text search. Importing
// a book stores it once as a shared notebook and jumps into the reader, where
// highlighting → saving → quizzing all happen. Already-imported shared books
// (gutenberg- ids, visible via quiz options) are listed under "Continue reading".
export default function BooksPage() {
  const router = useRouter();
  const [input, setInput] = useState("");
  const [query, setQuery] = useState(""); // the submitted term ("" = browse)
  const [page, setPage] = useState(1);

  const [books, setBooks] = useState<BookResult[]>([]);
  const [hasNext, setHasNext] = useState(false);
  const [hasPrev, setHasPrev] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [importingId, setImportingId] = useState<string | null>(null);

  const [myBooks, setMyBooks] = useState<NotebookSummary[]>([]);

  useEffect(() => {
    quizClient
      .getQuizOptions({})
      .then((res) =>
        setMyBooks(
          (res.notebooks ?? []).filter((n) => n.notebookId.startsWith("gutenberg-")),
        ),
      )
      .catch(() => {});
  }, []);

  // Load a catalog page whenever the query or page changes (mount browses the
  // popular catalog with an empty query).
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    notebookClient
      .searchBooks({ query, page })
      .then((res) => {
        if (cancelled) return;
        setBooks(res.books);
        setHasNext(res.hasNext);
        setHasPrev(res.hasPrev);
      })
      .catch((err) => {
        if (cancelled) return;
        setBooks([]);
        setHasNext(false);
        setHasPrev(false);
        setError(
          messageForRpcError(err, "Couldn't reach the book catalog. Try again."),
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [query, page]);

  const submitSearch = useCallback(() => {
    setQuery(input.trim());
    setPage(1);
  }, [input]);

  const importBook = async (book: BookResult) => {
    setImportingId(book.sourceId);
    setError(null);
    try {
      const res = await notebookClient.importBook({ sourceId: book.sourceId });
      router.push(`/learn/${res.notebookId}`);
    } catch (err) {
      setError(messageForRpcError(err, `Couldn't import “${book.title}”. Try again.`));
      setImportingId(null);
    }
  };

  return (
    <Box maxW="3xl" mx="auto" minH="100vh" p={{ base: 4, md: 6 }}>
      <Heading size="lg" mb={1}>
        Read a book
      </Heading>
      <Text fontSize="sm" color="fg.muted" mb={4}>
        Browse Project Gutenberg, or search a title or author. Import one to read
        it and mine new words.
      </Text>

      <Box display="flex" gap={2} mb={5}>
        <Input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitSearch();
          }}
          placeholder="Search title or author…"
        />
        <Button colorPalette="brand" onClick={submitSearch} flexShrink={0}>
          Search
        </Button>
        {query !== "" && (
          <Button
            variant="ghost"
            flexShrink={0}
            onClick={() => {
              setInput("");
              setQuery("");
              setPage(1);
            }}
          >
            Clear
          </Button>
        )}
      </Box>

      {error && (
        <Text color="red.500" fontSize="sm" mb={3}>
          {error}
        </Text>
      )}

      {myBooks.length > 0 && query === "" && page === 1 && (
        <Box mb={6}>
          <SectionLabel>Continue reading</SectionLabel>
          <VStack align="stretch" gap={2}>
            {myBooks.map((n) => (
              <Link key={n.notebookId} href={`/learn/${n.notebookId}`}>
                <Box
                  p={3}
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

      <SectionLabel>{query === "" ? "Popular books" : `Results for “${query}”`}</SectionLabel>

      {loading ? (
        <Box textAlign="center" py={10}>
          <Spinner size="lg" />
        </Box>
      ) : books.length === 0 && !error ? (
        <Text color="fg.muted" fontSize="sm">
          {query === "" ? "No books to show." : `No books found for “${query}”.`}
        </Text>
      ) : (
        <SimpleGrid columns={{ base: 1, sm: 2 }} gap={3}>
          {books.map((book) => (
            <BookCard
              key={book.sourceId}
              book={book}
              importing={importingId === book.sourceId}
              disabled={importingId !== null}
              onImport={() => void importBook(book)}
            />
          ))}
        </SimpleGrid>
      )}

      {!loading && (hasPrev || hasNext) && (
        <Box display="flex" justifyContent="space-between" alignItems="center" mt={5}>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasPrev}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            ← Previous
          </Button>
          <Text fontSize="sm" color="fg.muted">
            Page {page}
          </Text>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasNext}
            onClick={() => setPage((p) => p + 1)}
          >
            Next →
          </Button>
        </Box>
      )}
    </Box>
  );
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

function BookCard({
  book,
  importing,
  disabled,
  onImport,
}: {
  book: BookResult;
  importing: boolean;
  disabled: boolean;
  onImport: () => void;
}) {
  return (
    <Card
      p={3}
      borderRadius="12px"
      display="flex"
      gap={3}
    >
      <Box
        w="56px"
        h="80px"
        flexShrink={0}
        borderRadius="6px"
        overflow="hidden"
        bg="bg.subtle"
        display="flex"
        alignItems="center"
        justifyContent="center"
      >
        {book.coverUrl ? (
          <Image
            src={book.coverUrl}
            alt=""
            w="100%"
            h="100%"
            objectFit="cover"
            loading="lazy"
          />
        ) : (
          <Text fontSize="xl" color="fg.muted">
            {book.title.slice(0, 1).toUpperCase()}
          </Text>
        )}
      </Box>
      <Box display="flex" flexDirection="column" minW={0} flex="1">
        <Text fontWeight="600" fontSize="sm" lineClamp={2}>
          {book.title}
        </Text>
        <Text fontSize="xs" color="fg.muted" lineClamp={1}>
          {book.author || "Unknown author"}
          {book.language ? ` · ${book.language}` : ""}
        </Text>
        <Box mt="auto" pt={2}>
          <Button
            size="xs"
            colorPalette="brand"
            onClick={onImport}
            loading={importing}
            disabled={disabled}
          >
            Import &amp; read
          </Button>
        </Box>
      </Box>
    </Card>
  );
}
