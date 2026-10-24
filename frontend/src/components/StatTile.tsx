"use client";

import { Box, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";

// A single headline metric: big mono number (tabular) + a quiet label. `accent`
// paints it in the brand wash for the primary stat on a screen.
export function StatTile({
  value,
  label,
  accent = false,
}: {
  value: ReactNode;
  label: string;
  accent?: boolean;
}) {
  return (
    <Box
      bg={accent ? "brand.subtle" : "bg.subtle"}
      borderWidth="1px"
      borderColor={accent ? "transparent" : "border"}
      borderRadius="14px"
      px={4}
      py={3}
    >
      <Text
        fontFamily="mono"
        fontSize="28px"
        fontWeight="600"
        letterSpacing="-0.02em"
        color={accent ? "brand.fg" : "fg"}
        css={{ fontVariantNumeric: "tabular-nums" }}
      >
        {value}
      </Text>
      <Text fontSize="sm" color="fg.muted">
        {label}
      </Text>
    </Box>
  );
}
