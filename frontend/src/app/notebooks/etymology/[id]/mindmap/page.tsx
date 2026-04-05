"use client";

import { Suspense, useEffect, useState, useCallback, lazy } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { Box, Spinner, Text } from "@chakra-ui/react";
import {
  notebookClient,
  type EtymologyOriginPart,
  type EtymologyDefinition,
} from "@/lib/client";

const EtymologyMindmap = lazy(() => import("@/components/EtymologyMindmap"));

export default function MindmapPageWrapper() {
  return (
    <Suspense
      fallback={
        <Box
          w="100vw"
          h="100vh"
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          <Spinner size="lg" />
        </Box>
      }
    >
      <MindmapPage />
    </Suspense>
  );
}

function MindmapPage() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const id = params.id as string;
  const origin = searchParams.get("origin");

  const [origins, setOrigins] = useState<EtymologyOriginPart[]>([]);
  const [definitions, setDefinitions] = useState<EtymologyDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    notebookClient
      .getEtymologyNotebook({ notebookId: id })
      .then((res) => {
        setOrigins(res.origins ?? []);
        setDefinitions(res.definitions ?? []);
      })
      .catch(() => setError("Failed to load etymology notebook"))
      .finally(() => setLoading(false));
  }, [id]);

  const selectOrigin = useCallback(
    (originName: string) => {
      router.push(
        `/notebooks/etymology/${id}/mindmap?origin=${encodeURIComponent(originName)}`,
      );
    },
    [router, id],
  );

  if (loading) {
    return (
      <Box
        w="100vw"
        h="100vh"
        display="flex"
        alignItems="center"
        justifyContent="center"
      >
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error || !origin) {
    return (
      <Box p={4} textAlign="center">
        <Text color="red.500">{error ?? "No origin specified"}</Text>
        <Link href={`/notebooks/etymology/${id}`}>
          <Text color="blue.600" _dark={{ color: "blue.300" }} mt={2}>
            Back to etymology notebook
          </Text>
        </Link>
      </Box>
    );
  }

  return (
    <Box w="100vw" h="calc(100dvh - 41px)" display="flex" flexDirection="column" overflow="hidden">
      {/* Header */}
      <Box
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderBottomWidth="1px"
        borderColor="gray.200"
        px={4}
        py={2}
        display="flex"
        alignItems="center"
        justifyContent="space-between"
        flexShrink={0}
      >
        <Link
          href={`/notebooks/etymology/${id}?origin=${encodeURIComponent(origin)}`}
        >
          <Text
            fontSize="xs"
            color="blue.600"
            _dark={{ color: "blue.300" }}
            _hover={{ textDecoration: "underline" }}
          >
            &lt; Origin Detail
          </Text>
        </Link>
        <Text fontSize="sm" fontWeight="semibold">
          {origin}
        </Text>
        <Box w="70px" />
      </Box>

      {/* Mindmap fills remaining space */}
      <Box flex={1} position="relative">
        <Suspense
          fallback={
            <Box
              w="100%"
              h="100%"
              display="flex"
              alignItems="center"
              justifyContent="center"
            >
              <Spinner size="lg" />
            </Box>
          }
        >
          <EtymologyMindmap
            focusedOrigin={origin}
            origins={origins}
            definitions={definitions}
            onSelectOrigin={selectOrigin}
          />
        </Suspense>
      </Box>
    </Box>
  );
}
