"use client";

import Link from "next/link";
import { Box, HStack, Text } from "@chakra-ui/react";
import { QuizTypeChip } from "./QuizTypeChip";
import type { DailySummary } from "@/lib/client";

function formatDate(dateStr: string): string {
  // Parse YYYY-MM-DD as local; produce e.g. "Fri, Jun 5".
  const [y, m, d] = dateStr.split("-").map(Number);
  if (!y || !m || !d) return dateStr;
  const dt = new Date(y, m - 1, d);
  return dt.toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" });
}

function ratePercent(wrong: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((wrong / total) * 100);
}

export function DayCard({ day, queryString }: { day: DailySummary; queryString: string }) {
  const muted = day.wrongCount === 0;
  const href = `/history/${day.date}${queryString ? `?${queryString}` : ""}`;
  return (
    <Link href={href} aria-label={`View ${formatDate(day.date)} detail`}>
      <Box
        p={4}
        borderWidth="1px"
        borderRadius="lg"
        bg="white"
        _dark={{ bg: "gray.800" }}
        _hover={{ borderColor: "blue.400" }}
        opacity={muted ? 0.65 : 1}
        data-testid={`day-card-${day.date}`}
      >
        <HStack justifyContent="space-between" mb={1}>
          <Text fontWeight="semibold">{formatDate(day.date)}</Text>
          <Text fontSize="sm" color="fg.muted">
            {day.wrongCount} wrong / {day.totalCount} ({ratePercent(day.wrongCount, day.totalCount)}%)
          </Text>
        </HStack>
        <Text fontSize="xs" color="fg.muted" mb={2}>
          {day.notebookCount} notebook{day.notebookCount === 1 ? "" : "s"}
        </Text>
        <HStack gap={1} flexWrap="wrap">
          {day.quizTypes.slice(0, 3).map((qt) => (
            <QuizTypeChip key={qt} quizType={qt} />
          ))}
          {day.quizTypes.length > 3 && (
            <Text fontSize="xs" color="fg.muted">
              +{day.quizTypes.length - 3} more
            </Text>
          )}
        </HStack>
      </Box>
    </Link>
  );
}
