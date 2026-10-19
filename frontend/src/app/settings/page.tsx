"use client";

import { useState } from "react";
import Link from "next/link";
import { Box, Button, Heading, Input, Text, VStack } from "@chakra-ui/react";
import { Card } from "@/components/Card";
import { ThemeToggle } from "@/components/ThemeToggle";
import { useSession } from "@/components/SessionProvider";
import {
  setLlmCredential,
  setPrimaryProvider,
  deleteLlmCredential,
  logout,
} from "@/lib/auth";

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <Text
      fontFamily="mono"
      fontSize="xs"
      letterSpacing="0.12em"
      textTransform="uppercase"
      color="fg.muted"
      mb={2}
    >
      {children}
    </Text>
  );
}

type Status = "idle" | "saving" | "saved" | "error";

export default function SettingsPage() {
  const { user, loading, refresh } = useSession();
  const [keys, setKeys] = useState<Record<string, string>>({});
  const [models, setModels] = useState<Record<string, string>>({});
  const [status, setStatus] = useState<Status>("idle");
  const [error, setError] = useState<string | null>(null);

  if (loading) {
    return (
      <Box p={4} maxW="2xl" mx="auto">
        <Text color="fg.muted">Loading…</Text>
      </Box>
    );
  }
  if (!user) {
    return (
      <Box p={4} maxW="2xl" mx="auto">
        <Text color="fg.muted">You must be signed in to manage settings.</Text>
      </Box>
    );
  }

  const providers = user.availableProviders;
  const credFor = (p: string) => user.credentials.find((c) => c.provider === p);

  const run = async (fn: () => Promise<void>) => {
    setStatus("saving");
    setError(null);
    try {
      await fn();
      await refresh();
      setStatus("saved");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Something went wrong");
      setStatus("error");
    }
  };

  const save = (provider: string) =>
    run(async () => {
      const apiKey = (keys[provider] ?? "").trim();
      if (!apiKey) throw new Error("Enter an API key.");
      await setLlmCredential({ provider, apiKey, model: (models[provider] ?? "").trim() });
      setKeys((k) => ({ ...k, [provider]: "" }));
    });

  return (
    <Box p={4} maxW="2xl" mx="auto">
      <Box mb={4}>
        <Link href="/">
          <Text color="brand.fg" fontSize="xs">
            &lt; Home
          </Text>
        </Link>
      </Box>
      <Heading size="lg" mb={5}>
        Settings
      </Heading>

      <Box mb={7}>
        <SectionLabel>Appearance</SectionLabel>
        <Card p={4} display="flex" alignItems="center" justifyContent="space-between" gap={3}>
          <Box>
            <Text fontWeight="500" fontSize="sm">
              Theme
            </Text>
            <Text fontSize="xs" color="fg.muted">
              Light, dark, or match your device.
            </Text>
          </Box>
          <ThemeToggle />
        </Card>
      </Box>

      <SectionLabel>AI grading</SectionLabel>
      <Text fontSize="sm" color="fg.muted" mb={4}>
        Quizzes are graded with your own provider key(s), stored encrypted and
        never shown again. Set a key for more than one provider and grading
        automatically falls back to the other when your primary hits a rate
        limit.
      </Text>

      {!user.hasApiKey && (
        <Box
          role="note"
          borderWidth="1px"
          borderColor="border"
          borderRadius="md"
          px={3}
          py={2}
          mb={4}
          fontSize="sm"
        >
          No API key configured yet.
        </Box>
      )}

      <VStack align="stretch" gap={5}>
        {providers.map((provider) => {
          const cred = credFor(provider);
          return (
            <Card key={provider} p={3}>
              <Box display="flex" justifyContent="space-between" alignItems="center" mb={1}>
                <Heading size="sm" textTransform="capitalize">
                  {provider}
                </Heading>
                {cred?.primary && (
                  <Text fontSize="xs" color="brand.fg" fontWeight="bold">
                    primary
                  </Text>
                )}
              </Box>
              <Text fontSize="sm" color="fg.muted" mb={2}>
                {cred ? `Key set${cred.model ? ` (${cred.model})` : ""}.` : "No key set."}
              </Text>
              <Input
                type="password"
                placeholder="sk-…"
                aria-label={`${provider} API key`}
                value={keys[provider] ?? ""}
                autoComplete="off"
                mb={2}
                onChange={(e) => setKeys((k) => ({ ...k, [provider]: e.target.value }))}
              />
              <Input
                placeholder="model (optional)"
                aria-label={`${provider} model`}
                value={models[provider] ?? ""}
                mb={2}
                onChange={(e) => setModels((m) => ({ ...m, [provider]: e.target.value }))}
              />
              <Box display="flex" gap={3}>
                <Button
                  colorPalette="brand"
                  size="sm"
                  onClick={() => save(provider)}
                  loading={status === "saving"}
                >
                  Save {provider} key
                </Button>
                {cred && !cred.primary && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => run(() => setPrimaryProvider(provider))}
                    loading={status === "saving"}
                  >
                    Make {provider} primary
                  </Button>
                )}
                {cred && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => run(() => deleteLlmCredential(provider))}
                    loading={status === "saving"}
                  >
                    Remove {provider}
                  </Button>
                )}
              </Box>
            </Card>
          );
        })}
      </VStack>

      {status === "saved" && (
        <Box role="status" bg="green.subtle" color="green.fg" borderRadius="md" px={3} py={2} mt={4} fontSize="sm">
          Saved.
        </Box>
      )}
      {status === "error" && error && (
        <Box role="alert" bg="red.subtle" color="red.fg" borderRadius="md" px={3} py={2} mt={4} fontSize="sm">
          {error}
        </Box>
      )}

      <Box mt={8}>
        <SectionLabel>Account</SectionLabel>
        <Card p={4} display="flex" alignItems="center" justifyContent="space-between" gap={3}>
          <Box minW={0}>
            <Text fontWeight="500" fontSize="sm" truncate>
              {user.username}
            </Text>
            <Text fontSize="xs" color="fg.muted">
              Signed in
            </Text>
          </Box>
          <Button size="sm" variant="outline" flexShrink={0} onClick={() => logout()}>
            Log out
          </Button>
        </Card>
      </Box>
    </Box>
  );
}
