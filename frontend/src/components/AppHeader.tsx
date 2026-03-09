"use client";

import { Box } from "@chakra-ui/react";
import { ThemeToggle } from "./ThemeToggle";

export function AppHeader() {
  return (
    <Box
      as="header"
      display="flex"
      justifyContent="flex-end"
      alignItems="center"
      px={4}
      py={2}
      borderBottomWidth="1px"
    >
      <ThemeToggle />
    </Box>
  );
}
