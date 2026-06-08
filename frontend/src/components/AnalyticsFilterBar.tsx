"use client";

import { Box, HStack, NativeSelect, Text } from "@chakra-ui/react";

export type AnalyticsFilterState = {
  range: "7" | "30" | "90" | "0";
  notebookId: string;
  quizType: string;
};

export const RANGE_LABELS: Record<AnalyticsFilterState["range"], string> = {
  "7": "Last 7 days",
  "30": "Last 30 days",
  "90": "Last 90 days",
  "0": "All time",
};

export const QUIZ_TYPE_OPTIONS: readonly { value: string; label: string }[] = [
  { value: "", label: "All quizzes" },
  { value: "notebook", label: "— Vocabulary · Notebook" },
  { value: "reverse", label: "— Vocabulary · Reverse" },
  { value: "freeform", label: "— Vocabulary · Freeform" },
  { value: "etymology_breakdown", label: "— Etymology · Breakdown" },
  { value: "etymology_assembly", label: "— Etymology · Assembly" },
  { value: "etymology_freeform", label: "— Etymology · Freeform" },
];

export function AnalyticsFilterBar({
  state,
  notebookOptions,
  onChange,
  hideRange,
}: {
  state: AnalyticsFilterState;
  notebookOptions: readonly string[];
  onChange: (next: AnalyticsFilterState) => void;
  hideRange?: boolean;
}) {
  return (
    <Box
      display="flex"
      flexDirection={{ base: "column", md: "row" }}
      gap={3}
      mb={4}
      data-testid="analytics-filter-bar"
    >
      <HStack flex="1">
        <Text fontSize="sm" minW="20">Notebook</Text>
        <NativeSelect.Root size="sm">
          <NativeSelect.Field
            aria-label="Filter by notebook"
            value={state.notebookId}
            onChange={(e) => onChange({ ...state, notebookId: e.target.value })}
          >
            <option value="">All notebooks</option>
            {notebookOptions.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </NativeSelect.Field>
          <NativeSelect.Indicator />
        </NativeSelect.Root>
      </HStack>

      {!hideRange && (
        <HStack flex="1">
          <Text fontSize="sm" minW="20">Range</Text>
          <NativeSelect.Root size="sm">
            <NativeSelect.Field
              aria-label="Filter by time range"
              value={state.range}
              onChange={(e) =>
                onChange({ ...state, range: e.target.value as AnalyticsFilterState["range"] })
              }
            >
              {Object.entries(RANGE_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </NativeSelect.Field>
            <NativeSelect.Indicator />
          </NativeSelect.Root>
        </HStack>
      )}

      <HStack flex="1">
        <Text fontSize="sm" minW="20">Quiz</Text>
        <NativeSelect.Root size="sm">
          <NativeSelect.Field
            aria-label="Filter by quiz type"
            value={state.quizType}
            onChange={(e) => onChange({ ...state, quizType: e.target.value })}
          >
            {QUIZ_TYPE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </NativeSelect.Field>
          <NativeSelect.Indicator />
        </NativeSelect.Root>
      </HStack>
    </Box>
  );
}
