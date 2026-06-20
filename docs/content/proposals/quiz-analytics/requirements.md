---
title: "Product Requirements"
weight: 1
---

# Product Requirements

## Overview

Add Analytics to the web UI as two pages:

1. A **day list page** — a reverse-chronological list of days with per-day statistics (totals, wrong / correct counts). No per-word detail.
2. A **day detail page** — opened by tapping a day from the list, showing the words the learner got wrong on that one day, with the surrounding answer history for each word.

The split keeps the top-level page scannable: the learner first chooses *which day to look at* without being overwhelmed by per-word detail, then drills into a single day.

## Problem

Today the only signal a learner gets about a wrong answer is the immediate feedback inside the quiz. Once the session ends, there is no place to see:

- **On which day** the learner had quiz activity and how that day went overall
- Which words were failed on a specific day
- **The recent answer pattern** for each failed word — e.g. three wrong answers in a row (truly weak), or one wrong answer after two correct ones (an unlucky slip)
- Which notebooks or scenes contain the words they struggle with

The raw data already lives in each notebook's `learned_logs` (each record has `status` and `learned_at`), but it is spread across many files and quiz types. The learner needs a time-organized view to ask "what did I get wrong yesterday?" or "what did I get wrong in last week's session?".

## Goals

- Provide a scannable, day-level overview of recent quiz activity
- On selecting a day, show every word the learner got wrong that day with the context needed to act on it
- For each word, show the **recent answer pattern** so the learner can tell a persistent struggle from a one-off slip
- Reuse the existing learning history — no new data collection required

## Non-Goals

- Recommending or auto-starting a focused review session (may follow later)
- Cross-user analytics or leaderboards
- Time-series charts or trend graphs — a day list is enough for the first version
- A separate word-ranked leaderboard view (may follow later as a secondary tab)
- Showing per-word detail and per-day overview on the same page

## Pages

### Page 1: Day List (`/analytics`)

The top-level Analytics page. Shows a reverse-chronological list of days the learner had quiz activity, one row per day, with no per-word breakdown.

### Page 2: Day Detail (`/analytics/{date}`)

Reached by tapping a row in the Day List. Shows the words the learner got wrong on that single day, plus the per-word answer history.

## User Stories

### Browse Day-Level Activity

As a learner, I want to open the Analytics page and see a clean, day-by-day overview of how my recent quiz sessions went, with no per-word detail on this page, so I can quickly pick which day I want to look at more closely.

Each day row shows:

- Date (e.g. `Friday, Jun 5 2026`)
- Total attempted
- Wrong count and wrong-rate (e.g. `4 wrong / 14 attempted (29%)`)
- Notebook count touched that day (e.g. `2 notebooks`)
- Quiz types used that day, as small chips (e.g. `notebook · reverse · etymology_breakdown`)

Days are sorted newest first. Days with no quiz activity are omitted.

### Pick a Day to Dig Into

As a learner, I want to tap a day in the list and land on a detail page that shows me **only** the words I got wrong on that day, with the context I need to act on each one.

The Day Detail page shows, for each wrong word on that day:

- The word
- The notebook and scene it belongs to
- The quiz type for that day (`notebook` / `freeform` / `reverse` / `etymology_breakdown` / `etymology_assembly` / `etymology_freeform`)
- The **streak just before this failure** (e.g. "after 2 correct in a row", "3rd wrong in a row")
- The **recent answer pattern** — last 5 attempts as a glyph row (see [Recent Answer Pattern](#recent-answer-pattern))
- The current learning status after this attempt (`misunderstood` / `understood` / `usable`)

### Expand a Word to See Its Full History

As a learner who wants more than the last 5 attempts, I want to expand a word entry on the Day Detail page to see every attempt on that word, newest first, including the date, quiz type, result, and quality grade.

This expanded view is the per-word history (see [Expanded Word History](#expanded-word-history)).

### Tell a Persistent Struggle From an Unlucky Slip

As a learner, two words wrong on the same day can mean very different things. The Day Detail page must make that visible at a glance.

For example, both of these might appear under `Jun 5 2026`:

- `thrilled` — pattern `✗ ✗ ✗ . .` → 3rd wrong in a row → still not learned
- `excited` — pattern `✓ ✓ ✗ . .` → 1 fresh slip after 2 correct → was learned, probably just needs one more review

### Navigate Between Days

As a learner viewing one Day Detail page, I want to move to the previous or next day with quiz activity without going back to the Day List, so I can scan through recent days quickly.

- "Previous day" and "Next day" links at the top of the Day Detail page
- "Back to all days" link returns to the Day List

### Filter By Time Range

As a learner, I want to focus on recent days rather than scroll past months of history.

Provide a simple time-range filter on the Day List page:

- Last 7 days
- Last 30 days (default)
- Last 90 days
- All time

Filters apply only to the Day List; the Day Detail page always shows the full content of its one day.

### Filter By Notebook

As a learner with multiple notebooks, I want to narrow the Day List and Day Detail to a single notebook so I can prepare for one specific area.

- A notebook selector on the Day List page
- The selected notebook persists into the Day Detail page (the Day Detail page only shows wrong words from the selected notebook)
- Default: all notebooks

### Filter By Quiz Type

As a learner who uses both vocabulary quizzes and etymology quizzes, I want to see wrong answers per quiz type separately, because recognizing a word, producing it, and breaking down its etymology are different skills.

Quiz type selector with a two-level structure:

- `All` (default)
- **Vocabulary**
  - `Notebook` — shows the word, asks for the meaning (`notebook`)
  - `Freeform` — recall both word and meaning from context (`freeform`)
  - `Reverse` — shows the meaning, asks for the word (`reverse`)
- **Etymology**
  - `Breakdown` — shows the word, asks for its etymological parts (`etymology_breakdown`)
  - `Assembly` — shows the parts, asks the learner to assemble the word (`etymology_assembly`)
  - `Etymology Freeform` — open-ended etymology recall (`etymology_freeform`)

The learner can also pick a top-level group (`Vocabulary` or `Etymology`) to include all sub-types in that group.

The filter persists from the Day List into the Day Detail page.

### Jump From a Word to Its Notebook Entry

As a learner, when I see a wrong word on a Day Detail page, I want to click the word and jump to its page in the Learn section, so I can review the meaning, context, and examples in one click.

### Empty State — Day List

As a new learner who has not failed any quiz yet, or whose filters return nothing, I want a clear message in place of the Day List — not a broken-looking empty list.

If the Day List is empty for the selected filters, show: `No quiz activity in this range. Try widening the time range or removing filters.`

### Empty State — Day Detail

When a learner opens a Day Detail page for a day with no wrong answers (i.e. a day with only correct attempts), show a positive empty state: `All correct on this day. {N} attempted.` plus a link back to the Day List.

## Day Detail Page Contents

The Day Detail page lists only **words answered wrong on that day**, regardless of whether the wrong answer came from a vocabulary quiz or an etymology quiz. If the same word was answered wrong more than once on the same day across different quiz types (for example wrong in both `notebook` and `etymology_breakdown`), it appears once per quiz type so each attempt's streak and pattern are visible.

Streak and pattern are computed **per quiz type**, because the underlying learning history is stored per quiz type — getting a word right in `notebook` does not reset the wrong streak in `etymology_breakdown`.

The page header includes the day's overall stats (the same row from the Day List) so the learner keeps the context of "what day am I looking at and how did it go overall".

## Recent Answer Pattern

Each word entry on the Day Detail page shows the last **5 attempts** as a compact glyph sequence, newest on the right:

```
Pattern    Meaning
✗ ✗ ✗ . .  Three wrong in a row, no earlier attempts in range — truly weak
✓ ✓ ✗ . .  One slip after two correct — likely a one-off
✗ ✓ ✗ ✓ ✗  Inconsistent — knows it sometimes, forgets it sometimes
✓ ✓ ✓ ✓ ✗  Fresh failure after a long correct streak — possibly an edge case
```

- `✓` = correct (status was `understood` or `usable`)
- `✗` = wrong (status was `misunderstood`)
- `.` = no attempt (fewer than 5 attempts exist)

In addition, each word entry surfaces two derived counts:

- **Current wrong streak** — number of consecutive wrong attempts ending at this attempt (e.g. "3rd wrong in a row")
- **Last correct streak before this** — number of consecutive correct attempts immediately before the current wrong streak (e.g. "after 2 correct")

## Display Format

### Day List (`/analytics`)

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  Quiz Analytics                                                                │
│  Notebook: [All ▾]   Quiz: [All ▾]   Range: [Last 30 days ▾]                   │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│  Date            Wrong / Total   Rate   Notebooks   Quiz types                 │
│  ──────────────  ─────────────   ────   ─────────   ──────────────────────     │
│  Fri, Jun 5      4 / 14          29%    2           notebook · reverse · etym  │
│  Thu, Jun 4      0 / 8           0%     1           notebook                   │
│  Wed, Jun 3      2 / 11          18%    2           notebook · etym_assembly   │
│  Tue, Jun 2      2 / 9           22%    1           notebook                   │
│  ...                                                                           │
└────────────────────────────────────────────────────────────────────────────────┘
```

Each row is a link to `/analytics/{YYYY-MM-DD}`. Days with `0 wrong` rows are still shown (so the learner sees their "clean" days), but rendered in a muted style.

### Day Detail (`/analytics/{date}`)

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  ◀ Back to all days                                                            │
│                                                                                │
│  Friday, Jun 5 2026                                ◀ Jun 3   |   Jun 7 ▶       │
│  4 wrong / 14 attempted (29%) · 2 notebooks · notebook · reverse · etymology   │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│   ✗ ephemeral    vocabulary           notebook              ✗ ✗ ✗ . .  3rd wrong│
│   ✗ ubiquitous   vocabulary           reverse               ✓ ✗ ✗ . .  after 1✓ │
│   ✗ thrilled     friends / S01E02     notebook              ✓ ✓ ✗ . .  after 2✓ │
│   ✗ telephone    greek-roots          etymology_breakdown   ✗ ✗ . . .  2nd wrong│
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### Expanded Word History

Clicking a word entry on a Day Detail page expands an inline panel showing every attempt for that word, newest first:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  ephemeral — vocabulary                                                        │
│  Status: misunderstood — 3 wrong in a row (notebook quiz)                      │
├────────────────────────────────────────────────────────────────────────────────┤
│  Date          Quiz                  Result      Quality   Streak before       │
│  ───────────   ───────────────────   ─────────   ───────   ──────────────      │
│  Jun 05 2026   notebook              ✗ wrong     1         2 wrong in a row    │
│  May 30 2026   notebook              ✗ wrong     1         1 wrong in a row    │
│  May 27 2026   notebook              ✗ wrong     1         (first attempt)     │
│  May 20 2026   notebook              ✓ correct   4         —                   │
│  May 18 2026   etymology_breakdown   ✓ correct   5         —                   │
│  ...                                                                           │
└────────────────────────────────────────────────────────────────────────────────┘
```

## Entry Point

- Add an "Analytics" tile to the home page alongside "Learn" and "Quiz" — links to the Day List
- Add a link from the quiz completion screen: "Review what you got wrong today" — links directly to the Day Detail page for today

## Out of Scope

- Editing or deleting learning history from these pages
- Exporting analytics (CSV, PDF)
- Per-day or per-week charts and trend graphs
- A separate word-ranked leaderboard view
- Suggested-words recommendation engine
- Showing per-word detail and per-day overview on the same page
