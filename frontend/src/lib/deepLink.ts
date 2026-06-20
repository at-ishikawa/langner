// findDeepLinkStoryIndex picks the story (lesson) on the /learn/[id] page
// that contains the deep-linked word. The analytics card builds the URL
// from the learning-history record, so:
//
//   - targetWord may use a conjugated / plural form ("stuffed shirt") while
//     the YAML stores the canonical (`expression: stuffed shirts`) with a
//     dictionary alias (`definition: stuffed shirt`). Matching both fields
//     keeps the deep link from silently falling back to Lesson 1.
//   - targetScene, when provided, restricts the match to scenes whose
//     title matches exactly. Story-style notebooks declare the title as
//     the lesson's multi-paragraph plot summary; both the notebook detail
//     and the analytics card pull from the same YAML block so equality
//     is the right comparison.
//
// Returns -1 when no story matches; the caller falls back to whatever
// the user manually selected.
type DeepLinkScene = {
  readonly title: string;
  readonly definitions: readonly { readonly expression: string; readonly definition: string }[];
};

type DeepLinkStory = {
  readonly scenes: readonly DeepLinkScene[];
};

export function findDeepLinkStoryIndex(
  stories: readonly DeepLinkStory[],
  targetWord: string,
  targetScene: string,
): number {
  if (!targetWord) return -1;
  const lower = targetWord.toLowerCase();
  return stories.findIndex((story) =>
    story.scenes.some((scene) => {
      if (targetScene && scene.title !== targetScene) return false;
      return scene.definitions.some(
        (d) =>
          d.expression.toLowerCase() === lower ||
          d.definition.toLowerCase() === lower,
      );
    }),
  );
}
