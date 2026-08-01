import { useCallback } from "react";
import { useGrammarStore } from "@/store/grammarStore";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import type { ResultItem } from "@/components/QuizResultCard";
import type { QuizResultActions } from "@/lib/useQuizResultActions";

// useGrammarResultActions mirrors useQuizResultActions but targets the grammar
// store and passes QuizType.GRAMMAR, so Mark-correct / Undo / Exclude / Resume
// reuse the existing override/skip RPCs — no grammar-only backend path. The
// shared vocab hook can't be reused directly: its local QuizType union has no
// "grammar" member.
export function useGrammarResultActions(): QuizResultActions {
  const results = useGrammarStore((s) => s.results);
  const overrideResult = useGrammarStore((s) => s.overrideResult);
  const undoOverrideResult = useGrammarStore((s) => s.undoOverrideResult);
  const skipResult = useGrammarStore((s) => s.skipResult);
  const resumeResult = useGrammarStore((s) => s.resumeResult);

  const handleOverride = useCallback(
    async (item: ResultItem) => {
      if (!item.noteId || !item.learnedAt) return;
      try {
        const res = await quizClient.overrideAnswer({
          noteId: item.noteId,
          senseId: item.senseId,
          quizType: ProtoQuizType.GRAMMAR,
          learnedAt: item.learnedAt,
          markCorrect: !item.correct,
        });
        overrideResult(item.index, res.nextReviewDate || "", {
          quality: res.originalQuality,
          status: res.originalStatus,
          intervalDays: res.originalIntervalDays,
        });
      } catch {
        /* silently fail */
      }
    },
    [overrideResult],
  );

  const handleUndo = useCallback(
    async (item: ResultItem) => {
      if (!item.noteId || !item.learnedAt) return;
      const original = results[item.index];
      if (!original?.originalValues) return;
      try {
        const res = await quizClient.undoOverrideAnswer({
          noteId: item.noteId,
          senseId: item.senseId,
          quizType: ProtoQuizType.GRAMMAR,
          learnedAt: item.learnedAt,
          originalQuality: original.originalValues.quality,
          originalStatus: original.originalValues.status,
          originalIntervalDays: original.originalValues.intervalDays,
        });
        undoOverrideResult(item.index, res.correct, res.nextReviewDate || "");
      } catch {
        /* silently fail */
      }
    },
    [results, undoOverrideResult],
  );

  const handleSkip = useCallback(
    async (item: ResultItem) => {
      if (!item.noteId) return;
      try {
        await quizClient.skipWord({ noteId: item.noteId, quizTypes: [ProtoQuizType.GRAMMAR] });
        skipResult(item.index);
      } catch {
        /* silently fail */
      }
    },
    [skipResult],
  );

  const handleResume = useCallback(
    async (item: ResultItem) => {
      if (!item.noteId) return;
      try {
        await quizClient.resumeWord({ noteId: item.noteId, quizTypes: [ProtoQuizType.GRAMMAR] });
        resumeResult(item.index);
      } catch {
        /* silently fail */
      }
    },
    [resumeResult],
  );

  return { handleOverride, handleUndo, handleSkip, handleResume };
}
