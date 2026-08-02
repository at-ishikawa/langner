// Shared segmenting logic for the grammar inline-correction card: a post (or,
// for the Relearn Quiz, a single missed correction) is rendered as an ordered
// list of plain-text runs and blank tokens so each mistake is shown in place
// — the whole entry, with the mistaken span struck through. Blanks are
// located by their incorrect span; duplicates map to successive occurrences
// in blank order. Any blank whose span can't be placed is appended at the end
// so the rendered set always equals the graded set.
//
// Generic over the blank type so the live grammar quiz (whose blanks carry a
// per-blank grading noteId) and the Relearn Quiz (whose card carries a
// single blank identified by the card's own noteId) can both drive the same
// matching algorithm without either side reimplementing it.

export type PostSegment<T> = { type: "text"; text: string } | { type: "blank"; blank: T };

export function segmentPost<T extends { incorrect: string }>(
  postText: string,
  blanks: T[],
): PostSegment<T>[] {
  const cursors = new Map<string, number>();
  const placed: { blank: T; pos: number }[] = [];
  const unplaced: T[] = [];
  for (const blank of blanks) {
    const from = cursors.get(blank.incorrect) ?? 0;
    const pos = blank.incorrect ? postText.indexOf(blank.incorrect, from) : -1;
    if (pos >= 0) {
      cursors.set(blank.incorrect, pos + blank.incorrect.length);
      placed.push({ blank, pos });
    } else {
      unplaced.push(blank);
    }
  }
  placed.sort((a, b) => a.pos - b.pos);

  const segments: PostSegment<T>[] = [];
  let cursor = 0;
  for (const { blank, pos } of placed) {
    if (pos < cursor) {
      unplaced.push(blank);
      continue;
    }
    if (pos > cursor) segments.push({ type: "text", text: postText.slice(cursor, pos) });
    segments.push({ type: "blank", blank });
    cursor = pos + blank.incorrect.length;
  }
  if (cursor < postText.length) segments.push({ type: "text", text: postText.slice(cursor) });
  for (const blank of unplaced) segments.push({ type: "blank", blank });
  return segments;
}

// PILL_STYLES is the shared status palette for a graded correction pill —
// used by the live grammar quiz's post view and the Relearn Quiz's grammar
// card so a graded mistake reads identically in both places.
export const PILL_STYLES = {
  correct: { bg: "green.100", darkBg: "green.900", color: "green.700", darkColor: "green.200", glyph: "✓", label: "correct" },
  incorrect: { bg: "red.100", darkBg: "red.900", color: "red.700", darkColor: "red.200", glyph: "✗", label: "incorrect" },
  skipped: { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300", glyph: "–", label: "excluded" },
} as const;
