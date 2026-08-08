"use client";

import { Box, Text } from "@chakra-ui/react";

/** OriginPartDisplay is one resolved etymology part (root/prefix/suffix) with
 * its meaning and optional language/type — the shape the origin breakdown row
 * renders. Shared by the quiz result card and the etymology-origin Relearn
 * feedback so both render the roots identically. */
export interface OriginPartDisplay {
  origin: string;
  meaning: string;
  language?: string;
  type?: string;
}

function getTypeBadgeColors(type: string): { bg: string; darkBg: string; color: string; darkColor: string } {
  switch (type.toLowerCase()) {
    case "root":
      return { bg: "blue.100", darkBg: "blue.900", color: "blue.600", darkColor: "blue.300" };
    case "prefix":
      return { bg: "yellow.100", darkBg: "yellow.900", color: "yellow.800", darkColor: "yellow.200" };
    case "suffix":
      return { bg: "green.100", darkBg: "green.900", color: "green.800", darkColor: "green.200" };
    default:
      return { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300" };
  }
}

/** OriginBreakdown renders the roots-with-meanings badge row (origin +
 * (meaning) + language pill + type pill, joined with "+") shown on etymology
 * feedback. The surrounding label (e.g. "Breakdown" / "Etymology") stays with
 * the caller. */
export function OriginBreakdown({ parts }: { parts: OriginPartDisplay[] }) {
  return (
    <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
      {parts.map((p, i) => {
        const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
        return (
          <Box key={i} display="flex" alignItems="center" gap={1}>
            {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
            <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
            <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
            {p.language && (
              <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                <Text fontSize="2xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
              </Box>
            )}
            {typeBadge && p.type && (
              <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                <Text fontSize="2xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
              </Box>
            )}
          </Box>
        );
      })}
    </Box>
  );
}
