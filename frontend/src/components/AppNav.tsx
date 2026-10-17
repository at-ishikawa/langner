"use client";

import { Box, Button, Flex, HStack, Spacer, Text } from "@chakra-ui/react";
import NextLink from "next/link";
import { usePathname } from "next/navigation";
import { ThemeToggle } from "./ThemeToggle";
import { useSession } from "./SessionProvider";
import { logout } from "@/lib/auth";

// Primary destinations, shown to a signed-in user. One shared nav replaces the
// old home-page link grid + the user-only header strip, so navigation is the
// same on every page and both desktop and mobile.
const NAV_LINKS = [
  { href: "/learn", label: "Learn" },
  { href: "/quiz", label: "Quiz" },
  { href: "/analytics", label: "Analytics" },
  { href: "/history", label: "History" },
];

export function AppNav() {
  const { user } = useSession();
  const pathname = usePathname();

  const isActive = (href: string) => pathname === href || pathname.startsWith(href + "/");

  return (
    <Box
      as="header"
      borderBottomWidth="1px"
      bg="bg"
      position="sticky"
      top={0}
      zIndex={10}
    >
      <Flex
        maxW="6xl"
        mx="auto"
        px={4}
        py={2}
        align="center"
        gap={{ base: 1, md: 3 }}
        wrap="wrap"
      >
        <Button asChild variant="ghost" px={2} fontWeight="bold" colorPalette="brand">
          <NextLink href="/">Langner</NextLink>
        </Button>

        {user && (
          <HStack gap={0.5} wrap="wrap">
            {NAV_LINKS.map((link) => (
              <Button
                key={link.href}
                asChild
                size="sm"
                variant={isActive(link.href) ? "subtle" : "ghost"}
                colorPalette={isActive(link.href) ? "brand" : "gray"}
              >
                <NextLink href={link.href}>{link.label}</NextLink>
              </Button>
            ))}
          </HStack>
        )}

        <Spacer />

        <HStack gap={2}>
          {user && (
            <>
              <Text
                fontSize="sm"
                color="fg.muted"
                truncate
                maxW="10rem"
                display={{ base: "none", sm: "block" }}
              >
                {user.username}
              </Text>
              <Button asChild size="xs" variant="outline">
                <NextLink href="/settings">Settings</NextLink>
              </Button>
              <Button size="xs" variant="outline" onClick={() => logout()}>
                Log out
              </Button>
            </>
          )}
          <ThemeToggle />
        </HStack>
      </Flex>
    </Box>
  );
}
