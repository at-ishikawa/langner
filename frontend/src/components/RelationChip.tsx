"use client";

import { Box, Text } from "@chakra-ui/react";

// relationColors maps a relation type to its Chakra color token. Unknown
// types fall back to gray so user-invented relation strings still render.
const relationColors: Record<string, { bg: string; darkBg: string; color: string; darkColor: string }> = {
  antonym:    { bg: "red.100",    darkBg: "red.900",    color: "red.800",    darkColor: "red.200" },
  synonym:    { bg: "blue.100",   darkBg: "blue.900",   color: "blue.800",   darkColor: "blue.200" },
  hypernym:   { bg: "purple.100", darkBg: "purple.900", color: "purple.800", darkColor: "purple.200" },
  hyponym:    { bg: "purple.100", darkBg: "purple.900", color: "purple.800", darkColor: "purple.200" },
  holonym:    { bg: "teal.100",   darkBg: "teal.900",   color: "teal.800",   darkColor: "teal.200" },
  meronym:    { bg: "teal.100",   darkBg: "teal.900",   color: "teal.800",   darkColor: "teal.200" },
  similar_to: { bg: "green.100",  darkBg: "green.900",  color: "green.800",  darkColor: "green.200" },
  causes:     { bg: "orange.100", darkBg: "orange.900", color: "orange.800", darkColor: "orange.200" },
  entails:    { bg: "orange.100", darkBg: "orange.900", color: "orange.800", darkColor: "orange.200" },
};

const fallbackColor = { bg: "gray.100", darkBg: "gray.700", color: "gray.800", darkColor: "gray.200" };

interface RelationChipProps {
  type: string;        // relation type (e.g. "antonym", "hyponym")
  label: string;       // the visible text — typically the target concept's meaning or key
  onClick?: () => void;
}

// RelationChip renders one typed edge as a compact pill. Used inside
// ConceptCard's footer and inline on origin chips (the "antonym → right
// side" affordance). The color is keyed off the relation type so the
// learner picks up the type at a glance.
export function RelationChip({ type, label, onClick }: RelationChipProps) {
  const c = relationColors[type.toLowerCase()] ?? fallbackColor;
  return (
    <Box
      display="inline-flex"
      alignItems="center"
      gap={1}
      px={2}
      py={0.5}
      borderRadius="full"
      bg={c.bg}
      _dark={{ bg: c.darkBg }}
      cursor={onClick ? "pointer" : "default"}
      onClick={onClick}
    >
      <Text fontSize="xs" color={c.color} _dark={{ color: c.darkColor }} fontWeight="medium">
        {type}
      </Text>
      <Text fontSize="xs" color={c.color} _dark={{ color: c.darkColor }}>
        → {label}
      </Text>
    </Box>
  );
}
