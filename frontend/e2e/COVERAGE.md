# E2E coverage matrix

This file is the authoritative record of what's covered by `.feature` scenarios.
Update it whenever you add a new route, a new user interaction, or a new feature
file.

## Routes × scenarios

| Route                                | Feature                  | Scenario                                  |
| ------------------------------------ | ------------------------ | ----------------------------------------- |
| `/`                                  | home-navigation          | Open the Learn hub from home              |
| `/`                                  | home-navigation          | Open the Quiz hub from home               |
| `/learn`                             | home-navigation          | Open the Learn hub from home              |
| `/learn`                             | learn-vocabulary         | Open the Idioms notebook and see its cards |
| `/learn`                             | learn-etymology          | Open the Word Roots etymology notebook    |
| `/learn/[id]`                        | _uncovered_              | _add when Story notebook detail exists_   |
| `/notebooks/[id]`                    | learn-vocabulary         | Open the Idioms notebook and see its cards |
| `/notebooks/[id]`                    | learn-vocabulary         | Expand a word card to see its meaning     |
| `/notebooks/etymology/[id]`          | learn-etymology          | Open the Word Roots etymology notebook    |
| `/notebooks/etymology/[id]/mindmap`  | learn-etymology          | Open the mindmap for an origin            |
| `/quiz`                              | home-navigation          | Open the Quiz hub from home               |
| `/quiz`                              | quiz-standard            | Finish a Standard quiz across two cards   |
| `/quiz`                              | quiz-reverse             | Finish a Reverse quiz across two cards    |
| `/quiz`                              | quiz-freeform            | Submit one freeform answer and finish     |
| `/quiz`                              | quiz-etymology           | Finish an etymology quiz in each mode     |
| `/quiz/standard`                     | quiz-standard            | Finish a Standard quiz across two cards   |
| `/quiz/reverse`                      | quiz-reverse             | Finish a Reverse quiz across two cards    |
| `/quiz/freeform`                     | quiz-freeform            | Submit one freeform answer and finish     |
| `/quiz/etymology-standard`           | quiz-etymology           | Standard mode example                     |
| `/quiz/etymology-reverse`            | quiz-etymology           | Reverse mode example                      |
| `/quiz/etymology-freeform`           | quiz-etymology           | Freeform mode example                     |
| `/quiz/complete`                     | quiz-standard            | Finish a Standard quiz across two cards   |

The route check script (`scripts/check-feature-coverage.ts`) enforces that every
route under `frontend/src/app/**/page.tsx` appears in at least one `.feature`
scenario via its URL path.

## Interactions per page

These are the user-visible inputs and outputs that a scenario should exercise.
Tick the box when at least one scenario hits the interaction.

### `/` (home)

- [x] Click Learn link → navigate to `/learn`
- [x] Click Quiz link → navigate to `/quiz`

### `/learn`

- [x] See vocabulary notebook in Vocabulary tab
- [x] Switch to Etymology tab
- [x] See etymology notebook in Etymology tab
- [x] Click a vocabulary notebook → navigate to `/notebooks/[id]`
- [x] Click an etymology notebook → navigate to `/notebooks/etymology/[id]`

### `/notebooks/[id]` (vocabulary detail)

- [x] See notebook heading
- [x] See word entries
- [x] Click a word card → see expanded details with example
- [ ] Filter words by learning status _(no scenario)_

### `/notebooks/etymology/[id]`

- [x] See notebook heading
- [x] See origin entry
- [x] Click mindmap link → navigate to `/mindmap`

### `/notebooks/etymology/[id]/mindmap`

- [x] See origin node
- [ ] Pan/zoom interactions _(not asserted)_

### `/quiz`

- [x] Choose Vocabulary tab (default)
- [x] Switch to Etymology tab
- [x] Choose a quiz mode (Standard / Reverse / Freeform)
- [x] Select a notebook via checkbox
- [x] Click Start

### `/quiz/standard`

- [x] See current card entry
- [x] Type an answer
- [x] Submit answer (Enter key or button)
- [x] See feedback / continue to next card
- [ ] Override grading result _(no scenario)_
- [ ] Skip a card _(no scenario)_

### `/quiz/reverse`

- [x] See meaning prompt
- [x] Type word
- [x] Submit and continue

### `/quiz/freeform`

- [x] Type word and meaning
- [x] Submit and finish

### `/quiz/etymology-*`

- [x] All three modes covered via Scenario Outline

### `/quiz/complete`

- [x] See total words count
- [ ] Restart / new quiz actions _(no scenario)_

## Add a row when you...

- Add a new page → add the route to the route table and a feature scenario that visits it.
- Add a new user input (button, form, link) → add a row under the relevant page.
- Add a new feature file → add its scenarios to the route table.
