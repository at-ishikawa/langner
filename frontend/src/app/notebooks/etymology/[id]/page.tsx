"use client";

import { useEffect, useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  Box,
  Heading,
  Input,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import {
  notebookClient,
  type EtymologyOriginPart,
  type EtymologyDefinition,
  type EtymologyMeaningGroup,
} from "@/lib/client";

type Tab = "origins" | "meanings";

const typeBadgeColors: Record<string, { bg: string; color: string }> = {
  root: { bg: "#dbeafe", color: "#1e40af" },
  prefix: { bg: "#fef3c7", color: "#92400e" },
  suffix: { bg: "#dcfce7", color: "#166534" },
};

function TypeBadge({ type }: { type: string }) {
  const colors = typeBadgeColors[type.toLowerCase()] ?? { bg: "#f3f4f6", color: "#374151" };
  return (
    <Box
      as="span"
      display="inline-block"
      px={2}
      py={0.5}
      borderRadius="sm"
      fontSize="xs"
      fontWeight="medium"
      bg={colors.bg}
      color={colors.color}
    >
      {type}
    </Box>
  );
}

function LanguageBadge({ language }: { language: string }) {
  if (!language) return null;
  return (
    <Box
      as="span"
      display="inline-block"
      px={2}
      py={0.5}
      borderRadius="sm"
      fontSize="xs"
      bg="#f3f4f6"
      color="#374151"
    >
      {language}
    </Box>
  );
}

function OriginCard({
  origin,
  notebookId,
}: {
  origin: EtymologyOriginPart;
  notebookId: string;
}) {
  return (
    <Box
      p={3}
      borderWidth="1px"
      borderRadius="md"
      _hover={{ bg: "bg.muted" }}
    >
      <Box display="flex" alignItems="center" gap={2} mb={1} flexWrap="wrap">
        <Link
          href={`/notebooks/etymology/${notebookId}?origin=${encodeURIComponent(origin.origin)}`}
        >
          <Text
            fontWeight="semibold"
            color="#2563eb"
            cursor="pointer"
            _hover={{ textDecoration: "underline" }}
          >
            {origin.origin}
          </Text>
        </Link>
        {origin.type && <TypeBadge type={origin.type} />}
        <LanguageBadge language={origin.language} />
      </Box>
      <Text fontSize="sm" color="fg.muted">
        {origin.meaning}
      </Text>
      <Text fontSize="xs" color="fg.subtle" mt={1}>
        {origin.wordCount} {origin.wordCount === 1 ? "word" : "words"}
      </Text>
    </Box>
  );
}

export default function EtymologyNotebookPage() {
  const params = useParams();
  const id = params.id as string;

  const [origins, setOrigins] = useState<EtymologyOriginPart[]>([]);
  const [definitions, setDefinitions] = useState<EtymologyDefinition[]>([]);
  const [meaningGroups, setMeaningGroups] = useState<EtymologyMeaningGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [tab, setTab] = useState<Tab>("origins");
  const [selectedOrigin, setSelectedOrigin] = useState<string | null>(null);

  useEffect(() => {
    notebookClient
      .getEtymologyNotebook({ notebookId: id })
      .then((res) => {
        setOrigins(res.origins ?? []);
        setDefinitions(res.definitions ?? []);
        setMeaningGroups(res.meaningGroups ?? []);

        // Check URL for origin query param
        const url = new URL(window.location.href);
        const originParam = url.searchParams.get("origin");
        if (originParam) {
          setSelectedOrigin(originParam);
        }
      })
      .catch(() => setError("Failed to load etymology notebook"))
      .finally(() => setLoading(false));
  }, [id]);

  const filteredOrigins = useMemo(() => {
    if (!search.trim()) return origins;
    const lower = search.toLowerCase();
    return origins.filter(
      (o) =>
        o.origin.toLowerCase().includes(lower) ||
        o.meaning.toLowerCase().includes(lower),
    );
  }, [origins, search]);

  const filteredMeaningGroups = useMemo(() => {
    if (!search.trim()) return meaningGroups;
    const lower = search.toLowerCase();
    return meaningGroups.filter(
      (g) =>
        g.meaning.toLowerCase().includes(lower) ||
        g.origins.some((o) => o.origin.toLowerCase().includes(lower)),
    );
  }, [meaningGroups, search]);

  if (loading) {
    return (
      <Box p={4} maxW="2xl" mx="auto" textAlign="center">
        <Spinner size="lg" />
      </Box>
    );
  }

  if (error) {
    return (
      <Box p={4} maxW="2xl" mx="auto">
        <Text color="red.500">{error}</Text>
      </Box>
    );
  }

  // Origin detail view
  if (selectedOrigin) {
    const originData = origins.find((o) => o.origin === selectedOrigin);
    const relatedDefs = definitions.filter((d) =>
      d.originParts.some((p) => p.origin === selectedOrigin),
    );

    return (
      <Box p={4} maxW="2xl" mx="auto">
        <Box mb={2}>
          <Text
            color="blue.600"
            fontSize="sm"
            cursor="pointer"
            onClick={() => setSelectedOrigin(null)}
          >
            &larr; Back to origins
          </Text>
        </Box>

        {originData && (
          <Box
            p={4}
            borderWidth="1px"
            borderRadius="md"
            mb={4}
            bg="blue.50"
            _dark={{ bg: "blue.900/20" }}
          >
            <Box display="flex" alignItems="center" gap={2} mb={2} flexWrap="wrap">
              <Heading size="md">{originData.origin}</Heading>
              {originData.type && <TypeBadge type={originData.type} />}
              <LanguageBadge language={originData.language} />
            </Box>
            <Text>{originData.meaning}</Text>
          </Box>
        )}

        <Heading size="sm" mb={3}>
          Words ({relatedDefs.length})
        </Heading>
        <VStack align="stretch" gap={2}>
          {relatedDefs.map((def, i) => (
            <Box key={i} p={3} borderWidth="1px" borderRadius="md">
              <Text fontWeight="semibold">{def.expression}</Text>
              <Text fontSize="sm" color="fg.muted" mb={2}>
                {def.meaning}
              </Text>
              <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
                {def.originParts.map((part, j) => (
                  <Box key={j} display="flex" alignItems="center" gap={1}>
                    {j > 0 && (
                      <Text fontSize="sm" color="fg.muted">
                        +
                      </Text>
                    )}
                    {part.origin === selectedOrigin ? (
                      <Box
                        px={2}
                        py={0.5}
                        borderRadius="sm"
                        borderWidth="2px"
                        borderColor="#2563eb"
                        bg="blue.50"
                        _dark={{ bg: "blue.900/20" }}
                      >
                        <Text fontSize="sm" color="#2563eb" fontWeight="medium">
                          {part.origin} (current)
                        </Text>
                      </Box>
                    ) : (
                      <Text
                        fontSize="sm"
                        color="#2563eb"
                        cursor="pointer"
                        _hover={{ textDecoration: "underline" }}
                        onClick={() => setSelectedOrigin(part.origin)}
                      >
                        {part.origin}
                      </Text>
                    )}
                    <LanguageBadge language={part.language} />
                  </Box>
                ))}
              </Box>
            </Box>
          ))}
        </VStack>
      </Box>
    );
  }

  // Origin list view
  return (
    <Box p={4} maxW="2xl" mx="auto">
      <Box mb={2}>
        <Link href="/notebooks">
          <Text color="blue.600" fontSize="sm">
            &larr; Back to notebooks
          </Text>
        </Link>
      </Box>

      <Heading size="lg" mb={4}>
        Etymology Origins
      </Heading>

      <Input
        placeholder="Search origins or meanings..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        mb={4}
      />

      <Box display="flex" gap={2} mb={4}>
        <Box
          px={3}
          py={1}
          borderRadius="md"
          cursor="pointer"
          fontWeight="medium"
          fontSize="sm"
          bg={tab === "origins" ? "blue.500" : "gray.100"}
          color={tab === "origins" ? "white" : "fg.default"}
          _dark={{
            bg: tab === "origins" ? "blue.500" : "gray.700",
            color: tab === "origins" ? "white" : "fg.default",
          }}
          onClick={() => setTab("origins")}
        >
          All Origins
        </Box>
        <Box
          px={3}
          py={1}
          borderRadius="md"
          cursor="pointer"
          fontWeight="medium"
          fontSize="sm"
          bg={tab === "meanings" ? "blue.500" : "gray.100"}
          color={tab === "meanings" ? "white" : "fg.default"}
          _dark={{
            bg: tab === "meanings" ? "blue.500" : "gray.700",
            color: tab === "meanings" ? "white" : "fg.default",
          }}
          onClick={() => setTab("meanings")}
        >
          By Meaning
        </Box>
      </Box>

      {tab === "origins" ? (
        <VStack align="stretch" gap={2}>
          {filteredOrigins.map((origin, i) => (
            <OriginCard key={i} origin={origin} notebookId={id} />
          ))}
          {filteredOrigins.length === 0 && (
            <Text color="fg.muted" textAlign="center">
              No origins match your search.
            </Text>
          )}
        </VStack>
      ) : (
        <VStack align="stretch" gap={3}>
          {filteredMeaningGroups.map((group, i) => (
            <Box key={i} p={3} borderWidth="1px" borderRadius="md">
              <Text fontWeight="semibold" mb={2}>
                {group.meaning}
              </Text>
              <VStack align="stretch" gap={1}>
                {group.origins.map((origin, j) => (
                  <Box
                    key={j}
                    display="flex"
                    alignItems="center"
                    gap={2}
                    pl={2}
                  >
                    <Text
                      color="#2563eb"
                      cursor="pointer"
                      _hover={{ textDecoration: "underline" }}
                      onClick={() => setSelectedOrigin(origin.origin)}
                    >
                      {origin.origin}
                    </Text>
                    <LanguageBadge language={origin.language} />
                    <Text fontSize="xs" color="fg.subtle">
                      {origin.wordCount} {origin.wordCount === 1 ? "word" : "words"}
                    </Text>
                  </Box>
                ))}
              </VStack>
            </Box>
          ))}
          {filteredMeaningGroups.length === 0 && (
            <Text color="fg.muted" textAlign="center">
              No meaning groups match your search.
            </Text>
          )}
        </VStack>
      )}
    </Box>
  );
}
