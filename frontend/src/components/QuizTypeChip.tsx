import { Badge } from "@chakra-ui/react";

const LABEL: Record<string, string> = {
  notebook: "What does it mean?",
  reverse: "Which word is it?",
  freeform: "Free practice",
  etymology_breakdown: "Word roots",
  etymology_assembly: "Word roots",
  etymology_freeform: "Word roots",
  grammar: "Grammar",
};

const PALETTE: Record<string, string> = {
  notebook: "blue",
  reverse: "purple",
  freeform: "teal",
  etymology_breakdown: "orange",
  etymology_assembly: "orange",
  etymology_freeform: "orange",
  grammar: "pink",
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
