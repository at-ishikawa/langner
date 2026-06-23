import type { Chapter, Conversation, StoryScene, TranscriptCue } from "../types.js";

export interface SegmentationOptions {
  // Maximum accumulated duration of a single gap-based scene, in milliseconds.
  // Scenes longer than this are split even if the cue gap is short.
  maxSceneDurationMs: number;
  // Minimum silence (gap between cues) that triggers a new scene boundary
  // when no chapters are available, in milliseconds.
  gapThresholdMs: number;
}

export const DEFAULT_SEGMENTATION: SegmentationOptions = {
  maxSceneDurationMs: 60_000,
  gapThresholdMs: 4_000,
};

// Splits a list of transcript cues into story scenes.
// When chapters are available the chapters define scene boundaries. When they
// are not, cues are grouped into runs separated by silence and capped at a
// maximum duration.
//
// Cues with empty text are dropped. Scenes with no cues are dropped.
export function segmentIntoScenes(
  cues: TranscriptCue[],
  chapters: Chapter[],
  options: SegmentationOptions = DEFAULT_SEGMENTATION,
): StoryScene[] {
  const cleanCues = cues.filter((c) => c.text.trim().length > 0);
  if (cleanCues.length === 0) {
    return [];
  }
  if (chapters.length > 0) {
    return segmentByChapters(cleanCues, chapters);
  }
  return segmentByGap(cleanCues, options);
}

function segmentByChapters(cues: TranscriptCue[], chapters: Chapter[]): StoryScene[] {
  const sortedChapters = [...chapters].sort((a, b) => a.startMs - b.startMs);
  const scenes: StoryScene[] = [];

  for (let i = 0; i < sortedChapters.length; i++) {
    const chapter = sortedChapters[i]!;
    const next = sortedChapters[i + 1];
    const endMs = next ? next.startMs : Number.POSITIVE_INFINITY;
    const chapterCues = cues.filter(
      (c) => c.offsetMs >= chapter.startMs && c.offsetMs < endMs,
    );
    if (chapterCues.length === 0) {
      continue;
    }
    scenes.push({
      scene: chapter.title || `Chapter ${i + 1}`,
      conversations: cuesToConversations(chapterCues),
      definitions: [],
    });
  }

  return scenes;
}

function segmentByGap(
  cues: TranscriptCue[],
  options: SegmentationOptions,
): StoryScene[] {
  const scenes: StoryScene[] = [];
  let currentBucket: TranscriptCue[] = [];
  let bucketStartMs = cues[0]!.offsetMs;

  const flush = () => {
    if (currentBucket.length === 0) {
      return;
    }
    const startMs = currentBucket[0]!.offsetMs;
    const endMs = currentBucket[currentBucket.length - 1]!.offsetMs;
    scenes.push({
      scene: `Segment ${scenes.length + 1} (${formatTime(startMs)}–${formatTime(endMs)})`,
      conversations: cuesToConversations(currentBucket),
      definitions: [],
    });
    currentBucket = [];
  };

  for (let i = 0; i < cues.length; i++) {
    const cue = cues[i]!;
    const prev = cues[i - 1];
    const gapFromPrev = prev ? cue.offsetMs - prev.offsetMs : 0;
    const durationSoFar = cue.offsetMs - bucketStartMs;

    const shouldStartNewScene =
      currentBucket.length > 0 &&
      (gapFromPrev >= options.gapThresholdMs ||
        durationSoFar >= options.maxSceneDurationMs);

    if (shouldStartNewScene) {
      flush();
      bucketStartMs = cue.offsetMs;
    }
    if (currentBucket.length === 0) {
      bucketStartMs = cue.offsetMs;
    }
    currentBucket.push(cue);
  }
  flush();

  return scenes;
}

function cuesToConversations(cues: TranscriptCue[]): Conversation[] {
  // YouTube captions do not carry speaker labels. The Langner story notebook
  // format allows empty speaker values, so we leave the field blank and put
  // the caption text in the quote field.
  return cues.map((cue) => ({ speaker: "", quote: cue.text.trim() }));
}

function formatTime(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}
