// Types mirroring the Langner story notebook format.
// The shape intentionally matches backend/internal/notebook/story.go so that
// the generated YAML can be consumed by Langner without transformation.

export interface StoryNotebookMetadata {
  series: string;
  season: number;
  episode: number;
}

export interface Conversation {
  speaker: string;
  quote: string;
}

export interface Definition {
  expression: string;
  meaning: string;
  // Other fields from the Go struct are intentionally omitted. They are
  // optional and the PoC never populates them.
}

export interface StoryScene {
  scene: string;
  conversations: Conversation[];
  definitions: Definition[];
}

export interface StoryNotebook {
  event: string;
  metadata: StoryNotebookMetadata;
  date: Date;
  scenes: StoryScene[];
}

// Intermediate types produced by the YouTube scraper.
// These are not part of the Langner notebook format.

export interface TranscriptCue {
  offsetMs: number;
  text: string;
}

export interface Chapter {
  startMs: number;
  title: string;
}

export interface ScrapedVideo {
  videoId: string;
  title: string;
  channel: string;
  url: string;
  cues: TranscriptCue[];
  chapters: Chapter[];
}
