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

export function etymologyResultToItem(r: EtymologyOriginResult, index: number): ResultItem {
  // r.meaning is always the origin's English gloss regardless of which
  // quiz side asked — set in both standard and reverse pages. Older
  // saved results (pre-field-added) fall back to correctAnswer, which
  // happens to be the meaning for standard quiz (the field's old role).
  const meaning = r.meaning || r.correctAnswer;
  return {
    index,
    key: r.noteId ? r.noteId.toString() : `ety-${index}`,
    entry: r.origin,
    meaning,
    correct: r.correct,
    noteId: r.noteId,
    learnedAt: r.learnedAt,
    isOverridden: r.isOverridden,
    isSkipped: r.isSkipped,
    originalCorrect: r.isOverridden ? !r.correct : r.correct,
    originBreakdown: [{
      origin: r.origin,
      meaning,
      language: r.language,
      type: r.type,
    }],
    userAnswer: r.answer,
    reason: r.reason,
    graphContext: r.graphContext,
  };
}
