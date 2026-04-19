import type { ScrapedVideo, StoryNotebook } from "../types.js";
import { segmentIntoScenes, type SegmentationOptions, DEFAULT_SEGMENTATION } from "./segmentation.js";

// Converts scraped YouTube video data into a Langner StoryNotebook.
//
// The notebook is immediately usable by the Langner reader:
// - `event` holds the video title and URL (matching the convention in
//    examples/stories/friends/example2.yml)
// - `metadata.series` holds the channel name
// - `scenes` are derived from chapters when available, otherwise from cue gaps
// - `definitions` are empty; definition extraction is out of scope for the PoC
export function buildStoryNotebook(
  video: ScrapedVideo,
  options: SegmentationOptions = DEFAULT_SEGMENTATION,
  captureDate: Date = new Date(),
): StoryNotebook {
  const scenes = segmentIntoScenes(video.cues, video.chapters, options);

  return {
    event: video.url
      ? `${video.title}: ${video.url}`
      : video.title,
    metadata: {
      series: video.channel || "YouTube",
      season: 0,
      episode: 0,
    },
    date: captureDate,
    scenes,
  };
}
