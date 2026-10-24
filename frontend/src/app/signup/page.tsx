"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import NextLink from "next/link";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { signup } from "@/lib/auth";

// Account creation is Google-only and invite-based: Langner stores no password,
// and the server provisions the account on the first successful Google sign-in
// whose email is on the allowlist. So "Sign up with Google" runs the same OAuth
// flow as sign-in; a non-allowlisted email is bounced back to /login with
// error=not_allowed (handled there), which we also surface here if routed back.
const errorMessages: Record<string, string> = {
  not_allowed:
    "That Google account is not on the allowlist yet. Ask an administrator for access.",
  invalid_state: "Your sign-up session expired. Please try again.",
  exchange_failed: "Sign-up with Google failed. Please try again.",
  userinfo_failed: "Could not read your Google profile. Please try again.",
  missing_code: "Sign-up with Google was interrupted. Please try again.",
  server_error: "Something went wrong. Please try again.",
};

function SignupContent() {
  const searchParams = useSearchParams();
  const error = searchParams.get("error");
  const message = error ? (errorMessages[error] ?? errorMessages.server_error) : null;

  return (
    <Box minH="100vh" display="flex" alignItems="center" justifyContent="center" px={4}>
      <VStack gap={6} maxW="sm" textAlign="center">
        <Heading size="lg">Create your account</Heading>
        <Text color="fg.muted">
          Langner uses your Google account — there is no separate password to set. We
          create your account the first time you sign in.
        </Text>
        {message && (
          <Box
            role="alert"
            w="100%"
            borderWidth="1px"
            borderColor="red.emphasized"
            bg="red.subtle"
            color="red.fg"
            borderRadius="md"
            px={4}
            py={3}
            fontSize="sm"
          >
            {message}
          </Box>
        )}
        <Button colorPalette="brand" onClick={() => signup()} w="100%">
          Sign up with Google
        </Button>
        <Text color="fg.muted" fontSize="sm">
          Already have an account?{" "}
          <Button
            asChild
            variant="plain"
            size="sm"
            color="brand.fg"
            h="auto"
            py={0}
            textDecoration="underline"
          >
            <NextLink href="/login">Sign in</NextLink>
          </Button>
        </Text>
      </VStack>
    </Box>
  );
}

export default function SignupPage() {
  return (
    <Suspense fallback={null}>
      <SignupContent />
    </Suspense>
  );
}
