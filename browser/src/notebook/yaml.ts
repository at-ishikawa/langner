import { stringify } from "yaml";
import type { StoryNotebook } from "../types.js";

// Serializes one or more story notebooks as a YAML document compatible with
// the existing Langner story notebook format (see examples/stories/).
//
// The top level is always a YAML sequence, even for a single notebook —
// this matches the Langner Go reader which expects `[]StoryNotebook`.
export function serializeStoryNotebooks(notebooks: StoryNotebook[]): string {
  const normalized = notebooks.map(normalizeStoryNotebook);
  return stringify(normalized, {
    lineWidth: 0,
    indent: 2,
    defaultStringType: "PLAIN",
    defaultKeyType: "PLAIN",
  });
}

// Forces a stable key order (event → metadata → date → scenes) so that the
// YAML output visually matches existing example notebooks and diffs cleanly
// when the file is regenerated.
function normalizeStoryNotebook(notebook: StoryNotebook): Record<string, unknown> {
  return {
    event: notebook.event,
    metadata: {
      series: notebook.metadata.series,
      season: notebook.metadata.season,
      episode: notebook.metadata.episode,
    },
    date: notebook.date,
    scenes: notebook.scenes.map((scene) => {
      const out: Record<string, unknown> = {
        scene: scene.scene,
        conversations: scene.conversations.map((conv) => ({
          speaker: conv.speaker,
          quote: conv.quote,
        })),
      };
      if (scene.definitions.length > 0) {
        out.definitions = scene.definitions.map((def) => ({
          expression: def.expression,
          meaning: def.meaning,
        }));
      }
      return out;
    }),
  };
}
