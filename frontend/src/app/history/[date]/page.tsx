"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import { Box, HStack, Heading, Spinner, Text, VStack } from "@chakra-ui/react";
import {
  AnalyticsFilterBar,
  type AnalyticsFilterState,
} from "@/components/AnalyticsFilterBar";
import { QuizTypeChip } from "@/components/QuizTypeChip";
import { WrongWordCard } from "@/components/WrongWordCard";
import { analyticsClient, type GetDayDetailResponse } from "@/lib/client";

function formatDayHeader(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  if (!y || !m || !d) return date;
  const dt = new Date(y, m - 1, d);
  return dt.toLocaleDateString("en-US", {
    weekday: "long",
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function parseFilters(params: URLSearchParams): AnalyticsFilterState {
  return {
    range: "30",
    notebookId: params.get("notebook") ?? "",
    quizType: params.get("quiz") ?? "",
  };
}

function buildBackQuery(state: AnalyticsFilterState): string {
  const sp = new URLSearchParams();
  if (state.notebookId) sp.set("notebook", state.notebookId);
  if (state.quizType) sp.set("quiz", state.quizType);
  return sp.toString();
}

export default function DayDetailPage() {
  const params = useParams<{ date: string }>();
  const search = useSearchParams();
  const date = params.date;
  const [state, setState] = useState<AnalyticsFilterState>(() => parseFilters(search));
  const [detail, setDetail] = useState<GetDayDetailResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setState(parseFilters(search));
  }, [search]);

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    analyticsClient
      .getDayDetail({
        date,
        filters: { notebookId: state.notebookId, quizType: state.quizType },
      })
      .then((res) => {
        if (!cancelled) setDetail(res);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [date, state.notebookId, state.quizType]);

  const backQuery = buildBackQuery(state);

  function dayLink(target: string): string {
    return `/history/${target}${backQuery ? `?${backQuery}` : ""}`;
  }

  return (
    <Box maxW="md" mx="auto" p={4} pb={20}>
      <Link
        href={`/history${backQuery ? `?${backQuery}` : ""}`}
        aria-label="Back to history"
      >
        <Text fontSize="sm" mb={2}>◀ History</Text>
      </Link>
      <Heading size="md" mb={1}>
        {formatDayHeader(date)}
      </Heading>

      <HStack mb={4} gap={3}>
        {detail?.previousDate && (
          <Link href={dayLink(detail.previousDate)} aria-label="Previous day with activity">
            <Text fontSize="sm">◀ {detail.previousDate}</Text>
          </Link>
        )}
        {detail?.nextDate && (
          <Link href={dayLink(detail.nextDate)} aria-label="Next day with activity">
            <Text fontSize="sm">{detail.nextDate} ▶</Text>
          </Link>
        )}
      </HStack>

      <AnalyticsFilterBar
        state={state}
        notebookOptions={[]}
        onChange={setState}
        hideRange
      />

      {error && (
        <Text color="red.500" data-testid="day-detail-error">
          Failed to load day: {error}
        </Text>
      )}

      {!error && detail === null && <Spinner data-testid="day-detail-loading" />}

      {!error && detail && (
        <>
          <Box mb={4} data-testid="day-summary">
            <Text fontSize="sm" color="fg.muted">
              {detail.summary?.wrongCount ?? 0} wrong / {detail.summary?.totalCount ?? 0}
              {" "}
              attempted · {detail.summary?.notebookCount ?? 0} notebook
              {(detail.summary?.notebookCount ?? 0) === 1 ? "" : "s"}
            </Text>
            <HStack gap={1} mt={2} flexWrap="wrap">
              {detail.summary?.quizTypes.map((qt) => (
                <QuizTypeChip key={qt} quizType={qt} />
              ))}
            </HStack>
          </Box>

          {detail.wrongWords.length === 0 ? (
            <Box textAlign="center" mt={6} data-testid="all-correct">
              <Text fontSize="lg" fontWeight="semibold" mb={2}>
                All correct on this day.
              </Text>
              <Text color="fg.muted">{detail.summary?.totalCount ?? 0} attempted, 0 wrong.</Text>
            </Box>
          ) : (
            <VStack align="stretch" gap={0} data-testid="wrong-word-list">
              {detail.wrongWords.map((w, i) => (
                <WrongWordCard key={`${w.expression}-${w.quizType}-${i}`} word={w} />
              ))}
            </VStack>
          )}
        </>
      )}
    </Box>
  );
}
