"use client";

import { Box, Button, Text } from "@chakra-ui/react";
import { ThemeToggle } from "./ThemeToggle";
import { useSession } from "./SessionProvider";
import { logout } from "@/lib/auth";

export function AppHeader() {
  const { user } = useSession();

  return (
    <Box as="header" borderBottomWidth="1px">
      <Box
        display="flex"
        justifyContent="flex-end"
        alignItems="center"
        gap={3}
        maxW="md"
        mx="auto"
        px={4}
        py={2}
      >
        {user && (
          <>
            <Text fontSize="sm" color="fg.muted" truncate maxW="12rem">
              {user.email}
            </Text>
            <Button size="xs" variant="outline" onClick={() => logout()}>
              Log out
            </Button>
          </>
        )}
        <ThemeToggle />
      </Box>
    </Box>
  );
}
