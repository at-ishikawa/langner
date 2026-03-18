---
title: "UI/UX Design"
weight: 2
---

# UI/UX Design

Mobile-first wireframes (375px viewport) for etymology-based word learning pages and quizzes. The quiz screens reuse the same layout patterns as the existing Standard, Reverse, and Freeform quizzes — progress bars, fixed-bottom buttons, feedback banners, and session complete cards are all consistent with the current implementation.

## Screen Flow

Two main flows: **Browse** (explore origins and words) and **Quiz** (test etymology knowledge with three modes).

![Screen Flow](/proposals/etymology-learning/wireframes/00-screen-flow.svg)

## Browse Flow

### Screen 1: Origin List

The entry point for an etymology notebook. Shows all origins with their type, language, meaning, and word count.

- **Search bar** at the top to filter by origin or meaning text
- **Two tabs**: "All Origins" (default list) and "By Meaning" (grouped view)
- Each origin card shows:
  - Origin (blue, clickable)
  - Type badge (root/prefix/suffix) with color coding
  - Language badge (Greek, Latin, etc.)
  - Meaning
  - Word count
- Tapping an origin navigates to Origin Detail

![Origin List](/proposals/etymology-learning/wireframes/01-origin-list.svg)

### Screen 2: Origin Detail

Shows all words that use a specific origin. This is the core navigation screen — each word's breakdown contains clickable links to other origins.

- **Origin info card** at the top: origin value, type, language, meaning
- **Word cards** listed below, each showing:
  - Word
  - Meaning
  - Origin breakdown with each origin as a clickable blue link
  - The current origin is highlighted with a border and "(current)" label
- **Tapping a different origin** in any word's breakdown navigates to that origin's detail page
- **Back navigation** returns to the previous origin or the origin list

![Origin Detail](/proposals/etymology-learning/wireframes/02-origin-detail.svg)

### Screen 3: Browse by Meaning

Accessed via the "By Meaning" tab on the Origin List. Groups origins that share similar meanings, enabling cross-language discovery.

- Origins are grouped under their shared meaning (e.g., "to write")
- Each origin shows its language badge and word count
- Tapping any origin navigates to its Origin Detail page
- Useful for comparing word families across languages for the same concept (e.g., Greek `graphein` vs. Latin `scribere` for "to write")

![Browse by Meaning](/proposals/etymology-learning/wireframes/03-browse-by-meaning.svg)

## Quiz Flow

Three quiz modes are available: **Breakdown**, **Assembly**, and **Freeform**. The Breakdown and Assembly modes are structured quizzes with a fixed word count and progress bar. The Freeform mode is open-ended — the user types any word and the system looks it up.

### Screen 4: Quiz Start

Select notebooks and quiz mode before starting a structured quiz (Breakdown or Assembly).

- **Single notebook list** with checkboxes (multi-select supported): shows regular notebooks (book, flashcard, story) that have at least one definition with `origin_parts`. Each notebook shows its name and the count of words with etymology data. The system automatically finds the relevant origins from whatever etymology notebooks are loaded — the user does not need to separately select etymology notebooks.
- **Quiz mode** radio buttons:
  - **Breakdown**: Given a word, type its origins and their meanings
  - **Assembly**: Given origins with meanings, type the word
- **Toggle** for including unstudied words
- **Word count** shows how many words are due for review (SM-2), based on the selected notebooks
- **Start button** begins the quiz

![Quiz Start](/proposals/etymology-learning/wireframes/04-quiz-start.svg)

### Screen 5: Quiz Card — Breakdown Mode

The user sees a word and must identify its etymological origins.

- **Progress bar** and counter at the top (same style as existing quizzes)
- **Word card** with the word displayed prominently and its meaning below
- **Input rows** for each origin: two fields per row (origin and meaning), connected by "="
- **"+ Add origin"** link to add more rows (words can have varying numbers of origins)
- **Submit button** fixed at the bottom (same position as existing quizzes)
- Input fields are auto-focused for fast typing

![Quiz Breakdown](/proposals/etymology-learning/wireframes/05-quiz-breakdown.svg)

### Screen 6: Quiz Card — Assembly Mode

The user sees etymological origins and must type the word they form.

- **Progress bar** and counter at the top
- **Origins card** showing each origin with its meaning and language, connected by "+" signs
- **"=" separator** followed by a single text input for the word
- **Submit button** fixed at the bottom

![Quiz Assembly](/proposals/etymology-learning/wireframes/06-quiz-assembly.svg)

### Screen 7: Feedback — Correct

Shown after a correct answer in Breakdown or Assembly mode. Provides the full breakdown and related words for exploration.

- **Green banner** with checkmark (same style as existing quiz feedback)
- **Word card** with word and meaning
- **Your answer** section with green border, showing each origin with a checkmark
- **Full breakdown** section showing each origin as a clickable link with its meaning, language badge, and type badge
- **Related words** section grouping other words that share the same origins — encouraging the user to explore further
- **Next review date** based on SM-2 calculation
- **Next button** to proceed

![Feedback Correct](/proposals/etymology-learning/wireframes/07-feedback-correct.svg)

### Screen 8: Feedback — Incorrect

Shown after an incorrect answer. Highlights what was wrong and shows the correct breakdown.

- **Red banner** with X mark (same style as existing quiz feedback)
- **Word card** with word and meaning
- **Your answer** section with red border, incorrect parts shown with strikethrough and X mark, correct parts with checkmark
- **Correct breakdown** section with clickable origin links, language and type badges
- **Related words** section (same as correct feedback — learning opportunity)
- **Next review date** (shorter interval due to incorrect answer)
- **Next button** to proceed

![Feedback Incorrect](/proposals/etymology-learning/wireframes/08-feedback-incorrect.svg)

### Screen 9: Freeform Etymology Quiz

Open-ended quiz mode — the user types any word and identifies its origins. Same pattern as the existing Freeform quiz but for etymology. The freeform quiz also requires notebook selection (etymology + definition notebooks), using the same selections as the quiz start screen. The user navigates through the quiz start screen first to select notebooks before entering freeform mode.

- **No progress bar** (open-ended, like existing Freeform quiz)
- **Word input** at the top — user types any word
- **Status indicator** shows whether the word was found in etymology notebooks
- **Origin input rows** below — same layout as Breakdown mode (origin + meaning pairs with "+" to add more)
- **Submit button** fixed at the bottom
- After feedback, buttons to "Next Word" or "See Results" (same as existing Freeform quiz)

![Freeform Etymology](/proposals/etymology-learning/wireframes/10-quiz-freeform.svg)

### Screen 10: Session Complete

Summary of the quiz session. Uses the same layout as the existing session complete page.

- **Stats card** with total words, correct count (green), incorrect count (red) — same layout as existing
- **Incorrect section** listed first, each card showing:
  - Red top border (same as existing result cards)
  - Word and user's answer
  - Correct breakdown
  - Next review date
- **Correct section** below, each card showing:
  - Green top border (same as existing result cards)
  - Word and origin breakdown
  - Next review date
- **Back to Start** button

The existing session complete page already supports Override, Skip/Resume, and Change Review Date actions on each card. Etymology quiz results should support these same actions.

![Session Complete](/proposals/etymology-learning/wireframes/09-session-complete.svg)

### Screen 11: Notebook Detail — Etymology Links

Existing notebook detail pages (book, flashcard, story) are enhanced to show `origin_parts` on definitions that have them. This integrates etymology data directly into the vocabulary browsing experience without requiring the user to navigate to a separate etymology section.

- Each definition card displays the expression and meaning as usual
- **Definitions with `origin_parts`** show an additional "Origins:" row below the meaning, with each origin rendered as a clickable blue pill/badge
- Each origin pill shows the origin name (blue, clickable) and a language badge (gray)
- Origins are connected by "+" separators to show how the word is composed
- **Tapping an origin pill** navigates to that origin's detail page (Screen 2), where the user can explore all words built from that origin
- **Definitions without `origin_parts`** (e.g., idioms like "break the ice") display normally with no origin breakdown — the section is entirely omitted, not shown empty
- This provides a natural discovery path: users browsing their vocabulary can see etymology breakdowns in context and follow links to explore origin families

![Notebook Detail — Etymology Links](/proposals/etymology-learning/wireframes/11-notebook-origin-parts.svg)

## Design Notes

### Color Coding

| Element | Color |
|---------|-------|
| Origin links | Blue (#2563eb) — clickable |
| Root badge | Blue (#dbeafe) |
| Prefix badge | Amber (#fef3c7) |
| Suffix badge | Green (#dcfce7) |
| Language badge | Gray (#f3f4f6) |
| Correct feedback | Green (#16a34a) |
| Incorrect feedback | Red (#dc2626) |

### Navigation Pattern

The browse flow is designed as a **graph navigation** rather than a linear hierarchy:

1. Origin List is the starting point
2. Origin Detail shows words, each with clickable origins
3. Clicking an origin in a word breakdown navigates to that origin's detail
4. The user can traverse the graph indefinitely: origin → word → different origin → word → ...
5. Back navigation and breadcrumbs let users retrace their path

### Reuse of Existing UI Patterns

The etymology quiz screens intentionally reuse the same patterns from the current quiz implementation:

| Pattern | Source | Used In |
|---------|--------|---------|
| Progress bar + counter | Standard/Reverse quiz | Breakdown, Assembly |
| Fixed-bottom submit/next | All existing quizzes | All etymology quizzes |
| Green/red feedback banner | All existing quizzes | Breakdown, Assembly, Freeform |
| Result cards with color top bar | Session Complete page | Session Complete |
| Override / Skip / Change Date | Quiz UI Improvement | Session Complete |
| Word input + status indicator | Freeform quiz | Freeform Etymology |
| "Next Word" / "See Results" | Freeform quiz | Freeform Etymology |
| Stats card layout | Session Complete page | Session Complete |
