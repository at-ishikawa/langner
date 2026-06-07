---
title: "UI/UX Design"
weight: 2
---

# UI/UX Design

Mobile-first design (375px viewport) for the Quiz Analytics feature. Analytics is split across two pages — a **Day List** at `/analytics` and a **Day Detail** at `/analytics/{date}`. The two pages share a header, filter bar, and the same visual language as the rest of the app.

## Screen Flow

```
                        ┌──── tap day ────▶ /analytics/{date} ──tap word──▶ /learn/{notebook}/{word}
                        │                          │
Home ──"Analytics" tile──▶ /analytics              │
                        ▲                          │
Quiz Complete ──"Review wrong today"───────────────┘ (deep link to today)
                                                   │
                                                   └── ◀ Back to all days ──▶ /analytics
```

- The Day List is the only landing page for Analytics
- The Day Detail page is always reached **from** the Day List (or from a deep link such as the Quiz Complete screen)
- The Day Detail page never shows per-day overview content for other days; only navigation arrows to the adjacent days with quiz activity

## Page 1: Day List (`/analytics`)

The day-level overview. No per-word detail on this page.

### Screen D1: Day List — Empty State

Shown when no quiz activity matches the current filters.

```
┌─────────────────────────────────────┐
│ ◀ Analytics                         │
│                                     │
│ Notebook: [All ▾]    Range: [30d ▾] │
│ Quiz:     [All ▾]                   │
│─────────────────────────────────────│
│                                     │
│            ┌───────┐                │
│            │  📊   │                │
│            └───────┘                │
│                                     │
│   No quiz activity in this range.   │
│                                     │
│  Try widening the time range or     │
│         removing filters.           │
│                                     │
│        [ Start a quiz ]             │
└─────────────────────────────────────┘
```

### Screen D2: Day List — Populated

```
┌─────────────────────────────────────┐
│ ◀ Analytics                         │
│                                     │
│ Notebook: [All ▾]    Range: [30d ▾] │
│ Quiz:     [All ▾]                   │
│─────────────────────────────────────│
│                                     │
│  ┌───────────────────────────────┐  │
│  │ Fri, Jun 5         4 wrong /  │  │
│  │                    14 (29%) ▶ │  │
│  │ 2 notebooks                   │  │
│  │ notebook · reverse · etym     │  │
│  └───────────────────────────────┘  │
│  ┌───────────────────────────────┐  │
│  │ Thu, Jun 4         0 wrong /  │  │
│  │                    8 (0%)   ▶ │  │
│  │ 1 notebook                    │  │
│  │ notebook                      │  │
│  └───────────────────────────────┘  │
│  ┌───────────────────────────────┐  │
│  │ Wed, Jun 3         2 wrong /  │  │
│  │                    11 (18%) ▶ │  │
│  │ 2 notebooks                   │  │
│  │ notebook · etym_assembly      │  │
│  └───────────────────────────────┘  │
│  ...                                │
└─────────────────────────────────────┘
```

- **Header**: back arrow, title "Analytics"
- **Filter bar**: Notebook / Range / Quiz — same component as the existing notebook list page
- **Day card**: each card is the whole tap target and links to `/analytics/{YYYY-MM-DD}`
  - Top row: date (bold) on the left; `wrong / total (rate%)` on the right with `▶` chevron
  - Middle row: notebook count
  - Bottom row: quiz type chips, truncated with `…` if more than three
- Cards for days with `0 wrong` are rendered in muted text so they recede visually but still show overall activity
- Infinite scroll: more day cards load as the user reaches the last visible one
- Filter state is reflected in the URL query string (`?range=30d&notebook=vocabulary&quiz=notebook`) so the view is shareable and survives a refresh

## Page 2: Day Detail (`/analytics/{date}`)

The per-day detail. Lists the wrong words for that day. No information about other days.

### Screen D3: Day Detail — All Correct (Empty Wrong-Word List)

Shown when the learner visits a day that had only correct attempts.

```
┌─────────────────────────────────────┐
│ ◀ Back to all days                  │
│                                     │
│ Thursday, Jun 4 2026                │
│         ◀ Jun 3   |   Jun 5 ▶       │
│ 0 wrong / 8 attempted (0%)          │
│ 1 notebook · notebook               │
│─────────────────────────────────────│
│                                     │
│            ┌───────┐                │
│            │  🎉   │                │
│            └───────┘                │
│                                     │
│   All correct on this day!          │
│   8 words attempted, 0 wrong.       │
│                                     │
│       [ Back to all days ]          │
└─────────────────────────────────────┘
```

### Screen D4: Day Detail — With Wrong Words

```
┌─────────────────────────────────────┐
│ ◀ Back to all days                  │
│                                     │
│ Friday, Jun 5 2026                  │
│         ◀ Jun 3   |   Jun 7 ▶       │
│ 4 wrong / 14 attempted (29%)        │
│ 2 notebooks · notebook · reverse    │
│              · etymology            │
│─────────────────────────────────────│
│                                     │
│  ┌───────────────────────────────┐  │
│  │ ✗ ephemeral                   │  │
│  │   vocabulary                  │  │
│  │   notebook    ✗ ✗ ✗ . .       │  │
│  │   3rd wrong in a row          │  │
│  └───────────────────────────────┘  │
│  ┌───────────────────────────────┐  │
│  │ ✗ ubiquitous                  │  │
│  │   vocabulary                  │  │
│  │   reverse     ✓ ✗ ✗ . .       │  │
│  │   after 1 correct             │  │
│  └───────────────────────────────┘  │
│  ┌───────────────────────────────┐  │
│  │ ✗ thrilled                    │  │
│  │   friends / S01E02            │  │
│  │   notebook    ✓ ✓ ✗ . .       │  │
│  │   after 2 correct             │  │
│  └───────────────────────────────┘  │
│  ┌───────────────────────────────┐  │
│  │ ✗ telephone                   │  │
│  │   greek-roots                 │  │
│  │   etymology   ✗ ✗ . . .       │  │
│  │   breakdown                   │  │
│  │   2nd wrong in a row          │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

- **Header**: `◀ Back to all days` link (to `/analytics`, preserving filter query string)
- **Day header**: same content shape as a Day List card but full-bleed, with prev/next day links to the adjacent days that have quiz activity (skipping empty days)
- **Word card**: one card per (word × quiz type) attempt, vertically stacked
  - Line 1: ✗ icon + word in bold
  - Line 2: notebook (and scene if any) in muted text
  - Line 3: quiz type chip + recent-pattern glyph row, right-aligned
  - Line 4: streak summary in small italic muted text
- Tapping a word card opens the expanded inline panel (Screen D5)
- Tapping the word title (not the whole card) navigates to `/learn/{notebook}/{word}`

### Screen D5: Word — Expanded Inline Panel

When the user taps a word card on the Day Detail page, the card expands in place to reveal the full attempt history. Tapping again collapses it. Only one panel is expanded at a time.

```
│  ┌───────────────────────────────┐  │
│  │ ✗ ephemeral             [×]   │  │
│  │   vocabulary                  │  │
│  │   notebook quiz               │  │
│  │   3 wrong in a row            │  │
│  │ ─────────────────────────     │  │
│  │ Jun 05  notebook   ✗  Q1      │  │
│  │         after 2 wrong         │  │
│  │ May 30  notebook   ✗  Q1      │  │
│  │         after 1 wrong         │  │
│  │ May 27  notebook   ✗  Q1      │  │
│  │         (first attempt)       │  │
│  │ May 20  notebook   ✓  Q4      │  │
│  │ May 18  etymology  ✓  Q5      │  │
│  │         breakdown             │  │
│  │ ─────────────────────────     │  │
│  │ [ Open in Learn → ]           │  │
│  └───────────────────────────────┘  │
```

- Header now includes a close button `[×]`
- Status line summarizes the current streak for this quiz type
- Attempt list (newest first): date, quiz type chip, result icon, quality grade
- A muted line below each wrong attempt shows the streak before it
- A bottom CTA links to `/learn/{notebook}/{word}`

## Interaction Details

### Filters (Day List page)

- **Notebook** dropdown: lists every notebook the learner has at least one attempt in, plus `All` (default)
- **Range** dropdown: `Last 7 days` / `Last 30 days` (default) / `Last 90 days` / `All time`
- **Quiz** dropdown is grouped:
  ```
  All ✓
  ──── Vocabulary ────
    Notebook
    Freeform
    Reverse
  ──── Etymology ────
    Breakdown
    Assembly
    Etymology Freeform
  ```
  Tapping a group header (Vocabulary / Etymology) selects all sub-items in that group.

### Filter Persistence Across Pages

- Filter state lives in the URL query string on both pages
- When the learner taps a Day card on the Day List, the same `notebook` and `quiz` query params are carried into `/analytics/{date}?notebook=…&quiz=…` so the Day Detail page only shows wrong words matching those filters
- The `range` filter is **not** carried to the Day Detail page (it's a single day) but is preserved on the back link `◀ Back to all days`

### Day Navigation (Day Detail page)

- The `◀ Prev | Next ▶` links jump to the adjacent days **with quiz activity matching the current filters**
- Disabled (greyed) when no such adjacent day exists
- Keyboard shortcut: `← / →` arrow keys trigger the same navigation

### Recent Answer Pattern Glyph

The 5-glyph pattern row uses the same icon set as the result icon on each card:

| Glyph | Color (light) | Color (dark) | Meaning |
|-------|---------------|--------------|---------|
| `✓` | green.600 | green.300 | Correct |
| `✗` | red.600 | red.300 | Wrong |
| `·` | gray.400 | gray.500 | No attempt |

Glyphs are equal-width and separated by a hair space so the row aligns across cards.

### Navigation Targets

- Back arrow on Day List → home (`/`)
- Back link on Day Detail → `/analytics` (with the same filters preserved)
- Tapping a word title on Day Detail → `/learn/{notebook}/{word}`
- Tapping a Day card on Day List → `/analytics/{YYYY-MM-DD}` (with `notebook` and `quiz` preserved)

## Entry Points

### Home Page Tile

Add a third tile alongside Learn and Quiz on the home page (`/`):

```
┌───────────────────────────────────┐
│  [A]  Analytics                ›  │
│       Review your quiz activity   │
└───────────────────────────────────┘
```

Icon style matches the existing `[L]` Learn and `[Q]` Quiz tiles. The tile links to the Day List.

### Quiz Complete Screen

After a quiz session ends, the existing Session Complete screen gains a secondary link below the result summary:

```
[ Back to Start ]
[ Review what you got wrong today → ]
```

The link is only shown when the session had at least one wrong answer. It navigates directly to `/analytics/{today}` (Day Detail for today), skipping the Day List, so the just-completed session is visible immediately.

## Responsive Behavior

| Breakpoint | Layout |
|------------|--------|
| < 768px (mobile) | Single column. Filter bar wraps to two rows on the Day List. Cards full width on both pages. |
| ≥ 768px (tablet/desktop) | Single column max-width 720px, centered. Filter bar single row. Cards keep the same vertical layout. |

A horizontal-table desktop view is **out of scope** for the first version to keep the same layout across screen sizes.

## Component Inventory

| Component | Page | New / Reused | Notes |
|-----------|------|--------------|-------|
| `AppHeader` | Both | Reused | Back arrow + title |
| `FilterBar` | Day List | New | Three dropdowns; controlled by URL query string |
| `DayCard` | Day List | New | One row in the Day List; whole card is a link |
| `DayHeader` | Day Detail | New | Repeats the Day Card content + prev/next day arrows |
| `WrongWordCard` | Day Detail | New | Collapsed view of one (word × quiz type) attempt |
| `WordHistoryPanel` | Day Detail | New | Expanded inline panel with full attempt list |
| `PatternGlyphs` | Day Detail | New | Shared 5-glyph row used in card and expanded view |
| `QuizTypeChip` | Both | New | Small pill showing the quiz type (`notebook`, `etymology_breakdown`, …) |
| `EmptyState` | Both | Reused (if present) or New | Centered illustration + headline + CTA |

## Accessibility

- Day cards and word cards are `button` / `a` elements so they are keyboard-focusable; Enter activates the link, Space toggles expand on word cards
- The pattern row has an `aria-label` describing the sequence ("3 wrong, 1 no attempt, 1 no attempt" etc.) so screen readers do not just hear "✗✗✗··"
- The `✓` / `✗` glyphs are paired with text ("correct" / "wrong") in the expanded view so result is never color-only
- Day headers are `h2`; word names on the Day Detail page are `h3`
- The Day Detail prev/next links use `aria-label="Previous day with activity"` / `aria-label="Next day with activity"`

## Out of Scope for the Design

- Horizontal table view for desktop (everything stays vertical)
- Charts, sparklines, or trend visualizations
- Bulk actions (e.g. "add all wrong words from this day to a focused review")
- Inline editing of quiz history from the analytics pages
- A single page that mixes per-day overview with per-word detail
