import type {
  QuizResult,
  ReverseQuizResult,
  FreeformResult,
  EtymologyOriginResult,
  WordDetail,
} from "@/store/quizStore";
import type { OriginPartDisplay, ResultItem } from "@/components/QuizResultCard";

function buildOriginBreakdown(detail?: WordDetail): OriginPartDisplay[] | undefined {
  return detail?.originParts?.map((p) => ({
    origin: p.origin,
    meaning: p.meaning,
    language: p.language,
    type: p.type,
  }));
}

export function standardResultToItem(r: QuizResult, index: number): ResultItem {
  return {
    index,
    key: r.noteId.toString(),
    entry: r.entry,
    meaning: r.meaning,
    correct: r.correct,
    contexts: r.contexts,
    noteId: r.noteId,
    learnedAt: r.learnedAt,
    isOverridden: r.isOverridden,
    isSkipped: r.isSkipped,
    originalCorrect: r.isOverridden ? !r.correct : r.correct,
    images: r.images,
    userAnswer: r.answer,
    reason: r.reason,
    pronunciation: r.wordDetail?.pronunciation,
    partOfSpeech: r.wordDetail?.partOfSpeech,
    originBreakdown: buildOriginBreakdown(r.wordDetail),
  };
}

export function reverseResultToItem(r: ReverseQuizResult, index: number): ResultItem {
  return {
    index,
    key: r.noteId.toString(),
    entry: r.expression,
    meaning: r.meaning,
    correct: r.correct,
    contexts: r.contexts,
    noteId: r.noteId,
    learnedAt: r.learnedAt,
    isOverridden: r.isOverridden,
    isSkipped: r.isSkipped,
    originalCorrect: r.isOverridden ? !r.correct : r.correct,
    images: r.images,
    userAnswer: r.answer,
    reason: r.reason,
    pronunciation: r.wordDetail?.pronunciation,
    partOfSpeech: r.wordDetail?.partOfSpeech,
    originBreakdown: buildOriginBreakdown(r.wordDetail),
  };
}

export function freeformResultToItem(r: FreeformResult, index: number): ResultItem {
  return {
    index,
    key: `freeform-${index}`,
    entry: r.word,
    meaning: r.meaning,
    correct: r.correct,
    contexts: r.contexts,
    learnedAt: r.learnedAt,
    isOverridden: r.isOverridden,
    isSkipped: r.isSkipped,
    originalCorrect: r.isOverridden ? !r.correct : r.correct,
    images: r.images,
    userAnswer: r.answer,
    reason: r.reason,
    pronunciation: r.wordDetail?.pronunciation,
    partOfSpeech: r.wordDetail?.partOfSpeech,
    originBreakdown: buildOriginBreakdown(r.wordDetail),
  };
}

export function etymologyResultToItem(r: EtymologyOriginResult, index: number): ResultItem {
  return {
    index,
    key: r.noteId ? r.noteId.toString() : `ety-${index}`,
    entry: r.origin,
    meaning: r.correctAnswer,
    correct: r.correct,
    noteId: r.noteId,
    learnedAt: r.learnedAt,
    isOverridden: r.isOverridden,
    isSkipped: r.isSkipped,
    originalCorrect: r.isOverridden ? !r.correct : r.correct,
    originBreakdown: [{
      origin: r.origin,
      meaning: r.correctAnswer,
      language: r.language,
      type: r.type,
    }],
    userAnswer: r.answer,
    reason: r.reason,
  };
}
