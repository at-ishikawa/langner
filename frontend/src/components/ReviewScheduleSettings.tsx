"use client";

import { useEffect, useState } from "react";
import { Box, Button, Input, Text } from "@chakra-ui/react";
import { ConnectError } from "@connectrpc/connect";
import { Card } from "@/components/Card";
import { quizClient } from "@/lib/client";

// The two schedules a learner can pick. The values are the backend's
// algorithm names; the labels avoid jargon.
const FIXED = "fixed";
const ADAPTIVE = "modified_sm2";

interface Schedule {
  algorithm: string;
  fixedIntervals: number[];
}

interface Loaded {
  settings: Schedule;
  isDefault: boolean;
  defaultSettings: Schedule;
}

function toSchedule(s: { algorithm: string; fixedIntervals: number[] } | undefined): Schedule {
  return { algorithm: s?.algorithm ?? "", fixedIntervals: s?.fixedIntervals ?? [] };
}

function describe(s: Schedule): string {
  return s.algorithm === FIXED
    ? `fixed steps of ${s.fixedIntervals.join(", ")} days`
    : "adaptive";
}

// parseDays reads "1, 7, 30" into [1, 7, 30], or returns null when an entry
// is not a whole number. Order and range are checked by the server.
export function parseDays(text: string): number[] | null {
  const parts = text.split(",").map((p) => p.trim()).filter((p) => p !== "");
  if (parts.some((p) => !/^\d+$/.test(p))) return null;
  return parts.map(Number);
}

function errorMessage(e: unknown): string {
  if (e instanceof ConnectError && e.rawMessage) return e.rawMessage;
  return e instanceof Error ? e.message : "Something went wrong";
}

// ReviewScheduleSettings lets the signed-in learner choose how long Langner
// waits before asking a word again. Unset follows the server default.
export function ReviewScheduleSettings() {
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [algorithm, setAlgorithm] = useState(FIXED);
  const [daysText, setDaysText] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ kind: "saved" | "error"; text: string } | null>(null);

  const apply = (res: { settings?: Schedule; isDefault: boolean; defaultSettings?: Schedule }) => {
    const next: Loaded = {
      settings: toSchedule(res.settings),
      isDefault: res.isDefault,
      defaultSettings: toSchedule(res.defaultSettings),
    };
    setLoaded(next);
    setAlgorithm(next.settings.algorithm);
    const days = next.settings.algorithm === FIXED ? next.settings : next.defaultSettings;
    setDaysText(days.fixedIntervals.join(", "));
  };

  useEffect(() => {
    quizClient
      .getIntervalSettings({})
      .then(apply)
      .catch((e: unknown) => setMessage({ kind: "error", text: errorMessage(e) }));
  }, []);

  const run = async (fn: () => Promise<void>) => {
    setSaving(true);
    setMessage(null);
    try {
      await fn();
      setMessage({ kind: "saved", text: "Saved." });
    } catch (e) {
      setMessage({ kind: "error", text: errorMessage(e) });
    } finally {
      setSaving(false);
    }
  };

  const save = () =>
    run(async () => {
      let fixedIntervals: number[] = [];
      if (algorithm === FIXED) {
        const days = parseDays(daysText);
        if (!days || days.length === 0) {
          throw new Error("Enter whole numbers of days separated by commas, like 1, 7, 30.");
        }
        fixedIntervals = days;
      }
      apply(await quizClient.updateIntervalSettings({ settings: { algorithm, fixedIntervals } }));
    });

  const reset = () =>
    run(async () => {
      apply(await quizClient.updateIntervalSettings({ resetToDefault: true }));
    });

  return (
    <Card p={4}>
      <Text fontSize="sm" color="fg.muted" mb={3}>
        Choose how long to wait before a word comes back. Changes apply to your next
        answers; words you have already studied keep the review dates they have now.
      </Text>

      <Box display="flex" gap={2} mb={3} flexWrap="wrap">
        <Button
          size="sm"
          variant={algorithm === FIXED ? "solid" : "outline"}
          colorPalette={algorithm === FIXED ? "brand" : undefined}
          aria-pressed={algorithm === FIXED}
          onClick={() => setAlgorithm(FIXED)}
        >
          Fixed steps
        </Button>
        <Button
          size="sm"
          variant={algorithm === ADAPTIVE ? "solid" : "outline"}
          colorPalette={algorithm === ADAPTIVE ? "brand" : undefined}
          aria-pressed={algorithm === ADAPTIVE}
          onClick={() => setAlgorithm(ADAPTIVE)}
        >
          Adaptive
        </Button>
      </Box>

      {algorithm === FIXED ? (
        <Box mb={3}>
          <Text fontSize="sm" mb={1}>
            Each right answer moves a word one step further; a wrong answer moves it
            one step back. Enter the waits in days, shortest first.
          </Text>
          <Input
            aria-label="Days between reviews"
            placeholder="1, 7, 30, 90, 365"
            value={daysText}
            onChange={(e) => setDaysText(e.target.value)}
          />
        </Box>
      ) : (
        <Text fontSize="sm" mb={3}>
          The wait grows quickly for words you find easy and slowly for words you
          find hard.
        </Text>
      )}

      {loaded && (
        <Text fontSize="xs" color="fg.muted" mb={3}>
          {loaded.isDefault
            ? `You are using the default schedule (${describe(loaded.defaultSettings)}).`
            : `The default schedule is ${describe(loaded.defaultSettings)}.`}
        </Text>
      )}

      <Box display="flex" gap={3} flexWrap="wrap">
        <Button colorPalette="brand" size="sm" onClick={save} loading={saving} disabled={!loaded}>
          Save schedule
        </Button>
        {loaded && !loaded.isDefault && (
          <Button size="sm" variant="outline" onClick={reset} loading={saving}>
            Use the default
          </Button>
        )}
      </Box>

      {message?.kind === "saved" && (
        <Box role="status" bg="green.subtle" color="green.fg" borderRadius="md" px={3} py={2} mt={3} fontSize="sm">
          {message.text}
        </Box>
      )}
      {message?.kind === "error" && (
        <Box role="alert" bg="red.subtle" color="red.fg" borderRadius="md" px={3} py={2} mt={3} fontSize="sm">
          {message.text}
        </Box>
      )}
    </Card>
  );
}
