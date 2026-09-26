"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Box, Spinner, Text, VStack } from "@chakra-ui/react";
import { setAccessToken } from "@/lib/authToken";

// AuthCallbackPage receives the langner access token from the OAuth callback in
// the URL fragment (#access_token=…&next=…). The fragment is never sent to a
// server; this client page reads it into the in-memory token store, scrubs it
// from the URL/history, and forwards the user to `next` (default home). With no
// token it falls back to /login.
export default function AuthCallbackPage() {
  const router = useRouter();

  useEffect(() => {
    const hash = window.location.hash.startsWith("#")
      ? window.location.hash.slice(1)
      : window.location.hash;
    const params = new URLSearchParams(hash);
    const token = params.get("access_token");
    const next = params.get("next") ?? "/";

    // Scrub the token from the URL/history so it never lingers in the address bar.
    window.history.replaceState(null, "", "/auth/callback");

    if (token) {
      setAccessToken(token);
      router.replace(next.startsWith("/") ? next : "/");
    } else {
      router.replace("/login");
    }
  }, [router]);

  return (
    <Box minH="100vh" display="flex" alignItems="center" justifyContent="center">
      <VStack gap={3}>
        <Spinner />
        <Text color="fg.muted">Signing you in…</Text>
      </VStack>
    </Box>
  );
}
