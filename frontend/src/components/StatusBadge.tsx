"use client";

import { Badge } from "@chakra-ui/react";
import type { ReactNode } from "react";

// One source of truth for learning-state pills (replaces the ~4 ad-hoc
// status-color maps). State color is intentionally separate from the brand
// accent; "learning" is the only one that uses brand.
export type LearningStatus = "due" | "learning" | "correct" | "missed" | "excluded";

const MAP: Record<LearningStatus, { palette: string; label: string }> = {
  due: { palette: "orange", label: "Due" },
  learning: { palette: "brand", label: "Learning" },
  correct: { palette: "green", label: "Correct" },
  missed: { palette: "red", label: "Missed" },
  excluded: { palette: "gray", label: "Excluded" },
};

export function StatusBadge({ status, children }: { status: LearningStatus; children?: ReactNode }) {
  const { palette, label } = MAP[status];
  return (
    <Badge
      colorPalette={palette}
      variant="subtle"
      borderRadius="full"
      px={2.5}
      py={1}
      fontFamily="mono"
      fontSize="10px"
      letterSpacing="0.06em"
      textTransform="uppercase"
    >
      {children ?? label}
    </Badge>
  );
}
