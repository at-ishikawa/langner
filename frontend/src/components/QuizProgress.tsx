"use client";

import { Box, Progress, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";

// The one quiz session header: a mono, tabular left label (e.g. "3 / 12" or
// "2 words left"), an optional right-aligned secondary stat (grammar's
// "4 / 9 correct"), and a brand progress bar. Every quiz mode uses this so the
// top of a session looks identical across vocabulary, reverse, relearn, and
// grammar. `progress` is 0–100.
export function QuizProgress({
  label,
  secondary,
  progress,
}: {
  label: ReactNode;
  secondary?: ReactNode;
  progress: number;
}) {
  return (
    <Box mb={5}>
      <Box display="flex" alignItems="baseline" justifyContent="space-between" gap={3} mb={1}>
        <Text
          fontFamily="mono"
          fontSize="sm"
          color="fg.muted"
          css={{ fontVariantNumeric: "tabular-nums" }}
        >
          {label}
        </Text>
        {secondary != null && (
          <Text
            fontFamily="mono"
            fontSize="sm"
            color="fg.muted"
            aria-live="polite"
            css={{ fontVariantNumeric: "tabular-nums" }}
          >
            {secondary}
          </Text>
        )}
      </Box>
      <Progress.Root value={progress} size="sm" colorPalette="brand">
        <Progress.Track>
          <Progress.Range />
        </Progress.Track>
      </Progress.Root>
    </Box>
  );
}
