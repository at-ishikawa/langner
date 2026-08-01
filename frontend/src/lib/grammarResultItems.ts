import type { GrammarResultState } from "@/store/grammarStore";
import type { ResultItem } from "@/components/QuizResultCard";

// grammarResultToItem adapts a graded grammar blank to the shared ResultItem
// shape so the grammar quiz reuses QuizResultCard / QuizResultsGroupedList.
// entry = the wrong span, meaning = the correction; noteId + senseId + learnedAt
// are carried so the Mark-correct / Exclude footer targets the same entry via
// the existing override/skip RPCs.
export function grammarResultToItem(r: GrammarResultState, index: number): ResultItem {
  return {
    index,
    key: r.noteId.toString(),
    entry: r.incorrect,
    meaning: r.correctAnswer,
    correct: r.correct,
    noteId: r.noteId,
    senseId: r.senseId,
    learnedAt: r.learnedAt,
    isOverridden: r.isOverridden,
    isSkipped: r.isSkipped,
    originalCorrect: r.isOverridden ? !r.correct : r.correct,
    userAnswer: r.answer,
    reason: r.reason,
  };
}
