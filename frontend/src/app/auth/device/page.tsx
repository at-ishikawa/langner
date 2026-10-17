"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { Box, Button, Code, Heading, Input, Text, VStack } from "@chakra-ui/react";

// The browser-facing API origin (same source the Connect client uses). The
// device-approval form submits DIRECTLY to the API so the OAuth state cookie
// and the Google callback share the API origin — the approval page itself is a
// frontend route; the API renders no user-facing HTML, it only redirects back
// here with ?result=approved|denied.
const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

function Centered({ children }: { children: React.ReactNode }) {
  return (
    <Box minH="100vh" display="flex" alignItems="center" justifyContent="center" p={4}>
      <VStack gap={4} maxW="32rem" textAlign="center">
        {children}
      </VStack>
    </Box>
  );
}

function DeviceApproval() {
  const params = useSearchParams();
  const result = params.get("result");
  const userCode = (params.get("user_code") ?? "").toUpperCase();

  if (result === "approved") {
    return (
      <Centered>
        <Heading size="lg">Approved</Heading>
        <Text color="fg.muted">You can return to your terminal — the CLI is now signed in.</Text>
      </Centered>
    );
  }
  if (result === "denied") {
    return (
      <Centered>
        <Heading size="lg">Denied</Heading>
        <Text color="fg.muted">The sign-in request was denied. You can close this tab.</Text>
      </Centered>
    );
  }

  return (
    <Centered>
      <Heading size="lg">Approve CLI access?</Heading>
      <Text color="fg.muted">
        Enter the code shown in your terminal by <Code>langner login</Code>, then approve. Approving
        takes you through a one-time Google sign-in; the account you sign in as is the account the
        CLI will act as. Only approve a code you started yourself.
      </Text>
      {/* GET → the API's Google sign-in (folds user_code into the signed OAuth
          state); the Deny button POSTs to the API, which redirects back here. */}
      <form method="get" action={`${API_BASE}/auth/google/login`} style={{ width: "100%" }}>
        <VStack gap={4}>
          <Input
            name="user_code"
            defaultValue={userCode}
            placeholder="XXXX-XXXX"
            required
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            textAlign="center"
            textTransform="uppercase"
            letterSpacing="0.15em"
            fontSize="xl"
            maxW="12rem"
          />
          <Box display="flex" gap={3}>
            <Button type="submit" colorPalette="blue">
              Approve &amp; sign in with Google
            </Button>
            <Button type="submit" variant="outline" formMethod="post" formAction={`${API_BASE}/auth/device/deny`}>
              Deny
            </Button>
          </Box>
        </VStack>
      </form>
    </Centered>
  );
}

// useSearchParams requires a Suspense boundary; force-dynamic so the page is
// never statically prerendered (it reads per-request query params).
export const dynamic = "force-dynamic";

export default function DeviceApprovalPage() {
  return (
    <Suspense fallback={null}>
      <DeviceApproval />
    </Suspense>
  );
}
