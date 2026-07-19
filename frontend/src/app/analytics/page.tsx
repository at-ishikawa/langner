"use client";

import { useEffect, useMemo, useState, useCallback } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Flex,
  Heading,
  HStack,
  NativeSelect,
  SimpleGrid,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { QUIZ_TYPE_OPTIONS } from "@/components/AnalyticsFilterBar";
import { TrendChart, type TrendMetric } from "@/components/TrendChart";
import {
  analyticsClient,
  quizClient,
  Granularity,
  TrendGroupBy,
  type GetTrendsResponse,
} from "@/lib/client";

type RangeKey = "month" | "3m" | "year" | "all";
type SplitKey = "none" | "quiz" | "notebook";

const RANGES: { key: RangeKey; label: string }[] = [
  { key: "month", label: "Month" },
  { key: "3m", label: "3 mo" },
  { key: "year", label: "Year" },
  { key: "all", label: "All" },
];

const GRANULARITIES: { key: Granularity; label: string }[] = [
  { key: Granularity.DAY, label: "Day" },
  { key: Granularity.WEEK, label: "Week" },
  { key: Granularity.MONTH, label: "Month" },
  { key: Granularity.YEAR, label: "Year" },
];

const SPLITS: { key: SplitKey; groupBy: TrendGroupBy; label: string }[] = [
  { key: "none", groupBy: TrendGroupBy.UNSPECIFIED, label: "None" },
  { key: "quiz", groupBy: TrendGroupBy.QUIZ_TYPE, label: "Quiz type" },
  { key: "notebook", groupBy: TrendGroupBy.NOTEBOOK, label: "Notebook" },
];

type SummaryField = "attempts" | "wordsTested" | "wordsLearned" | "levelUps";

const METRICS: { key: TrendMetric; label: string; summaryField: SummaryField }[] = [
  { key: "attempts", label: "Attempts", summaryField: "attempts" },
  { key: "wordsTested", label: "Words tested", summaryField: "wordsTested" },
  { key: "wordsLearned", label: "Words learned", summaryField: "wordsLearned" },
  { key: "levelUps", label: "Level-ups", summaryField: "levelUps" },
];

function ymd(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

function rangeDates(range: RangeKey): { start: string; end: string } {
  const now = new Date();
  const end = ymd(now);
  switch (range) {
    case "month":
      return { start: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), end };
    case "3m":
      return { start: ymd(new Date(now.getFullYear(), now.getMonth() - 3, now.getDate())), end };
    case "year":
      return { start: ymd(new Date(now.getFullYear(), 0, 1)), end };
    default:
      return { start: "", end };
  }
}

function makeFormatPeriod(granularity: Granularity): (period: string) => string {
  return (period: string) => {
    const [y, m, d] = period.split("-").map(Number);
    const date = new Date(y, m - 1, d);
    if (granularity === Granularity.YEAR) {
      return String(y);
    }
    if (granularity === Granularity.MONTH) {
      return date.toLocaleDateString("en-US", { month: "short" });
    }
    if (granularity === Granularity.WEEK) {
      return date.toLocaleDateString("en-US", { month: "short", day: "numeric" });
    }
    return `${m}/${d}`;
  };
}

function historyQuery(notebookId: string, quizType: string): string {
  const sp = new URLSearchParams();
  sp.set("range", "0"); // all time, so the drilled-in period is present in the list
  if (notebookId) sp.set("notebook", notebookId);
  if (quizType) sp.set("quiz", quizType);
  return sp.toString();
}

function Segmented<T extends string | number>({
  label,
  options,
  value,
  onChange,
  disabled,
}: {
  label: string;
  options: { key: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
  disabled?: boolean;
}) {
  return (
    <HStack gap={2} align="center" flexShrink={0}>
      <Text fontSize="xs" color="fg.muted" textTransform="uppercase" letterSpacing="wider">
        {label}
      </Text>
      <HStack gap={1} bg="bg.muted" p={1} borderRadius="md" flexWrap="wrap">
        {options.map((o) => (
          <Button
            key={String(o.key)}
            size="xs"
            variant={value === o.key ? "solid" : "ghost"}
            disabled={disabled}
            onClick={() => onChange(o.key)}
          >
            {o.label}
          </Button>
        ))}
      </HStack>
    </HStack>
  );
}

export default function AnalyticsOverviewPage() {
  const router = useRouter();
  const [range, setRange] = useState<RangeKey>("year");
  const [granularity, setGranularity] = useState<Granularity>(Granularity.MONTH);
  const [split, setSplit] = useState<SplitKey>("quiz");
  const [metric, setMetric] = useState<TrendMetric>("wordsTested");
  const [notebookId, setNotebookId] = useState("");
  const [quizType, setQuizType] = useState("");
  const [notebooks, setNotebooks] = useState<{ id: string; name: string }[]>([]);

  const [data, setData] = useState<GetTrendsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Level-ups are only meaningful split by their target box, so that metric
  // forces the LEVEL grouping regardless of the split control.
  const groupBy = metric === "levelUps"
    ? TrendGroupBy.LEVEL
    : (SPLITS.find((s) => s.key === split)?.groupBy ?? TrendGroupBy.UNSPECIFIED);

  const { start, end } = useMemo(() => rangeDates(range), [range]);

  useEffect(() => {
    quizClient
      .getQuizOptions({ includeUnstudied: true })
      .then((res) => setNotebooks(res.notebooks.map((n) => ({ id: n.notebookId, name: n.name }))))
      .catch(() => setNotebooks([]));
  }, []);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    analyticsClient
      .getTrends({
        granularity,
        groupBy,
        startDate: start,
        endDate: end,
        filters: { notebookId, quizType },
      })
      .then((res) => {
        if (!cancelled) {
          setData(res);
          setError(null);
        }
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [granularity, groupBy, start, end, notebookId, quizType]);

  // Clicking a bar drills into the History detail: a day bucket opens that
  // exact day; a coarser bucket opens the history list filtered the same way.
  const onBarClick = useCallback(
    (period: string) => {
      if (granularity === Granularity.DAY) {
        const sp = new URLSearchParams();
        if (notebookId) sp.set("notebook", notebookId);
        if (quizType) sp.set("quiz", quizType);
        const q = sp.toString();
        router.push(`/history/${period}${q ? `?${q}` : ""}`);
        return;
      }
      router.push(`/history?${historyQuery(notebookId, quizType)}`);
    },
    [granularity, notebookId, quizType, router],
  );

  const formatPeriod = useMemo(() => makeFormatPeriod(granularity), [granularity]);
  const metricLabel = METRICS.find((m) => m.key === metric)?.label ?? "";
  const hasBuckets = data !== null && data.buckets.length > 0;

  // When splitting by notebook, prefer the notebook's display name over its
  // id for the legend/tooltip.
  const resolveLabel = useMemo(() => {
    if (groupBy !== TrendGroupBy.NOTEBOOK) return undefined;
    const names = new Map(notebooks.map((n) => [n.id, n.name]));
    return (key: string, fallback: string) => names.get(key) || fallback || key;
  }, [groupBy, notebooks]);

  return (
    <Box maxW="4xl" mx="auto" p={4} pb={20}>
      <Link href="/" aria-label="Back to home">
        <Text fontSize="sm" mb={2}>◀ Home</Text>
      </Link>
      <Heading size="lg" mb={3}>Analytics</Heading>

      {/* Controls sit directly under the title so the totals below update
          visibly when a filter changes. */}
      <VStack align="stretch" gap={3} mb={4} pb={4} borderBottomWidth="1px" borderColor="border">
        <Flex gap={4} wrap="wrap">
          <Segmented label="Range" options={RANGES} value={range} onChange={setRange} />
          <Segmented label="Bucket" options={GRANULARITIES} value={granularity} onChange={setGranularity} />
        </Flex>
        <Flex gap={4} wrap="wrap" align="center">
          <Segmented
            label="Split"
            options={SPLITS}
            value={split}
            onChange={setSplit}
            disabled={metric === "levelUps"}
          />
          {metric === "levelUps" && (
            <Text fontSize="xs" color="fg.muted">Level-ups are split by review box.</Text>
          )}
        </Flex>
        <Flex gap={3} wrap="wrap">
          <HStack flex="1" minW="200px">
            <Text fontSize="sm" minW="16">Notebook</Text>
            <NativeSelect.Root size="sm">
              <NativeSelect.Field
                aria-label="Filter by notebook"
                value={notebookId}
                onChange={(e) => setNotebookId(e.target.value)}
              >
                <option value="">All notebooks</option>
                {notebooks.map((n) => (
                  <option key={n.id} value={n.id}>{n.name || n.id}</option>
                ))}
              </NativeSelect.Field>
              <NativeSelect.Indicator />
            </NativeSelect.Root>
          </HStack>
          <HStack flex="1" minW="200px">
            <Text fontSize="sm" minW="10">Quiz</Text>
            <NativeSelect.Root size="sm">
              <NativeSelect.Field
                aria-label="Filter by quiz type"
                value={quizType}
                onChange={(e) => setQuizType(e.target.value)}
              >
                {QUIZ_TYPE_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </NativeSelect.Field>
              <NativeSelect.Indicator />
            </NativeSelect.Root>
          </HStack>
        </Flex>
      </VStack>

      {/* KPI tiles double as the chart's metric selector */}
      <SimpleGrid columns={{ base: 2, md: 4 }} gap={3} mb={4} data-testid="trend-kpis">
        {METRICS.map((m) => {
          const active = metric === m.key;
          const value = data?.summary?.[m.summaryField];
          return (
            <Box
              key={m.key}
              as="button"
              textAlign="left"
              onClick={() => setMetric(m.key)}
              borderWidth="1px"
              borderColor={active ? "fg" : "border"}
              bg={active ? "bg.subtle" : "bg"}
              borderRadius="lg"
              p={3}
              aria-pressed={active}
            >
              <Text fontSize="xs" color="fg.muted">{m.label}</Text>
              <Text fontSize="2xl" fontWeight="bold" lineHeight="1.1">
                {value === undefined ? "—" : String(value)}
              </Text>
            </Box>
          );
        })}
      </SimpleGrid>

      {/* Backlog — point-in-time state, not flow */}
      {data && (
        <SimpleGrid columns={{ base: 3 }} gap={3} mb={5} data-testid="trend-backlog">
          {[
            { label: "Never correct", value: data.backlog?.neverCorrect ?? 0, color: "orange.500" },
            { label: "In progress", value: data.backlog?.inProgress ?? 0, color: "blue.500" },
            { label: "Usable+", value: data.backlog?.mastered ?? 0, color: "green.500" },
          ].map((s) => (
            <HStack key={s.label} borderWidth="1px" borderColor="border" borderRadius="md" p={3} gap={3}>
              <Box w="3px" alignSelf="stretch" borderRadius="full" bg={s.color} />
              <Box>
                <Text fontSize="xl" fontWeight="bold" lineHeight="1.1">{String(s.value)}</Text>
                <Text fontSize="xs" color="fg.muted">{s.label}</Text>
              </Box>
            </HStack>
          ))}
        </SimpleGrid>
      )}

      {/* Chart */}
      {error && (
        <Text color="red.500" data-testid="trends-error">Failed to load trends: {error}</Text>
      )}
      {!error && data === null && <Spinner data-testid="trends-loading" />}
      {!error && data !== null && !hasBuckets && (
        <Box textAlign="center" mt={10} data-testid="trends-empty">
          <Text fontSize="lg" fontWeight="semibold" mb={2}>No quiz activity in this range.</Text>
          <Text color="fg.muted">Try widening the range or removing filters.</Text>
        </Box>
      )}
      {!error && hasBuckets && (
        <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={4}>
          <Flex justify="space-between" align="baseline" mb={2}>
            <Text fontSize="sm" fontWeight="semibold">{metricLabel} over time</Text>
            <Text fontSize="xs" color="fg.muted">Click a bar for that period’s days</Text>
          </Flex>
          <TrendChart
            buckets={data!.buckets}
            metric={metric}
            metricLabel={metricLabel}
            formatPeriod={formatPeriod}
            onBarClick={onBarClick}
            resolveLabel={resolveLabel}
          />
        </Box>
      )}
    </Box>
  );
}
