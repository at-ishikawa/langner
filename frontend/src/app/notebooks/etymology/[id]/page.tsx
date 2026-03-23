"use client";

import { Suspense, useEffect, useMemo, useState, useCallback } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
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

const typeBadgeColors: Record<string, { bg: string; darkBg: string; color: string; darkColor: string }> = {
  root: { bg: "blue.100", darkBg: "blue.900", color: "blue.800", darkColor: "blue.200" },
  prefix: { bg: "yellow.100", darkBg: "yellow.900", color: "yellow.800", darkColor: "yellow.200" },
  suffix: { bg: "green.100", darkBg: "green.900", color: "green.800", darkColor: "green.200" },
};

function TypeBadge({ type }: { type: string }) {
  const colors = typeBadgeColors[type.toLowerCase()] ?? {
    bg: "gray.100",
    darkBg: "gray.700",
    color: "gray.700",
    darkColor: "gray.300",
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
      _dark={{ bg: colors.darkBg, color: colors.darkColor }}
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
      bg="gray.100"
      color="gray.600"
      _dark={{ bg: "gray.700", color: "gray.300" }}
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
      _dark={{ bg: "gray.800", borderColor: "gray.600" }}
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
            color="blue.600"
            _dark={{ color: "blue.300" }}
          >
            {origin.origin}
          </Text>
          {origin.type && <TypeBadge type={origin.type} />}
          <LanguageBadge language={origin.language} />
        </Box>
        <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }} flexShrink={0}>
          {origin.wordCount} {origin.wordCount === 1 ? "word" : "words"}
        </Text>
      </Box>
      <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>
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
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh">
      {/* Header */}
      <Box bg="white" _dark={{ bg: "gray.800", borderColor: "gray.600" }} borderBottomWidth="1px" borderColor="gray.200">
        <Box px={4} pt={2}>
          <Text
            color="gray.500"
            _dark={{ color: "gray.400" }}
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
            _dark={{ bg: "gray.800", borderColor: "gray.600" }}
            borderColor="gray.200"
          >
            <Heading size="lg" mb={2}>
              {originData.origin}
            </Heading>
            <Box display="flex" alignItems="center" gap={2} mb={2}>
              {originData.type && <TypeBadge type={originData.type} />}
              <LanguageBadge language={originData.language} />
            </Box>
            <Text color="gray.600" _dark={{ color: "gray.400" }}>{originData.meaning}</Text>
          </Box>
        )}

        {/* Words section */}
        <Text fontSize="sm" fontWeight="medium" color="gray.700" _dark={{ color: "gray.300" }} mb={3}>
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
              _dark={{ bg: "gray.800", borderColor: "gray.600" }}
              borderColor="gray.200"
            >
              <Text fontSize="md" fontWeight="semibold" mb={1}>
                {def.expression}
              </Text>
              <Text fontSize="sm" color="gray.700" _dark={{ color: "gray.300" }} mb={2}>
                {def.meaning}
              </Text>
              {def.note && (
                <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }} mb={2} fontStyle="italic">
                  {def.note}
                </Text>
              )}
              {def.examples && def.examples.length > 0 && (
                <Box mb={2} pl={3} borderLeftWidth="2px" borderColor="gray.200" _dark={{ borderColor: "gray.600" }}>
                  {def.examples.map((ex, k) => (
                    <Text key={k} fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }} fontStyle="italic">
                      {ex}
                    </Text>
                  ))}
                </Box>
              )}
              {def.contexts && def.contexts.length > 0 && (
                <Box mb={2} pl={3} borderLeftWidth="2px" borderColor="blue.100" _dark={{ borderColor: "blue.800" }}>
                  {def.contexts.map((ctx, k) => (
                    <Text key={k} fontSize="xs" color="gray.600" _dark={{ color: "gray.400" }}>
                      {ctx}
                    </Text>
                  ))}
                </Box>
              )}
              <Box display="flex" alignItems="center" gap={1} flexWrap="wrap">
                {def.originParts.map((part, j) => (
                  <Box key={j} display="flex" alignItems="center" gap={1}>
                    {j > 0 && (
                      <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
                        +
                      </Text>
                    )}
                    {part.origin === selectedOrigin ? (
                      <Box
                        px={2}
                        py={0.5}
                        borderRadius="sm"
                        borderWidth="1px"
                        borderColor="blue.600"
                        bg="blue.50"
                        _dark={{ bg: "blue.900", borderColor: "blue.400" }}
                      >
                        <Text
                          fontSize="sm"
                          color="blue.600"
                          _dark={{ color: "blue.300" }}
                          fontWeight="semibold"
                        >
                          {part.origin}
                        </Text>
                      </Box>
                    ) : (
                      <Text
                        fontSize="sm"
                        color="blue.600"
                        _dark={{ color: "blue.300" }}
                        fontWeight="medium"
                        cursor="pointer"
                        _hover={{ textDecoration: "underline" }}
                        onClick={() => onSelectOrigin(part.origin)}
                      >
                        {part.origin}
                      </Text>
                    )}
                    {part.origin === selectedOrigin && (
                      <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
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
            color="gray.500"
            _dark={{ color: "gray.400" }}
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
          _dark={{ bg: "gray.800", borderColor: "gray.600" }}
          borderColor="gray.200"
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
                  color="blue.600"
                  _dark={{ color: "blue.300" }}
                  fontWeight="medium"
                  _hover={{ textDecoration: "underline" }}
                >
                  {origin.origin}
                </Text>
                <LanguageBadge language={origin.language} />
                <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }}>
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

export default function EtymologyNotebookPageWrapper() {
  return (
    <Suspense fallback={<Box p={4} maxW="sm" mx="auto" textAlign="center"><Spinner size="lg" /></Box>}>
      <EtymologyNotebookPage />
    </Suspense>
  );
}

function EtymologyNotebookPage() {
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const id = params.id as string;
  const selectedOrigin = searchParams.get("origin");

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

  const selectOrigin = useCallback(
    (origin: string) => {
      router.push(`/notebooks/etymology/${id}?origin=${encodeURIComponent(origin)}`);
    },
    [router, id],
  );

  const clearOrigin = useCallback(() => {
    router.push(`/notebooks/etymology/${id}`);
  }, [router, id]);

  useEffect(() => {
    notebookClient
      .getEtymologyNotebook({ notebookId: id })
      .then((res) => {
        setOrigins(res.origins ?? []);
        setDefinitions(res.definitions ?? []);
        setMeaningGroups(res.meaningGroups ?? []);
        setOriginCount(res.originCount);
        setDefinitionCount(res.definitionCount);
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
        onBack={clearOrigin}
        onSelectOrigin={selectOrigin}
      />
    );
  }

  // Origin list view
  return (
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh">
      {/* Header */}
      <Box bg="white" _dark={{ bg: "gray.800", borderColor: "gray.600" }} borderBottomWidth="1px" borderColor="gray.200">
        <Box px={4} pt={2}>
          <Link href="/learn">
            <Text
              color="gray.500"
              _dark={{ color: "gray.400" }}
              fontSize="xs"
              _hover={{ textDecoration: "underline" }}
            >
              &lt; Learn
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
          borderColor="gray.300"
          _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        />
      </Box>

      {/* Tabs */}
      <Box
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        mt={3}
        borderBottomWidth="1px"
        borderColor="gray.200"
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
            color={tab === "origins" ? "blue.600" : "gray.500"}
            _dark={{ color: tab === "origins" ? "blue.300" : "gray.400" }}
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
          onClick={() => setTab("meanings")}
          position="relative"
        >
          <Text
            fontSize="sm"
            fontWeight={tab === "meanings" ? "semibold" : "normal"}
            color={tab === "meanings" ? "blue.600" : "gray.500"}
            _dark={{ color: tab === "meanings" ? "blue.300" : "gray.400" }}
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
              bg="blue.600"
              _dark={{ bg: "blue.300" }}
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
                onClick={() => selectOrigin(origin.origin)}
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
            onSelectOrigin={selectOrigin}
          />
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
      >
        <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>
          {originCount} origins &middot; {definitionCount} words
        </Text>
      </Box>
    </Box>
  );
}
