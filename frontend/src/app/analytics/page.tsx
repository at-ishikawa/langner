"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { Box, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import {
  AnalyticsFilterBar,
  type AnalyticsFilterState,
  RANGE_LABELS,
} from "@/components/AnalyticsFilterBar";
import { DayCard } from "@/components/DayCard";
import { analyticsClient, type DailySummary } from "@/lib/client";

const DEFAULT_RANGE: AnalyticsFilterState["range"] = "30";

function parseFilters(params: URLSearchParams): AnalyticsFilterState {
  const range = (params.get("range") ?? DEFAULT_RANGE) as AnalyticsFilterState["range"];
  return {
    range: range in RANGE_LABELS ? range : DEFAULT_RANGE,
    notebookId: params.get("notebook") ?? "",
    quizType: params.get("quiz") ?? "",
  };
}

function buildQuery(state: AnalyticsFilterState): string {
  const sp = new URLSearchParams();
  if (state.range !== DEFAULT_RANGE) sp.set("range", state.range);
  if (state.notebookId) sp.set("notebook", state.notebookId);
  if (state.quizType) sp.set("quiz", state.quizType);
  return sp.toString();
}

export default function AnalyticsPage() {
  const router = useRouter();
  const params = useSearchParams();
  const [state, setState] = useState<AnalyticsFilterState>(() => parseFilters(params));
  const [days, setDays] = useState<DailySummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setState(parseFilters(params));
  }, [params]);

  useEffect(() => {
    let cancelled = false;
    setDays(null);
    analyticsClient
      .getDailySummaries({
        rangeDays: Number(state.range),
        filters: { notebookId: state.notebookId, quizType: state.quizType },
      })
      .then((res) => {
        if (!cancelled) setDays(res.days);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [state.range, state.notebookId, state.quizType]);

  const onFilterChange = useCallback(
    (next: AnalyticsFilterState) => {
      setState(next);
      const q = buildQuery(next);
      router.replace(`/analytics${q ? `?${q}` : ""}`);
    },
    [router],
  );

  const queryString = buildQuery(state);
  const notebookOptions = Array.from(
    new Set((days ?? []).flatMap(() => [])), // Day list doesn't expose notebooks; selector populated from notebooks endpoint in future iterations.
  );

  return (
    <Box maxW="md" mx="auto" p={4} pb={20}>
      <Link href="/" aria-label="Back to home">
        <Text fontSize="sm" mb={2}>◀ Home</Text>
      </Link>
      <Heading size="lg" mb={4}>
        Analytics
      </Heading>

      <AnalyticsFilterBar
        state={state}
        notebookOptions={notebookOptions}
        onChange={onFilterChange}
      />

      {error && (
        <Text color="red.500" data-testid="analytics-error">
          Failed to load analytics: {error}
        </Text>
      )}

      {!error && days === null && <Spinner data-testid="analytics-loading" />}

      {!error && days !== null && days.length === 0 && (
        <Box textAlign="center" mt={10} data-testid="analytics-empty">
          <Text fontSize="lg" fontWeight="semibold" mb={2}>
            No quiz activity in this range.
          </Text>
          <Text color="fg.muted">Try widening the time range or removing filters.</Text>
        </Box>
      )}

      {!error && days !== null && days.length > 0 && (
        <VStack align="stretch" gap={3} data-testid="day-list">
          {days.map((d) => (
            <DayCard key={d.date} day={d} queryString={queryString} />
          ))}
        </VStack>
      )}
    </Box>
  );
}
