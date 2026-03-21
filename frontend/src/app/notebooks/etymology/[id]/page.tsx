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
  const colors = typeBadgeColors[type.toLowerCase()] ?? {
    bg: "#f3f4f6",
    color: "#374151",
  };
  return (
    <Box
      as="span"
      display="inline-block"
      px={2}
      py={0.5}
      borderRadius="full"
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
      borderRadius="full"
      fontSize="xs"
      bg="#f3f4f6"
      color="#666"
    >
      {language}
    </Box>
  );
}

function OriginCard({
  origin,
  onClick,
}: {
  origin: EtymologyOriginPart;
  onClick: () => void;
}) {
  return (
    <Box
      p={3}
      borderWidth="1px"
      borderRadius="lg"
      bg="white"
      _hover={{ bg: "gray.50" }}
      cursor="pointer"
      onClick={onClick}
    >
      <Box
        display="flex"
        alignItems="center"
        justifyContent="space-between"
        mb={1}
      >
        <Box display="flex" alignItems="center" gap={2} flexWrap="wrap">
          <Text
            fontSize="md"
            fontWeight="semibold"
            color="#2563eb"
          >
            {origin.origin}
          </Text>
          {origin.type && <TypeBadge type={origin.type} />}
          <LanguageBadge language={origin.language} />
        </Box>
        <Text fontSize="xs" color="#666" flexShrink={0}>
          {origin.wordCount} {origin.wordCount === 1 ? "word" : "words"}
        </Text>
      </Box>
      <Text fontSize="sm" color="#555">
        {origin.meaning}
      </Text>
    </Box>
  );
}

function OriginDetailView({
  selectedOrigin,
  origins,
  definitions,
  notebookId,
  onBack,
  onSelectOrigin,
}: {
  selectedOrigin: string;
  origins: EtymologyOriginPart[];
  definitions: EtymologyDefinition[];
  notebookId: string;
  onBack: () => void;
  onSelectOrigin: (origin: string) => void;
}) {
  const originData = origins.find((o) => o.origin === selectedOrigin);
  const relatedDefs = definitions.filter((d) =>
    d.originParts.some((p) => p.origin === selectedOrigin),
  );

  return (
    <Box maxW="sm" mx="auto" bg="#f8f9fa" minH="100vh">
      {/* Header */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb">
        <Box px={4} pt={2}>
          <Text
            color="#999"
            fontSize="xs"
            cursor="pointer"
            onClick={onBack}
            _hover={{ textDecoration: "underline" }}
          >
            &lt; Origin List
          </Text>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">Origin Detail</Heading>
        </Box>
      </Box>

      <Box p={4}>
        {/* Origin info card */}
        {originData && (
          <Box
            p={4}
            borderWidth="1px"
            borderRadius="lg"
            mb={4}
            bg="white"
            borderColor="#e5e7eb"
          >
            <Heading size="lg" mb={2}>
              {originData.origin}
            </Heading>
            <Box display="flex" alignItems="center" gap={2} mb={2}>
              {originData.type && <TypeBadge type={originData.type} />}
              <LanguageBadge language={originData.language} />
            </Box>
            <Text color="#666">{originData.meaning}</Text>
          </Box>
        )}

        {/* Words section */}
        <Text fontSize="sm" fontWeight="medium" color="#333" mb={3}>
          Words using this origin ({relatedDefs.length})
        </Text>

        <VStack align="stretch" gap={2}>
          {relatedDefs.map((def, i) => (
            <Box
              key={i}
              p={3}
              borderWidth="1px"
              borderRadius="lg"
              bg="white"
              borderColor="#e5e7eb"
            >
              <Text fontSize="md" fontWeight="semibold" mb={1}>
                {def.expression}
              </Text>
              <Text fontSize="sm" color="#333" mb={2}>
                {def.meaning}
              </Text>
              {def.note && (
                <Text fontSize="xs" color="#666" mb={2} fontStyle="italic">
                  {def.note}
                </Text>
              )}
              {def.examples && def.examples.length > 0 && (
                <Box mb={2} pl={3} borderLeftWidth="2px" borderColor="#e5e7eb">
                  {def.examples.map((ex, k) => (
                    <Text key={k} fontSize="xs" color="#555" fontStyle="italic">
                      {ex}
                    </Text>
                  ))}
                </Box>
              )}
              {def.contexts && def.contexts.length > 0 && (
                <Box mb={2} pl={3} borderLeftWidth="2px" borderColor="#dbeafe">
                  {def.contexts.map((ctx, k) => (
                    <Text key={k} fontSize="xs" color="#555">
                      {ctx}
                    </Text>
                  ))}
                </Box>
              )}
              <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
                {def.originParts.map((part, j) => (
                  <Box key={j} display="flex" alignItems="center" gap={1}>
                    {j > 0 && (
                      <Text fontSize="sm" color="#999">
                        +
                      </Text>
                    )}
                    {part.origin === selectedOrigin ? (
                      <Box
                        px={2}
                        py={0.5}
                        borderRadius="sm"
                        borderWidth="1px"
                        borderColor="#2563eb"
                        bg="#eff6ff"
                      >
                        <Text
                          fontSize="sm"
                          color="#2563eb"
                          fontWeight="semibold"
                        >
                          {part.origin}
                        </Text>
                      </Box>
                    ) : (
                      <Text
                        fontSize="sm"
                        color="#2563eb"
                        fontWeight="medium"
                        cursor="pointer"
                        _hover={{ textDecoration: "underline" }}
                        onClick={() => onSelectOrigin(part.origin)}
                      >
                        {part.origin}
                      </Text>
                    )}
                    {part.origin === selectedOrigin && (
                      <Text fontSize="xs" color="#999">
                        (current)
                      </Text>
                    )}
                  </Box>
                ))}
              </Box>
            </Box>
          ))}
        </VStack>

        {relatedDefs.length > 0 && (
          <Text
            fontSize="xs"
            color="#999"
            textAlign="center"
            mt={4}
          >
            Tap a blue origin to navigate to its page
          </Text>
        )}
      </Box>
    </Box>
  );
}

function ByMeaningView({
  meaningGroups,
  search,
  onSelectOrigin,
}: {
  meaningGroups: EtymologyMeaningGroup[];
  search: string;
  onSelectOrigin: (origin: string) => void;
}) {
  const filtered = useMemo(() => {
    if (!search.trim()) return meaningGroups;
    const lower = search.toLowerCase();
    return meaningGroups.filter(
      (g) =>
        g.meaning.toLowerCase().includes(lower) ||
        g.origins.some((o) => o.origin.toLowerCase().includes(lower)),
    );
  }, [meaningGroups, search]);

  return (
    <VStack align="stretch" gap={2}>
      {filtered.map((group, i) => (
        <Box
          key={i}
          p={3}
          borderWidth="1px"
          borderRadius="lg"
          bg="white"
          borderColor="#e5e7eb"
        >
          <Text fontSize="md" fontWeight="semibold" mb={2}>
            &ldquo;{group.meaning}&rdquo;
          </Text>
          <VStack align="stretch" gap={1}>
            {group.origins.map((origin, j) => (
              <Box
                key={j}
                display="flex"
                alignItems="center"
                gap={2}
                cursor="pointer"
                onClick={() => onSelectOrigin(origin.origin)}
              >
                <Text
                  fontSize="sm"
                  color="#2563eb"
                  fontWeight="medium"
                  _hover={{ textDecoration: "underline" }}
                >
                  {origin.origin}
                </Text>
                <LanguageBadge language={origin.language} />
                <Text fontSize="xs" color="#999">
                  {origin.wordCount}{" "}
                  {origin.wordCount === 1 ? "word" : "words"}
                </Text>
              </Box>
            ))}
          </VStack>
        </Box>
      ))}
      {filtered.length === 0 && (
        <Text color="fg.muted" textAlign="center">
          No meaning groups match your search.
        </Text>
      )}
    </VStack>
  );
}

export default function EtymologyNotebookPage() {
  const params = useParams();
  const id = params.id as string;

  const [origins, setOrigins] = useState<EtymologyOriginPart[]>([]);
  const [definitions, setDefinitions] = useState<EtymologyDefinition[]>([]);
  const [meaningGroups, setMeaningGroups] = useState<EtymologyMeaningGroup[]>(
    [],
  );
  const [originCount, setOriginCount] = useState(0);
  const [definitionCount, setDefinitionCount] = useState(0);
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
        setOriginCount(res.originCount);
        setDefinitionCount(res.definitionCount);

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

  // Origin detail view
  if (selectedOrigin) {
    return (
      <OriginDetailView
        selectedOrigin={selectedOrigin}
        origins={origins}
        definitions={definitions}
        notebookId={id}
        onBack={() => setSelectedOrigin(null)}
        onSelectOrigin={setSelectedOrigin}
      />
    );
  }

  // Origin list view
  return (
    <Box maxW="sm" mx="auto" bg="#f8f9fa" minH="100vh">
      {/* Header */}
      <Box bg="white" borderBottomWidth="1px" borderColor="#e5e7eb">
        <Box px={4} pt={2}>
          <Link href="/notebooks">
            <Text
              color="#999"
              fontSize="xs"
              _hover={{ textDecoration: "underline" }}
            >
              &lt; Notebooks
            </Text>
          </Link>
        </Box>
        <Box px={4} pb={3} textAlign="center">
          <Heading size="md">Etymology</Heading>
        </Box>
      </Box>

      {/* Search bar */}
      <Box px={4} pt={3}>
        <Input
          placeholder="Search origins or meanings..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          borderRadius="lg"
          bg="white"
          borderColor="#d1d5db"
        />
      </Box>

      {/* Tabs */}
      <Box
        bg="white"
        mt={3}
        borderBottomWidth="1px"
        borderColor="#e5e7eb"
        display="flex"
      >
        <Box
          flex={1}
          textAlign="center"
          py={2}
          cursor="pointer"
          onClick={() => setTab("origins")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "origins" ? "semibold" : "normal"}
            color={tab === "origins" ? "#2563eb" : "#999"}
          >
            All Origins
          </Text>
          {tab === "origins" && (
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
          onClick={() => setTab("meanings")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "meanings" ? "semibold" : "normal"}
            color={tab === "meanings" ? "#2563eb" : "#999"}
          >
            By Meaning
          </Text>
          {tab === "meanings" && (
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
        {tab === "origins" ? (
          <VStack align="stretch" gap={2}>
            {filteredOrigins.map((origin, i) => (
              <OriginCard
                key={i}
                origin={origin}
                onClick={() => setSelectedOrigin(origin.origin)}
              />
            ))}
            {filteredOrigins.length === 0 && (
              <Text color="fg.muted" textAlign="center">
                No origins match your search.
              </Text>
            )}
          </VStack>
        ) : (
          <ByMeaningView
            meaningGroups={meaningGroups}
            search={search}
            onSelectOrigin={setSelectedOrigin}
          />
        )}
      </Box>

      {/* Summary footer */}
      <Box
        bg="white"
        borderTopWidth="1px"
        borderColor="#e5e7eb"
        py={3}
        textAlign="center"
      >
        <Text fontSize="sm" color="#666">
          {originCount} origins &middot; {definitionCount} words
        </Text>
      </Box>
    </Box>
  );
}
