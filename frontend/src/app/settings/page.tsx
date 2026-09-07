"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Box,
  Button,
  Heading,
  Input,
  NativeSelect,
  Text,
  VStack,
} from "@chakra-ui/react";
import { useSession } from "@/components/SessionProvider";
import { setLlmCredential, deleteLlmCredential } from "@/lib/auth";

type Status = "idle" | "saving" | "saved" | "error";

export default function SettingsPage() {
  const { user, loading, refresh } = useSession();

  const providers = user?.availableProviders ?? [];
  const [provider, setProvider] = useState<string>("");
  const [apiKey, setApiKey] = useState("");
  const [model, setModel] = useState("");
  const [status, setStatus] = useState<Status>("idle");
  const [error, setError] = useState<string | null>(null);

  // Default the provider select to the user's current provider, else the first
  // available one, once the session has loaded.
  const effectiveProvider =
    provider || user?.provider || providers[0] || "";

  if (loading) {
    return (
      <Box p={4} maxW="md" mx="auto">
        <Text color="fg.muted">Loading…</Text>
      </Box>
    );
  }

  if (!user) {
    return (
      <Box p={4} maxW="md" mx="auto">
        <Text color="fg.muted">
          You must be signed in to manage settings.
        </Text>
      </Box>
    );
  }

  const handleSave = async () => {
    if (!apiKey.trim()) {
      setError("Enter an API key.");
      setStatus("error");
      return;
    }
    setStatus("saving");
    setError(null);
    try {
      await setLlmCredential({
        provider: effectiveProvider,
        apiKey: apiKey.trim(),
        model: model.trim(),
      });
      setApiKey("");
      await refresh();
      setStatus("saved");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save API key");
      setStatus("error");
    }
  };

  const handleRemove = async () => {
    setStatus("saving");
    setError(null);
    try {
      await deleteLlmCredential();
      await refresh();
      setStatus("idle");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to remove API key");
      setStatus("error");
    }
  };

  return (
    <Box p={4} maxW="md" mx="auto">
      <Box mb={4}>
        <Link href="/">
          <Text color="blue.600" _dark={{ color: "blue.300" }} fontSize="xs">
            &lt; Home
          </Text>
        </Link>
      </Box>
      <Heading size="lg" mb={4}>
        Settings
      </Heading>

      <VStack align="stretch" gap={4}>
        <Box>
          <Heading size="sm" mb={1}>
            LLM API key
          </Heading>
          <Text fontSize="sm" color="fg.muted">
            Quizzes are graded with your own provider key. It is stored encrypted
            and never shown again.
          </Text>
        </Box>

        <Box
          borderWidth="1px"
          borderColor="border"
          borderRadius="md"
          px={3}
          py={2}
        >
          <Text fontSize="sm">
            {user.hasApiKey ? (
              <>
                Current: <b>{user.provider || "unknown"}</b>
                {user.model ? ` (${user.model})` : " (default model)"} — key set.
              </>
            ) : (
              "No API key configured yet."
            )}
          </Text>
        </Box>

        <Box>
          <Text fontSize="sm" mb={1}>
            Provider
          </Text>
          <NativeSelect.Root size="sm">
            <NativeSelect.Field
              aria-label="LLM provider"
              value={effectiveProvider}
              onChange={(e) => setProvider(e.target.value)}
            >
              {providers.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </NativeSelect.Field>
            <NativeSelect.Indicator />
          </NativeSelect.Root>
        </Box>

        <Box>
          <Text fontSize="sm" mb={1}>
            API key
          </Text>
          <Input
            type="password"
            placeholder="sk-…"
            value={apiKey}
            autoComplete="off"
            onChange={(e) => setApiKey(e.target.value)}
          />
        </Box>

        <Box>
          <Text fontSize="sm" mb={1}>
            Model <Text as="span" color="fg.muted">(optional)</Text>
          </Text>
          <Input
            placeholder="e.g. gpt-4o-mini"
            value={model}
            onChange={(e) => setModel(e.target.value)}
          />
        </Box>

        {status === "saved" && (
          <Box
            role="status"
            borderWidth="1px"
            borderColor="green.emphasized"
            bg="green.subtle"
            color="green.fg"
            borderRadius="md"
            px={3}
            py={2}
            fontSize="sm"
          >
            API key saved.
          </Box>
        )}
        {status === "error" && error && (
          <Box
            role="alert"
            borderWidth="1px"
            borderColor="red.emphasized"
            bg="red.subtle"
            color="red.fg"
            borderRadius="md"
            px={3}
            py={2}
            fontSize="sm"
          >
            {error}
          </Box>
        )}

        <Box display="flex" gap={3}>
          <Button
            colorPalette="blue"
            onClick={handleSave}
            loading={status === "saving"}
            disabled={!effectiveProvider}
          >
            Save
          </Button>
          {user.hasApiKey && (
            <Button
              variant="outline"
              onClick={handleRemove}
              loading={status === "saving"}
            >
              Remove key
            </Button>
          )}
        </Box>
      </VStack>
    </Box>
  );
}
