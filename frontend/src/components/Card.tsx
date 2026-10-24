"use client";

import { Box, type BoxProps } from "@chakra-ui/react";

// The one surface primitive: a panel that sits above the page ground, themed
// via tokens. Replaces the many hand-styled `<Box bg="white" _dark=…>` cards.
export function Card(props: BoxProps) {
  return (
    <Box
      bg="panel"
      borderWidth="1px"
      borderColor="border"
      borderRadius="16px"
      {...props}
    />
  );
}
