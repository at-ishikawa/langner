"use client";

import { Box } from "@chakra-ui/react";
import { ThemeToggle } from "./ThemeToggle";

export function AppHeader() {
  return (
    <Box as="header" borderBottomWidth="1px">
      <Box
        display="flex"
        justifyContent="flex-end"
        alignItems="center"
        maxW="md"
        mx="auto"
        px={4}
        py={2}
      >
        <ThemeToggle />
      </Box>
    </Box>
  );
}
