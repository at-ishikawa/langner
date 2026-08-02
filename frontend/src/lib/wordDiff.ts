// A minimal word-level diff for grammar feedback: given the user's answer and
// the reference correction, mark which words differ so the card can highlight
// exactly what still needs changing (instead of showing two near-identical
// sentences the reader has to compare by eye). Matching is case- and
// surrounding-punctuation-insensitive so "School" and "school," align.

export interface DiffToken {
  text: string;
  changed: boolean;
}

export interface WordDiff {
  left: DiffToken[]; // tokens of the "from" string (e.g. the user's answer)
  right: DiffToken[]; // tokens of the "to" string (e.g. the correction)
}

function tokenize(s: string): string[] {
  return s.trim().match(/\S+/g) ?? [];
}

function normalize(word: string): string {
  return word.toLowerCase().replace(/^[^\p{L}\p{N}]+|[^\p{L}\p{N}]+$/gu, "");
}

// Standard longest-common-subsequence diff over words. Words on the common
// subsequence are unchanged; everything else is marked changed on its own side.
export function wordDiff(fromStr: string, toStr: string): WordDiff {
  const from = tokenize(fromStr);
  const to = tokenize(toStr);
  const n = from.length;
  const m = to.length;

  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] =
        normalize(from[i]) === normalize(to[j])
          ? dp[i + 1][j + 1] + 1
          : Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }

  const left: DiffToken[] = [];
  const right: DiffToken[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (normalize(from[i]) === normalize(to[j])) {
      left.push({ text: from[i], changed: false });
      right.push({ text: to[j], changed: false });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      left.push({ text: from[i], changed: true });
      i++;
    } else {
      right.push({ text: to[j], changed: true });
      j++;
    }
  }
  while (i < n) left.push({ text: from[i++], changed: true });
  while (j < m) right.push({ text: to[j++], changed: true });
  return { left, right };
}
