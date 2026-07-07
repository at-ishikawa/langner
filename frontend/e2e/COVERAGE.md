# E2E coverage matrix

This file is the authoritative record of what's covered by `.feature` scenarios.
Update it whenever you add a new route, a new user interaction, or a new
feature file. The route table is enforced by `scripts/check-feature-coverage.ts`
(Layer 2) and by `e2e/reporters/coverage-reporter.ts` (Layer 3, traces every
actual navigation).

## Routes × scenarios

| Route                               | Feature           | Scenario                                                |
| ----------------------------------- | ----------------- | ------------------------------------------------------- |
| `/`                                 | home-navigation   | Open the Learn hub from home                            |
| `/`                                 | home-navigation   | Open the Quiz hub from home                             |
| `/learn`                            | home-navigation   | Open the Learn hub from home                            |
| `/learn`                            | learn-vocabulary  | Open the Idioms flashcard notebook from the Learn hub   |
| `/learn`                            | learn-etymology   | Open the Word Roots etymology notebook                  |
| `/learn/[id]`                       | learn-vocabulary  | Open the Short Tales story reader                       |
| `/notebooks/[id]`                   | learn-vocabulary  | Open the Idioms flashcard notebook from the Learn hub   |
| `/notebooks/[id]`                   | learn-vocabulary  | Expand a flashcard word card to see its meaning         |
| `/notebooks/[id]`                   | learn-vocabulary  | Filter Idioms words by Misunderstood learning status    |
| `/notebooks/[id]`                   | learn-vocabulary  | Toggle a per-quiz-type skip on a word card              |
| `/notebooks/etymology/[id]`         | learn-etymology   | Open the Word Roots etymology notebook                  |
| `/notebooks/etymology/[id]`         | learn-etymology   | Open the mindmap for an origin                          |
| `/notebooks/etymology/[id]/mindmap` | learn-etymology   | Open the mindmap for an origin                          |
| `/quiz`                             | home-navigation   | Open the Quiz hub from home                             |
| `/quiz`                             | quiz-standard     | Finish a Standard quiz across two cards                 |
| `/quiz`                             | quiz-standard     | Skip a Standard quiz card with "Don't Know"             |
| `/quiz`                             | quiz-standard     | Override an answer on the BatchFeedback view            |
| `/quiz`                             | quiz-standard     | Override, exclude, then restart from the summary        |
| `/quiz`                             | quiz-standard     | Retry grading after a transient failure                 |
| `/quiz`                             | quiz-reverse      | Finish a Reverse quiz across two cards                  |
| `/quiz`                             | quiz-reverse      | Reverse quiz with the "List words missing context" filter |
| `/quiz`                             | quiz-freeform     | Submit one freeform answer and finish                   |
| `/quiz`                             | quiz-etymology    | Finish an etymology quiz in Freeform mode               |
| `/quiz`                             | quiz-etymology    | Finish an etymology quiz in Standard mode               |
| `/quiz`                             | quiz-etymology    | Finish an etymology quiz in Reverse mode                |
| `/quiz/standard`                    | quiz-standard     | (all 5 standard scenarios)                              |
| `/quiz/reverse`                     | quiz-reverse      | (both reverse scenarios)                                |
| `/quiz/freeform`                    | quiz-freeform     | Submit one freeform answer and finish                   |
| `/quiz/etymology-standard`          | quiz-etymology    | Finish an etymology quiz in Standard mode               |
| `/quiz/etymology-reverse`           | quiz-etymology    | Finish an etymology quiz in Reverse mode                |
| `/quiz/etymology-freeform`          | quiz-etymology    | Finish an etymology quiz in Freeform mode               |
| `/quiz/complete`                    | quiz-standard     | (all 5 standard scenarios reach it)                     |
| `/quiz/complete`                    | quiz-reverse      | (both reverse scenarios reach it)                       |
| `/quiz/complete`                    | quiz-freeform     | Submit one freeform answer and finish                   |
| `/quiz/complete`                    | quiz-etymology    | (all 3 etymology scenarios reach it)                    |
| `/quiz/relearn`                     | relearn           | Relearn a word missed moments ago in a Standard quiz    |
| `/quiz/relearn/session`             | relearn           | Relearn a word missed moments ago in a Standard quiz    |
| `/quiz/relearn/complete`            | relearn           | Relearn a word missed moments ago in a Standard quiz    |
| `/analytics`                        | analytics         | Open the Analytics Day List from home                   |
| `/analytics/[date]`                 | analytics         | Open a Day Detail page with seeded wrong words          |

## Interactions per page

These are the user-visible inputs and outputs that a scenario should exercise.
Tick the box when at least one scenario hits the interaction.

### `/` (home)

- [x] Click Learn link → navigate to `/learn`
- [x] Click Quiz link → navigate to `/quiz`

### `/learn`

- [x] See vocabulary notebook in Vocabulary tab (default)
- [x] Switch to Etymology tab
- [x] See etymology notebook in Etymology tab
- [x] Click a flashcard notebook → navigate to `/notebooks/[id]`
- [x] Click an etymology notebook → navigate to `/notebooks/etymology/[id]`

### `/learn/[id]` (story reader)

- [x] Navigate to a story notebook's reader
- [x] See the notebook heading (e.g. "Short Tales")
- [x] See prose / dialogue text from the seeded story

### `/notebooks/[id]` (flashcard or story-list view)

- [x] See notebook heading (e.g. "Idioms")
- [x] Click a story row → see its word cards (e.g. "Common Idioms" → cards)
- [x] See expression text on each word card ("break the ice", "lose one's temper")
- [x] Click a word card → see expanded card with example sentence
- [x] Filter words by learning status via the `<select>` dropdown
- [x] Toggle a per-quiz-type "Skip from quiz" checkbox (Standard / Reverse / Freeform / All)

### `/notebooks/etymology/[id]`

- [x] See literal "Etymology" page heading (the notebook name is shown in
      the breadcrumb, not in a heading)
- [x] See origin entries ("graph", "tele")
- [x] Click an origin card → URL gains `?origin=<name>` and the origin detail panel opens
- [x] Click "View Mindmap" → navigate to `/notebooks/etymology/[id]/mindmap`

### `/notebooks/etymology/[id]/mindmap`

- [x] See the focused origin's ReactFlow node ("graph")

> **Out of scope:** pan / zoom is handled internally by ReactFlow. Driving
> wheel/drag events from Playwright and asserting that the viewport math
> changed would be testing the third-party library, not our app, so it is
> not part of this matrix.

### `/quiz` (quiz hub)

- [x] See the default Vocabulary tab with quiz modes listed
- [x] Switch to the Etymology tab
- [x] Choose a quiz mode (Standard / Reverse / Freeform — exercised by both Vocabulary and Etymology tabs)
- [x] Toggle "Include unstudied words" switch (used by Standard/Reverse — vocab and etymology)
- [x] Toggle "List words missing context" switch (Reverse-only, see quiz-reverse.feature)
- [x] Select a notebook via the Checkbox.Root label
- [x] Click "Start" → navigate to the per-mode quiz page

### `/quiz/standard`

- [x] See the current card's expression as a heading
- [x] Type into the single answer input
- [x] Click "Submit" (auto-advance between cards; BatchFeedback only on the final card)
- [x] Click "Don't Know" to skip a card
- [x] Click "Retry grading" after a transient grading-RPC failure
- [x] Override (Mark Correct/Incorrect) on the BatchFeedback view
- [x] Reach `/quiz/complete` and see "Total: 2 words"

### `/quiz/reverse`

- [x] See the meaning prompt + "Type the word" input
- [x] Type a word and click Submit, twice (across two cards)
- [x] Honor the "List words missing context" filter (one matching card runs end-to-end)
- [x] Reach `/quiz/complete` and see "Total: N words"

### `/quiz/freeform`

- [x] See both the freeform Word and Meaning inputs
- [x] Type a word, type a meaning, click Submit
- [x] Click "See Results" → navigate to `/quiz/complete`

### `/quiz/etymology-standard`

- [x] See the current origin and "type the meaning..." input
- [x] Type a meaning and click Submit, twice
- [x] See the "your answer" chip on the BatchFeedback card for a correct result
- [x] Reach `/quiz/complete`

### `/quiz/etymology-reverse`

- [x] See the current meaning and "type the origin..." input
- [x] Type an origin and click Submit, twice
- [x] See the "your answer" chip on the BatchFeedback card for a correct result
- [x] Reach `/quiz/complete`

### `/quiz/etymology-freeform`

- [x] See separate Origin (`e.g., spect`) and Meaning (`e.g., to look or see`) inputs
- [x] Type both, click Submit, then click "See Results"
- [x] See the "your answer" chip on the FeedbackActions card for a correct result
- [x] Submit stays enabled when the origin is "due now" (seed sets
      `interval_days: 0` for the etymology_freeform log)

### `/quiz/complete`

- [x] See "Total: N words" summary
- [x] Override (Mark Correct/Incorrect) on a result card
- [x] Click "Exclude" to mark a result as skipped
- [x] Click "Back to Start" → navigate back to `/quiz`

### `/analytics`

- [x] Open from the home Analytics tile
- [x] Day list renders with no "Failed to load analytics:" banner (DB-backed)

### `/analytics/[date]`

- [x] Open `/analytics/{YYYY-MM-DD}?range=0` directly
- [x] Page renders with no "Failed to load day:" banner (DB-backed; catches
      MySQL-only bugs like `only_full_group_by`)
- [x] At least one seeded wrong word is visible

## Add a row when you...

- Add a new page → add the route to the route table and a feature scenario
  that visits it. The Layer 2 script will fail CI until it's referenced.
- Add a new user input (button, form, link) → add a row under the relevant
  page's interactions list.
- Add a new feature file → add its scenarios to the route table.
