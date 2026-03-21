---
title: "UI/UX Design"
weight: 2
---

# UI/UX Design

Mobile-first wireframes (375px viewport) for the Langner language learning app. The app is organized into three top-level experiences: **Books** (reading), **Learn** (browsing vocabulary and etymology), and **Quiz** (practicing with spaced repetition). The quiz screens reuse the same layout patterns as the existing Standard, Reverse, and Freeform quizzes — progress bars, fixed-bottom buttons, feedback banners, and session complete cards are all consistent with the current implementation.

## Navigation Structure

The app uses a three-card home page that routes users into distinct experiences:

- **Books** — read books and look up words (separate reader UI)
- **Learn** — browse vocabulary notebooks (stories, flashcards) and etymology origins
- **Quiz** — practice vocabulary (standard/reverse/freeform) and etymology (breakdown/assembly/freeform)

The Learn and Quiz pages each use **tabs** ("Vocabulary" / "Etymology") to separate the two domains, so users can switch instantly without scrolling past a long list of notebooks.

## Screen Flow

![Screen Flow](/proposals/etymology-learning/wireframes/00-screen-flow.svg)

## Home Page

### Screen 0: Home

The entry point for the app. Three large cards let the user choose their activity.

- **Books card** — navigates to `/books` (the book reader, unchanged)
- **Learn card** — navigates to `/learn` (the learn hub)
- **Quiz card** — navigates to `/quiz` (the quiz hub)
- Clean layout with icon, title, and short description per card
- Cards use white background, rounded corners, subtle border — consistent with the app style

![Home](/proposals/etymology-learning/wireframes/12-home.svg)

## Learn Flow

### Screen L1: Learn Hub

The hub page for all browsing and learning. Two tabs at the top switch between Vocabulary and Etymology.

- **Two tabs**: "Vocabulary" (default) and "Etymology" — with underline indicator on the active tab
- **Vocabulary tab** — lists all non-etymology notebooks (stories, flashcards, books). Each card shows the notebook name and word count. Tapping navigates to the existing notebook detail page (`/notebooks/{id}`). Summary footer shows total notebook and word counts.
- **Etymology tab** — lists all etymology notebooks. Each card shows the notebook name, origin count, and word count. Tapping navigates to the origin list page (`/notebooks/etymology/{id}`). Summary footer shows total notebook and origin counts.
- Back link returns to Home

![Learn Hub — Vocabulary tab](/proposals/etymology-learning/wireframes/13-learn-hub.svg)

![Learn Hub — Etymology tab](/proposals/etymology-learning/wireframes/13b-learn-hub-etymology.svg)

### Screen L2: Origin List

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

### Screen L3: Origin Detail

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

### Screen L4: Browse by Meaning

Accessed via the "By Meaning" tab on the Origin List. Groups origins that share similar meanings, enabling cross-language discovery.

- Origins are grouped under their shared meaning (e.g., "to write")
- Each origin shows its language badge and word count
- Tapping any origin navigates to its Origin Detail page
- Useful for comparing word families across languages for the same concept (e.g., Greek `graphein` vs. Latin `scribere` for "to write")

![Browse by Meaning](/proposals/etymology-learning/wireframes/03-browse-by-meaning.svg)

## Quiz Flow

### Screen Q1: Quiz Hub

The hub page for all quiz modes. Two tabs at the top switch between Vocabulary and Etymology — same pattern as the Learn Hub.

- **Two tabs**: "Vocabulary" (default) and "Etymology" — with underline indicator on the active tab
- **Vocabulary tab** — three quiz mode cards:
  - **Standard**: See a word, type its meaning
  - **Reverse**: See a meaning, type the word
  - **Freeform**: Type any word and its meaning
- **Etymology tab** — three quiz mode cards:
  - **Breakdown**: See a word, identify its origins and meanings
  - **Assembly**: See origins and meanings, type the word
  - **Freeform**: Type any word and break down its origins
- Each card shows the mode name and a short description
- Tapping a card navigates to the quiz start screen for that mode
- Back link returns to Home

![Quiz Hub — Vocabulary tab](/proposals/etymology-learning/wireframes/14-quiz-hub.svg)

![Quiz Hub — Etymology tab](/proposals/etymology-learning/wireframes/14b-quiz-hub-etymology.svg)

### Screen Q2: Vocabulary Quiz Start

Select notebooks and start a vocabulary quiz. This screen is shown for Standard, Reverse, and Freeform vocabulary quizzes.

- **Header** shows the selected quiz mode name (e.g., "Standard Quiz")
- **Notebook list** with checkboxes (multi-select supported): shows all notebooks with word counts for the selected mode
- **"All notebooks"** checkbox to select/deselect all
- **Toggle** for including unstudied words
- **Word count** shows how many words are due for review (SM-2), based on the selected notebooks
- **Start button** begins the quiz
- Back link returns to Quiz Hub

![Vocabulary Quiz Start](/proposals/etymology-learning/wireframes/15-vocabulary-quiz-start.svg)

### Screen Q3: Etymology Quiz Start

Select notebooks and quiz mode before starting an etymology quiz (Breakdown, Assembly, or Freeform).

- **Quiz mode** radio buttons at the top:
  - **Breakdown**: Given a word, type its origins and their meanings
  - **Assembly**: Given origins with meanings, type the word
  - **Freeform**: Type any word and break down its origins
- **Notebook list** with checkboxes (multi-select supported): shows regular notebooks (book, flashcard, story) that have at least one definition with `origin_parts`. Each notebook shows its name and the count of words with etymology data. The system automatically finds the relevant origins from whatever etymology notebooks are loaded — the user does not need to separately select etymology notebooks.
- **"All notebooks"** checkbox to select/deselect all
- **Toggle** for including unstudied words
- **Word count** shows how many words are due for review (SM-2), based on the selected notebooks
- **Start button** begins the quiz
- Back link returns to Quiz Hub

![Etymology Quiz Start](/proposals/etymology-learning/wireframes/04-quiz-start.svg)

### Screen Q4: Quiz Card — Breakdown Mode

The user sees a word and must identify its etymological origins.

- **Progress bar** and counter at the top (same style as existing quizzes)
- **Word card** with the word displayed prominently and its meaning below
- **Input rows** for each origin: two fields per row (origin and meaning), connected by "="
- **"+ Add origin"** link to add more rows (words can have varying numbers of origins)
- **Submit button** fixed at the bottom (same position as existing quizzes)
- Input fields are auto-focused for fast typing

![Quiz Breakdown](/proposals/etymology-learning/wireframes/05-quiz-breakdown.svg)

### Screen Q5: Quiz Card — Assembly Mode

The user sees etymological origins and must type the word they form.

- **Progress bar** and counter at the top
- **Origins card** showing each origin with its meaning and language, connected by "+" signs
- **"=" separator** followed by a single text input for the word
- **Submit button** fixed at the bottom

![Quiz Assembly](/proposals/etymology-learning/wireframes/06-quiz-assembly.svg)

### Screen Q6: Feedback — Correct

Shown after a correct answer in Breakdown or Assembly mode. Provides the full breakdown and related words for exploration.

- **Green banner** with checkmark (same style as existing quiz feedback)
- **Word card** with word and meaning
- **Your answer** section with green border, showing each origin with a checkmark
- **Full breakdown** section showing each origin as a clickable link with its meaning, language badge, and type badge
- **Related words** section grouping other words that share the same origins — encouraging the user to explore further
- **Next review date** based on SM-2 calculation
- **Next button** to proceed

![Feedback Correct](/proposals/etymology-learning/wireframes/07-feedback-correct.svg)

### Screen Q7: Feedback — Incorrect

Shown after an incorrect answer. Highlights what was wrong and shows the correct breakdown.

- **Red banner** with X mark (same style as existing quiz feedback)
- **Word card** with word and meaning
- **Your answer** section with red border, incorrect parts shown with strikethrough and X mark, correct parts with checkmark
- **Correct breakdown** section with clickable origin links, language and type badges
- **Related words** section (same as correct feedback — learning opportunity)
- **Next review date** (shorter interval due to incorrect answer)
- **Next button** to proceed

![Feedback Incorrect](/proposals/etymology-learning/wireframes/08-feedback-incorrect.svg)

### Screen Q8: Freeform Etymology Quiz

Open-ended quiz mode — the user types any word and identifies its origins. Same pattern as the existing Freeform quiz but for etymology. The freeform quiz also requires notebook selection (etymology + definition notebooks), using the same selections as the quiz start screen. The user navigates through the quiz start screen first to select notebooks before entering freeform mode.

- **No progress bar** (open-ended, like existing Freeform quiz)
- **Word input** at the top — user types any word
- **Status indicator** shows whether the word was found in etymology notebooks
- **Origin input rows** below — same layout as Breakdown mode (origin + meaning pairs with "+" to add more)
- **Submit button** fixed at the bottom
- After feedback, buttons to "Next Word" or "See Results" (same as existing Freeform quiz)

![Freeform Etymology](/proposals/etymology-learning/wireframes/10-quiz-freeform.svg)

### Screen Q9: Session Complete

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

### Screen Q10: Notebook Detail — Etymology Links

Existing notebook detail pages (book, flashcard, story) are enhanced to show `origin_parts` on definitions that have them. This integrates etymology data directly into the vocabulary browsing experience without requiring the user to navigate to a separate etymology section.

- Each definition card displays the expression and meaning as usual
- **Definitions with `origin_parts`** show an additional "Origins:" row below the meaning, with each origin rendered as a clickable blue pill/badge
- Each origin pill shows the origin name (blue, clickable) and a language badge (gray)
- Origins are connected by "+" separators to show how the word is composed
- **Tapping an origin pill** navigates to that origin's detail page (Screen L3), where the user can explore all words built from that origin
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

The app uses a **hub-and-spoke** navigation model:

1. **Home** is the top-level hub with three spokes: Books, Learn, Quiz
2. **Learn Hub** is a second-level hub with two spokes: Vocabulary notebooks and Etymology notebooks
3. **Quiz Hub** is a second-level hub with six spokes: three vocabulary quiz modes and three etymology quiz modes
4. The browse flow within etymology is a **graph navigation** — users can traverse between origins through shared words indefinitely
5. Back navigation returns to the parent hub at each level

This replaces the previous flat navigation where all four sections (Books, Notebooks, Quiz, Etymology) were at the same level on the home page. The new structure groups related features together (vocabulary + etymology under Learn; all quiz modes under Quiz) and reduces the number of top-level choices from four to three.

### Route Structure

| Screen | Route | Back link |
|--------|-------|-----------|
| Home | `/` | — |
| Books | `/books` | Home |
| Learn Hub | `/learn` | Home |
| Vocabulary Notebooks | `/notebooks` | Learn |
| Etymology Notebook List | `/etymology` | Learn |
| Origin List | `/notebooks/etymology/{id}` | Etymology |
| Origin Detail | `/notebooks/etymology/{id}?origin=X` | Origin List |
| Quiz Hub | `/quiz` | Home |
| Vocabulary Quiz Start | `/quiz?mode=standard` | Quiz |
| Etymology Quiz Start | `/quiz?mode=etymology` | Quiz |
| Quiz Card (all types) | `/quiz/{type}` | — |
| Session Complete | `/quiz/complete` | Quiz Hub |

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
| Hub-and-spoke navigation | Home page cards | Learn Hub, Quiz Hub |
| Section dividers | Notebook list page | Learn Hub, Quiz Hub |
