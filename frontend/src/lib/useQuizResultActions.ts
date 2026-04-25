import { useCallback } from "react";
import { useQuizStore, type QuizType } from "@/store/quizStore";
import { quizClient, QuizType as ProtoQuizType } from "@/lib/client";
import type { ResultItem } from "@/components/QuizResultCard";

function toProtoQuizType(qt: QuizType): ProtoQuizType {
  if (qt === "reverse") return ProtoQuizType.REVERSE;
  if (qt === "freeform") return ProtoQuizType.FREEFORM;
  if (qt === "etymology-standard") return ProtoQuizType.ETYMOLOGY_STANDARD;
  if (qt === "etymology-reverse") return ProtoQuizType.ETYMOLOGY_REVERSE;
  if (qt === "etymology-freeform") return ProtoQuizType.ETYMOLOGY_FREEFORM;
  return ProtoQuizType.STANDARD;
}

export interface QuizResultActions {
  handleOverride: (item: ResultItem) => Promise<void>;
  handleUndo: (item: ResultItem) => Promise<void>;
  handleSkip: (item: ResultItem) => Promise<void>;
  handleResume: (item: ResultItem) => Promise<void>;
}

export function useQuizResultActions(quizType: QuizType): QuizResultActions {
  const results = useQuizStore((s) => s.results);
  const reverseResults = useQuizStore((s) => s.reverseResults);
  const freeformResults = useQuizStore((s) => s.freeformResults);
  const etymologyResults = useQuizStore((s) => s.etymologyOriginResults);
  const overrideResult = useQuizStore((s) => s.overrideResult);
  const undoOverrideResult = useQuizStore((s) => s.undoOverrideResult);
  const skipResult = useQuizStore((s) => s.skipResult);
  const resumeResult = useQuizStore((s) => s.resumeResult);

  const protoQt = toProtoQuizType(quizType);

  const handleOverride = useCallback(async (item: ResultItem) => {
    if (!item.noteId || !item.learnedAt) return;
    try {
      const res = await quizClient.overrideAnswer({
        noteId: item.noteId,
        quizType: protoQt,
        learnedAt: item.learnedAt,
        markCorrect: !item.correct,
      });
      overrideResult(item.index, quizType, res.nextReviewDate || "", {
        quality: res.originalQuality,
        status: res.originalStatus,
        intervalDays: res.originalIntervalDays,
      });
    } catch { /* silently fail */ }
  }, [protoQt, quizType, overrideResult]);

  const handleUndo = useCallback(async (item: ResultItem) => {
    if (!item.noteId || !item.learnedAt) return;
    const storeResults =
      quizType === "standard" ? results
      : quizType === "reverse" ? reverseResults
      : quizType === "freeform" ? freeformResults
      : etymologyResults;
    const original = storeResults[item.index];
    if (!original?.originalValues) return;
    try {
      const res = await quizClient.undoOverrideAnswer({
        noteId: item.noteId,
        quizType: protoQt,
        learnedAt: item.learnedAt,
        originalQuality: original.originalValues.quality,
        originalStatus: original.originalValues.status,
        originalIntervalDays: original.originalValues.intervalDays,
      });
      undoOverrideResult(item.index, quizType, res.correct, res.nextReviewDate || "");
    } catch { /* silently fail */ }
  }, [protoQt, quizType, results, reverseResults, freeformResults, etymologyResults, undoOverrideResult]);

  const handleSkip = useCallback(async (item: ResultItem) => {
    if (!item.noteId) return;
    try {
      await quizClient.skipWord({ noteId: item.noteId });
      skipResult(item.index, quizType);
    } catch { /* silently fail */ }
  }, [quizType, skipResult]);

  const handleResume = useCallback(async (item: ResultItem) => {
    if (!item.noteId) return;
    try {
      await quizClient.resumeWord({ noteId: item.noteId });
      resumeResult(item.index, quizType);
    } catch { /* silently fail */ }
  }, [quizType, resumeResult]);

  return { handleOverride, handleUndo, handleSkip, handleResume };
}
