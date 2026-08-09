import type {
  QuizResult,
  ReverseQuizResult,
  FreeformResult,
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
    senseId: r.senseId,
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
    wordDetail: r.wordDetail,
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
    senseId: r.senseId,
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
    wordDetail: r.wordDetail,
  };
}

export function freeformResultToItem(r: FreeformResult, index: number): ResultItem {
  return {
    index,
    key: r.noteId ? `freeform-${r.noteId.toString()}-${index}` : `freeform-${index}`,
    entry: r.word,
    meaning: r.meaning,
    correct: r.correct,
    contexts: r.contexts,
    noteId: r.noteId,
    senseId: r.senseId,
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
    wordDetail: r.wordDetail,
  };
}
