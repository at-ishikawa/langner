import { Badge } from "@chakra-ui/react";

const LABEL: Record<string, string> = {
  notebook: "Notebook",
  reverse: "Reverse",
  freeform: "Freeform",
  etymology_breakdown: "Etym · Breakdown",
  etymology_assembly: "Etym · Assembly",
  etymology_freeform: "Etym · Freeform",
};

const PALETTE: Record<string, string> = {
  notebook: "blue",
  reverse: "purple",
  freeform: "teal",
  etymology_breakdown: "orange",
  etymology_assembly: "orange",
  etymology_freeform: "orange",
};

export function QuizTypeChip({ quizType }: { quizType: string }) {
  const label = LABEL[quizType] ?? quizType;
  const palette = PALETTE[quizType] ?? "gray";
  return (
    <Badge colorPalette={palette} size="sm" data-testid={`quiz-type-chip-${quizType}`}>
      {label}
    </Badge>
  );
}
