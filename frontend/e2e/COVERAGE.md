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
| `/notebooks/etymology/[id]`         | learn-etymology   | Open the Word Roots etymology notebook                  |
| `/notebooks/etymology/[id]`         | learn-etymology   | Open the mindmap for an origin                          |
| `/notebooks/etymology/[id]/mindmap` | learn-etymology   | Open the mindmap for an origin                          |
| `/quiz`                             | home-navigation   | Open the Quiz hub from home                             |
| `/quiz`                             | quiz-standard     | Finish a Standard quiz across two cards                 |
| `/quiz`                             | quiz-reverse      | Finish a Reverse quiz across two cards                  |
| `/quiz`                             | quiz-freeform     | Submit one freeform answer and finish                   |
| `/quiz`                             | quiz-etymology    | Finish an etymology quiz in Freeform mode               |
| `/quiz`                             | quiz-etymology    | Finish an etymology quiz in Standard mode               |
| `/quiz`                             | quiz-etymology    | Finish an etymology quiz in Reverse mode                |
| `/quiz/standard`                    | quiz-standard     | Finish a Standard quiz across two cards                 |
| `/quiz/reverse`                     | quiz-reverse      | Finish a Reverse quiz across two cards                  |
| `/quiz/freeform`                    | quiz-freeform     | Submit one freeform answer and finish                   |
| `/quiz/etymology-standard`          | quiz-etymology    | Finish an etymology quiz in Standard mode               |
| `/quiz/etymology-reverse`           | quiz-etymology    | Finish an etymology quiz in Reverse mode                |
| `/quiz/etymology-freeform`          | quiz-etymology    | Finish an etymology quiz in Freeform mode               |
| `/quiz/complete`                    | quiz-standard     | Finish a Standard quiz across two cards                 |
| `/quiz/complete`                    | quiz-reverse      | Finish a Reverse quiz across two cards                  |
| `/quiz/complete`                    | quiz-freeform     | Submit one freeform answer and finish                   |
| `/quiz/complete`                    | quiz-etymology    | Finish an etymology quiz in each mode                   |

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
- [ ] Filter words by learning status _(no scenario)_

### `/notebooks/etymology/[id]`

- [x] See literal "Etymology" page heading (the notebook name is shown in
      the breadcrumb, not in a heading)
- [x] See origin entries ("graph", "tele")
- [x] Click an origin card → URL gains `?origin=<name>` and the origin detail panel opens
- [x] Click "View Mindmap" → navigate to `/notebooks/etymology/[id]/mindmap`

### `/notebooks/etymology/[id]/mindmap`

- [x] See the focused origin's ReactFlow node ("graph")
- [ ] Pan / zoom interactions _(not asserted)_

### `/quiz` (quiz hub)

- [x] See the default Vocabulary tab with quiz modes listed
- [x] Switch to the Etymology tab
- [x] Choose a quiz mode (Standard / Reverse / Freeform — exercised by both Vocabulary and Etymology tabs)
- [x] Toggle "Include unstudied words" switch (used by Standard/Reverse — vocab and etymology)
- [x] Select a notebook via the Checkbox.Root label
- [x] Click "Start" → navigate to the per-mode quiz page

### `/quiz/standard`

- [x] See the current card's expression as a heading
- [x] Type into the single answer input
- [x] Click "Submit" (auto-advance between cards; BatchFeedback only on the final card)
- [x] Reach `/quiz/complete` and see "Total: 2 words"
- [ ] Override grading result _(no scenario)_
- [ ] Skip a card _(no scenario)_
- [ ] Retry grading after a network error _(no scenario)_

### `/quiz/reverse`

- [x] See the meaning prompt + "Type the word" input
- [x] Type a word and click Submit, twice (across two cards)
- [x] Reach `/quiz/complete` and see "Total: 2 words"
- [ ] List words missing context (Reverse-only toggle) _(no scenario)_

### `/quiz/freeform`

- [x] See both the freeform Word and Meaning inputs
- [x] Type a word, type a meaning, click Submit
- [x] Click "See Results" → navigate to `/quiz/complete`

### `/quiz/etymology-standard`

- [x] See the current origin and "type the meaning..." input
- [x] Type a meaning and click Submit, twice
- [x] Reach `/quiz/complete`

### `/quiz/etymology-reverse`

- [x] See the current meaning and "type the origin..." input
- [x] Type an origin and click Submit, twice
- [x] Reach `/quiz/complete`

### `/quiz/etymology-freeform`

- [x] See separate Origin (`e.g., spect`) and Meaning (`e.g., to look or see`) inputs
- [x] Type both, click Submit, then click "See Results"
- [x] Submit stays enabled when the origin is "due now" (seed sets
      `interval_days: 0` for the etymology_freeform log)

### `/quiz/complete`

- [x] See "Total: N words" summary
- [ ] Restart / new quiz action _(no scenario)_
- [ ] Per-card override/skip actions on the summary _(no scenario)_

## Known gaps

These exist as `[ ]` items above and are listed here as a single backlog:

- `/notebooks/[id]`: filter words by learning status
- `/notebooks/etymology/[id]/mindmap`: pan/zoom
- `/quiz/standard`: override grading, skip a card, retry grading
- `/quiz/reverse`: "List words missing context" toggle
- `/quiz/complete`: restart, per-card override/skip from the summary

## Add a row when you...

- Add a new page → add the route to the route table and a feature scenario
  that visits it. The Layer 2 script will fail CI until it's referenced.
- Add a new user input (button, form, link) → add a row under the relevant
  page's interactions list.
- Add a new feature file → add its scenarios to the route table.
