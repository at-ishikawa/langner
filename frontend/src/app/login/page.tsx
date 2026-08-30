"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { Box, Button, Heading, Text, VStack } from "@chakra-ui/react";
import { login } from "@/lib/auth";

const errorMessages: Record<string, string> = {
  not_allowed:
    "That Google account is not on the allowlist. Ask an administrator for access.",
  invalid_state: "Your sign-in session expired. Please try again.",
  exchange_failed: "Sign-in with Google failed. Please try again.",
  userinfo_failed: "Could not read your Google profile. Please try again.",
  missing_code: "Sign-in with Google was interrupted. Please try again.",
  server_error: "Something went wrong. Please try again.",
};

function LoginContent() {
  const searchParams = useSearchParams();
  const error = searchParams.get("error");
  const message = error ? (errorMessages[error] ?? errorMessages.server_error) : null;

  return (
    <Box
      minH="100vh"
      display="flex"
      alignItems="center"
      justifyContent="center"
      px={4}
    >
      <VStack gap={6} maxW="sm" textAlign="center">
        <Heading size="lg">Langner</Heading>
        <Text color="fg.muted">Sign in to continue.</Text>
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
        <Button colorPalette="blue" onClick={() => login()} w="100%">
          Sign in with Google
        </Button>
      </VStack>
    </Box>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={null}>
      <LoginContent />
    </Suspense>
  );
}
