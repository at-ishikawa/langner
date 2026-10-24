"use client";

import type { ReactNode } from "react";
import type { BoxProps } from "@chakra-ui/react";
import { PageShell } from "./PageShell";
import { QuizProgress } from "./QuizProgress";

// QuizScaffold is the shared chrome for a quiz session: one centered column
// (PageShell, maxW="xl") with the QuizProgress header on top, then the mode's
// own prompt / input / feedback as children. Using it everywhere makes the
// container, spacing, and progress header identical across quiz modes.
export function QuizScaffold({
  label,
  secondary,
  progress,
  pb,
  children,
}: {
  label: ReactNode;
  secondary?: ReactNode;
  progress: number;
  pb?: BoxProps["pb"];
  children: ReactNode;
}) {
  return (
    <PageShell maxW="xl" pb={pb}>
      <QuizProgress label={label} secondary={secondary} progress={progress} />
      {children}
    </PageShell>
  );
}
