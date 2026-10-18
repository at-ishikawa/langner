"use client";

import { Box, Button, Flex, HStack, Spacer, Text } from "@chakra-ui/react";
import NextLink from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { ThemeToggle } from "./ThemeToggle";
import { useSession } from "./SessionProvider";
import { logout } from "@/lib/auth";

// Primary destinations, phone-first. Bottom tab bar on mobile; inline in the
// top bar on desktop. Mapped to the app's real routes.
const NAV: { href: string; label: string; icon: () => ReactNode; exact?: boolean }[] = [
  { href: "/", label: "Today", icon: IconToday, exact: true },
  { href: "/quiz", label: "Quiz", icon: IconQuiz },
  { href: "/analytics", label: "Progress", icon: IconProgress },
  { href: "/learn", label: "Words", icon: IconWords },
];

function Svg({ children }: { children: ReactNode }) {
  return (
    <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {children}
    </svg>
  );
}
function IconToday() { return <Svg><path d="M4 11.5 12 4l8 7.5" /><path d="M6 10v9h12v-9" /></Svg>; }
function IconQuiz() { return <Svg><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3 10h18" /></Svg>; }
function IconProgress() { return <Svg><path d="M4 19V5" /><path d="M4 15l4-4 3 2 5-6 4 4" /></Svg>; }
function IconWords() { return <Svg><path d="M5 4h11l3 3v13H5z" /><path d="M9 4v16" /></Svg>; }

// AppShell is the single application chrome: a slim top bar everywhere, a fixed
// bottom tab bar on mobile, and the primary nav inline on desktop. It reserves
// bottom padding so the fixed bar never covers page content.
export function AppShell({ children }: { children: ReactNode }) {
  const { user } = useSession();
  const pathname = usePathname();
  const isActive = (href: string, exact?: boolean) =>
    exact ? pathname === href : pathname === href || pathname.startsWith(href + "/");

  return (
    <>
      <Box as="header" borderBottomWidth="1px" borderColor="border" bg="bg" position="sticky" top={0} zIndex={20}>
        <Flex maxW="6xl" mx="auto" px={4} py={2} align="center" gap={3}>
          <Button asChild variant="ghost" px={2} fontFamily="heading" fontWeight="600" fontSize="lg" colorPalette="brand">
            <NextLink href="/">Langner</NextLink>
          </Button>

          {user && (
            <HStack gap={1} ml={2} display={{ base: "none", md: "flex" }}>
              {NAV.map((n) => (
                <Button
                  key={n.href}
                  asChild
                  size="sm"
                  variant={isActive(n.href, n.exact) ? "subtle" : "ghost"}
                  colorPalette={isActive(n.href, n.exact) ? "brand" : "gray"}
                >
                  <NextLink href={n.href}>{n.label}</NextLink>
                </Button>
              ))}
            </HStack>
          )}

          <Spacer />

          <HStack gap={2}>
            {user && (
              <>
                <Text fontSize="sm" color="fg.muted" truncate maxW="9rem" display={{ base: "none", sm: "block" }}>
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

      {/* Page content. Reserve room for the fixed mobile tab bar. */}
      <Box pb={user ? { base: "76px", md: 0 } : 0}>{children}</Box>

      {/* Mobile bottom tab bar */}
      {user && (
        <Box
          as="nav"
          aria-label="Primary"
          position="fixed"
          bottom={0}
          left={0}
          right={0}
          zIndex={20}
          bg="bg.subtle"
          borderTopWidth="1px"
          borderColor="border"
          display={{ base: "flex", md: "none" }}
        >
          {NAV.map((n) => {
            const active = isActive(n.href, n.exact);
            const Icon = n.icon;
            return (
              <Button
                key={n.href}
                asChild
                flex="1"
                variant="plain"
                h="auto"
                py={2}
                borderRadius={0}
                color={active ? "brand.fg" : "fg.muted"}
              >
                <NextLink href={n.href}>
                  <Flex direction="column" align="center" gap={1}>
                    <Icon />
                    <Text as="span" fontSize="10px" fontFamily="mono" letterSpacing="0.04em" textTransform="uppercase">
                      {n.label}
                    </Text>
                  </Flex>
                </NextLink>
              </Button>
            );
          })}
        </Box>
      )}
    </>
  );
}
