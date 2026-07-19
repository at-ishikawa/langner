"use client";

import { useState } from "react";
import { Box, Flex, HStack, Text } from "@chakra-ui/react";
import type { TrendBucket, TrendSeries } from "@/lib/client";

export type TrendMetric = "attempts" | "wordsTested" | "wordsLearned" | "levelUps";

// Colorblind-safe categorical slots (validated set from the design), used
// in the order series appear. A 9th series folds back to the start — the
// backend caps the meaningful dimensions well below that.
const SERIES_COLORS = [
  "#2a78d6",
  "#1baf7a",
  "#eb6834",
  "#4a3aa7",
  "#eda100",
  "#e34948",
  "#e87ba4",
  "#008300",
];

const W = 820;
const H = 300;
// PADL leaves room for up to 5-digit y-axis labels (e.g. "12000").
const PADL = 58;
const PADR = 12;
const PADT = 14;
const PADB = 30;
const PLOT_W = W - PADL - PADR;
const PLOT_H = H - PADT - PADB;

function metricValue(s: TrendSeries, metric: TrendMetric): number {
  switch (metric) {
    case "attempts":
      return s.attempts;
    case "wordsTested":
      return s.wordsTested;
    case "wordsLearned":
      return s.wordsLearned;
    case "levelUps":
      return s.levelUps;
  }
}

function niceMax(v: number): number {
  if (v <= 5) return 5;
  const pow = Math.pow(10, String(Math.round(v)).length - 1);
  return Math.ceil(v / pow) * pow;
}

type Hover = { x: number; y: number; label: string; period: string; value: number; total: number };

export function TrendChart({
  buckets,
  metric,
  metricLabel,
  formatPeriod,
  onBarClick,
  resolveLabel,
}: {
  buckets: TrendBucket[];
  metric: TrendMetric;
  metricLabel: string;
  formatPeriod: (period: string) => string;
  onBarClick?: (period: string) => void;
  resolveLabel?: (groupKey: string, groupLabel: string) => string;
}) {
  const [hover, setHover] = useState<Hover | null>(null);

  // Ordered unique series keys, preserving the order the backend emits.
  const seen = new Map<string, string>();
  for (const b of buckets) {
    for (const s of b.series) {
      if (!seen.has(s.groupKey)) seen.set(s.groupKey, s.groupLabel);
    }
  }
  const keys = Array.from(seen.entries()).map(([key, label], i) => ({
    key,
    label: resolveLabel ? resolveLabel(key, label) : label || metricLabel,
    color: SERIES_COLORS[i % SERIES_COLORS.length],
  }));
  const colorOf = new Map(keys.map((k) => [k.key, k.color]));
  const labelOf = new Map(keys.map((k) => [k.key, k.label]));
  const singleSeries = keys.length === 1 && keys[0].key === "";

  const totals = buckets.map((b) => b.series.reduce((sum, s) => sum + metricValue(s, metric), 0));
  const yMax = niceMax(Math.max(1, ...totals));

  const bandW = PLOT_W / Math.max(1, buckets.length);
  const barW = Math.min(46, bandW * 0.7);
  const labelStep = Math.ceil(buckets.length / 16); // avoid crowding x labels
  const ticks = 4;

  return (
    <Box position="relative" color="fg.muted" data-testid="trend-chart">
      <svg width="100%" style={{ display: "block" }} viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${metricLabel} over time`}>
        {/* gridlines + y labels */}
        {Array.from({ length: ticks + 1 }, (_, i) => {
          const yv = (yMax / ticks) * i;
          const y = PADT + PLOT_H - (yv / yMax) * PLOT_H;
          return (
            <g key={i}>
              <line x1={PADL} x2={W - PADR} y1={y} y2={y} stroke="currentColor" strokeOpacity={0.15} strokeWidth={1} />
              <text x={PADL - 6} y={y + 3} textAnchor="end" fontSize={10} fill="currentColor" fillOpacity={0.7}>
                {Math.round(yv)}
              </text>
            </g>
          );
        })}

        {/* bars */}
        {buckets.map((b, i) => {
          const x = PADL + bandW * i + (bandW - barW) / 2;
          let yCursor = PADT + PLOT_H;
          const total = totals[i];
          const rects = keys.map(({ key }) => {
            const s = b.series.find((se) => se.groupKey === key);
            const v = s ? metricValue(s, metric) : 0;
            if (v <= 0) return null;
            const h = (v / yMax) * PLOT_H;
            const top = yCursor - h;
            yCursor = top;
            const rect = (
              <rect
                key={key}
                x={x}
                y={top}
                width={barW}
                height={Math.max(1, h - 2)}
                rx={2}
                fill={colorOf.get(key)}
                style={{ cursor: onBarClick ? "pointer" : "default" }}
                onClick={() => onBarClick?.(b.period)}
                onMouseMove={(e) => {
                  const box = (e.currentTarget.ownerSVGElement!.parentElement as HTMLElement).getBoundingClientRect();
                  setHover({
                    x: e.clientX - box.left,
                    y: e.clientY - box.top,
                    label: labelOf.get(key) || metricLabel,
                    period: formatPeriod(b.period),
                    value: v,
                    total,
                  });
                }}
                onMouseLeave={() => setHover(null)}
              />
            );
            return rect;
          });
          return (
            <g key={b.period}>
              {rects}
              {i % labelStep === 0 && (
                <text x={x + barW / 2} y={H - 12} textAnchor="middle" fontSize={10} fill="currentColor" fillOpacity={0.8}>
                  {formatPeriod(b.period)}
                </text>
              )}
            </g>
          );
        })}
      </svg>

      {hover && (
        <Box
          position="absolute"
          left={`${hover.x}px`}
          top={`${hover.y}px`}
          transform="translate(-50%, -115%)"
          pointerEvents="none"
          bg="bg.inverted"
          color="fg.inverted"
          fontSize="xs"
          px={2.5}
          py={1.5}
          borderRadius="md"
          boxShadow="md"
          whiteSpace="nowrap"
          zIndex={2}
        >
          <Text fontWeight="bold">{hover.label} · {hover.period}</Text>
          <Flex justify="space-between" gap={4}>
            <span>{metricLabel}</span>
            <span>{hover.value}</span>
          </Flex>
          {!singleSeries && (
            <Flex justify="space-between" gap={4} opacity={0.7}>
              <span>Period total</span>
              <span>{hover.total}</span>
            </Flex>
          )}
        </Box>
      )}

      {!singleSeries && (
        <HStack gap={4} mt={2} flexWrap="wrap" data-testid="trend-legend">
          {keys.map((k) => (
            <HStack key={k.key} gap={1.5}>
              <Box w="10px" h="10px" borderRadius="3px" bg={k.color} />
              <Text fontSize="xs" color="fg.muted">{k.label}</Text>
            </HStack>
          ))}
        </HStack>
      )}
    </Box>
  );
}
