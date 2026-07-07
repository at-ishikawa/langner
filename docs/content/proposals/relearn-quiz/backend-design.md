---
title: "Backend & Data Model"
weight: 3
---

# Backend & Data Model

## Overview

Backend design for the Relearn Quiz — a practice-only quiz whose defining property is that it **writes nothing to learning history**. It reuses the existing pure OpenAI graders and the existing pool-selection logic that Quiz Analytics already uses to find "wrong words", but it bypasses every `Save*` path, so SM-2 intervals, easiness factors, and the Quiz Analytics view are untouched.

This document is grounded in the current code. All type/function references are to real symbols in `backend/`.

### Goals

- Select the pool as **every word whose most-recent in-window learning log has status `misunderstood`**, across all quiz types, mirroring how analytics selects wrong words — on both the DB (`learning_logs`) and YAML-only paths.
- Grade answers with the **existing pure graders** `GradeNotebookAnswer` and `GradeEtymologyStandardAnswer`.
- Persist **nothing** to learning history: the submit path calls only the `Grade*` methods and **never** `SaveResult` / `SaveReverseResult` / `SaveFreeformResult` / `SaveEtymologyOriginResult` / `learningRepository.Create` / `UpdateLog` / any etymology YAML write / `GetLatestLearnedInfo`.
- Keep already-recovered words out of the next Relearn session via a lightweight, non-SR **"relearn clears"** marker — a new `relearn_clears` table when a DB is present, or an in-memory map otherwise. It is explicitly **not** a learning log.
- Return a rich response carrying the result card **plus** context scenes, `graph_context`, and `example_words`, but deliberately **no** `next_review_date` / `learned_at`.

### Non-Goals

- Any change to SM-2, to the `learning_logs` schema, or to how existing quizzes record history.
- Server-persisted, resumable sessions. The loop is client-driven (see [Frontend Design]({{< relref "frontend-design" >}})); the backend is stateless about Relearn sessions.
- Surfacing Relearn activity anywhere in analytics.

## Relationship to the Learning-History Invariants

The repo's learning-history invariants (L1–L4) govern every path that **writes, reads, or displays** a quiz *learning record*. The Relearn Quiz is deliberately outside that system: it produces no learning record at all. The invariants are respected precisely by **not** participating — the submit handler never reaches `SaveResult` or `GetLatestLearnedInfo`, so there is no canonical-key, symmetric-read, or cross-notebook concern to get wrong. The one thing the Relearn Quiz *does* persist — the relearn-clear marker — is defined below as explicitly not a learning log and is never read by SM-2 or analytics.

## Word Pool Selection

The pool is "every word whose most-recent in-window log status is `misunderstood`, minus words already cleared more recently". This reuses the analytics definition of wrong: in `backend/internal/notebook/types.go`, "IsWrong is true when the attempt was marked misunderstood. Anything else (understood, usable, intuitive) counts as correct" — status constants live in `backend/internal/notebook/notebook.go:33-37` (`LearnedStatusMisunderstood = "misunderstood"`, `LearnedStatusUnderstood = "understood"`, `LearnedStatusCanBeUsed = "usable"`).

Selection runs on the same two backends the rest of the app uses. Per `backend/cmd/langner-server/main.go:93-137`, YAML is the source of truth and the DB is a secondary mirror enabled only when `Database.Host` and `Database.Password` are set.

### DB path (`learning_logs`)

The analytics DB repo already selects wrong rows with `WHERE ll.status = 'misunderstood'` and computes most-recent status via `recentAttempts` ordered `learned_at DESC` (`backend/internal/analytics/db_repository.go:128-260`). The Relearn pool applies the same idea over a rolling window using Postgres `DISTINCT ON` (the codebase runs on pgx/Postgres):

```sql
-- $1 = window start (now - window_hours)
WITH latest AS (
  SELECT DISTINCT ON (note_id, quiz_type)
         note_id, quiz_type, status, learned_at
  FROM learning_logs
  WHERE learned_at >= $1
  ORDER BY note_id, quiz_type, learned_at DESC   -- most-recent-in-window per series
)
SELECT DISTINCT ON (l.note_id) l.note_id, l.quiz_type, l.learned_at
FROM latest l
WHERE l.status = 'misunderstood'
  AND NOT EXISTS (
    SELECT 1 FROM relearn_clears rc
    WHERE rc.note_id = l.note_id
      AND rc.cleared_at > l.learned_at            -- cleared after the last wrong log
  )
ORDER BY l.note_id, l.learned_at DESC;            -- de-dup to one card per note
```

- The inner `latest` CTE picks the most-recent in-window log **per `(note_id, quiz_type)` series**, so a word answered correctly at 11am after being wrong at 9am (in a real quiz) is excluded — its latest log is `understood`.
- The outer `DISTINCT ON (note_id)` collapses a word that is wrong in multiple quiz types down to a single card, keeping the most-recent wrong quiz type for the "source" label.
- The `NOT EXISTS` clause drops words whose relearn-clear marker is newer than their last wrong log (see below).

Word text and metadata are joined from `notes` exactly as the analytics query does (`JOIN notes n ON n.id = ll.note_id`, selecting `n."usage"`).

### YAML-only path

The analytics YAML repo (`backend/internal/analytics/yaml_repository.go`) already reads on-disk learning history, computes `IsWrong: rec.Status == notebook.LearnedStatusMisunderstood` (`:128`), and resolves per-word metadata via `NotebookMetadataResolver` (`notebook_resolver.go`). The Relearn pool reuses this: for each expression's per-quiz-type log series, take the most recent log within the window; include the word if that log is `misunderstood`; exclude it if the in-memory relearn-clear time for its note is newer. De-duplicate to one card per expression.

Because both paths share the same "most-recent-in-window `misunderstood`" rule, the two implementations must apply that rule from **one selector** used by both — the same discipline the invariants require of read/write symmetry — even though neither writes a log here.

## "Relearn Clears" Marker

A Relearn attempt records no learning log, so a correctly-answered word's most-recent *real* log stays `misunderstood` and it would reappear in the next Relearn pool that day. The relearn-clear marker prevents that, without being a learning record.

### New table (DB present) — migration `017`

Migrations use golang-migrate with `NNN_description.up.sql` / `.down.sql`, 3-digit sequential; the current head is `016`, so this is `017` (`backend/schemas/migrations/`). The DB runner applies `m.Up()` on startup (`backend/internal/database/db.go:99-109`).

`017_add_relearn_clears.up.sql`:

```sql
CREATE TABLE relearn_clears (
    note_id BIGINT NOT NULL,
    cleared_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (note_id),
    FOREIGN KEY (note_id) REFERENCES notes(id)
);
CREATE INDEX idx_relearn_clears_cleared_at ON relearn_clears (cleared_at);
```

`017_add_relearn_clears.down.sql`:

```sql
DROP TABLE IF EXISTS relearn_clears;
```

Design choices that keep this from being a learning log:

- **One row per note**, keyed by `note_id`; a new clear upserts `cleared_at` to `now()`. There is no history of clears — only the latest matters for pool suppression.
- It has **no `quiz_type`, no `status`, no `quality`, no `interval_days`, no `easiness_factor`**. It is structurally incapable of feeding SM-2.
- Nothing reads this table except the Relearn pool query's `NOT EXISTS` clause. Neither the SM-2 write path (`learning.LearningRepository`) nor the analytics repos reference it.

The upsert:

```sql
INSERT INTO relearn_clears (note_id, cleared_at) VALUES ($1, $2)
ON CONFLICT (note_id) DO UPDATE SET cleared_at = EXCLUDED.cleared_at;
```

### In-memory map (YAML-only)

When no DB is configured, relearn clears live in a process-local `map[int64]time.Time` (note_id → latest cleared_at), guarded by a mutex, held on the handler (alongside its existing in-memory card stores). It is **never** written to any YAML learning-history file — writing it there would make it a learning record and violate the whole point. It is naturally ephemeral: a process restart clears it, at which point words age out of the pool on their own via the rolling window.

A tiny interface lets the handler treat both the same way:

```go
// backend/internal/quiz/relearn_clears.go
type RelearnClears interface {
    // MarkCleared upserts the latest clear time for a note.
    MarkCleared(ctx context.Context, noteID int64, at time.Time) error
    // ClearedAfter returns note IDs whose clear time is strictly after the given time.
    // (Used by the pool selector; on the DB path this is folded into the SQL instead.)
    IsClearedSince(ctx context.Context, noteID int64, since time.Time) (bool, error)
}
```

`DBRelearnClears` runs the upsert / `NOT EXISTS`; `MemoryRelearnClears` reads and writes the map. Selection is wired the same way as `learningRepo` in `main.go` — DB implementation when host+password are set, memory otherwise.

## Proto API

Add to `proto/api/v1/quiz.proto`. The `QuizService` currently has the RPCs listed in `quiz.proto:25-46`; add three:

```proto
service QuizService {
  // ... existing RPCs ...
  rpc StartRelearnQuiz(StartRelearnQuizRequest) returns (StartRelearnQuizResponse);
  rpc SubmitRelearnAnswer(SubmitRelearnAnswerRequest) returns (SubmitRelearnAnswerResponse);
  rpc BatchSubmitRelearnAnswers(BatchSubmitRelearnAnswersRequest) returns (BatchSubmitRelearnAnswersResponse);
}
```

### Label-only enum value

The existing `QuizType` enum (`quiz.proto:9-17`) ends at `QUIZ_TYPE_ETYMOLOGY_FREEFORM = 6`. Add:

```proto
enum QuizType {
  // ... existing values 0..6 ...
  QUIZ_TYPE_RELEARN = 7;  // UI label only — NEVER written to learning_logs or YAML
}
```

`QUIZ_TYPE_RELEARN` exists so the frontend can label the mode. It is never assigned to a `learning_logs.quiz_type` value and never written to a YAML `quiz_type` field — the submit handler never constructs a `learning.LearningLog` at all.

### Start

```proto
message StartRelearnQuizRequest {
  // Look-back window in hours. Default 24 when unset; clamped to [1, 168].
  int32 window_hours = 1 [(buf.validate.field).int32 = {gte: 1, lte: 168}];
}

message StartRelearnQuizResponse {
  repeated RelearnCard cards = 1;
}

message RelearnCard {
  int64 note_id = 1;
  string entry = 2;               // the expression shown; the meaning-grading key
  QuizType source_quiz_type = 3;  // where the most-recent wrong answer came from (label only)
  repeated Example examples = 4;  // reuses the existing Example message
}
```

Note on validation: the proto set has no existing two-sided ranged-int precedent (the only ranged int today is one-sided `int32.gte = 0` at `analytics.proto:40`). The `{gte: 1, lte: 168}` form above is the natural expression of a clamp; because unset/`0` must map to the default 24, the handler also normalizes in code:

```go
window := req.Msg.GetWindowHours()
if window == 0 { window = 24 }
if window < 1 { window = 1 } else if window > 168 { window = 168 }
```

### Submit

The response mirrors `SubmitAnswerResponse` (`quiz.proto:165-176`) for the result card, adds the context payload, and **omits `next_review_date` and `learned_at` entirely** — they are not "left blank", they are not fields on this message, so there is nothing for the client to render or gate an override on.

```proto
message SubmitRelearnAnswerRequest {
  int64 note_id = 1 [(buf.validate.field).int64.gt = 0];
  string answer = 2;
  int64 response_time_ms = 3;
  bool is_skipped = 4;
}

message SubmitRelearnAnswerResponse {
  bool correct = 1;
  string meaning = 2;
  string reason = 3;
  WordDetail word_detail = 4;
  repeated string images = 5;

  // Rich Learn-page context (see Frontend Design):
  repeated RelearnContextScene context_scenes = 6;  // conversations/statements the word appears in
  GraphPrompt graph_context = 7;                     // reuse of the etymology relation graph
  repeated string example_words = 8;                 // related words sharing the origin

  // Deliberately NO next_review_date and NO learned_at.
}

message RelearnContextScene {
  string notebook_name = 1;
  string scene_title = 2;
  repeated string statements = 3;   // prose lines containing the expression
  repeated ConversationLine conversations = 4;
}

message ConversationLine {
  string speaker = 1;
  string quote = 2;
}

message BatchSubmitRelearnAnswersRequest {
  repeated SubmitRelearnAnswerRequest answers = 1 [(buf.validate.field).repeated.min_items = 1];
}
message BatchSubmitRelearnAnswersResponse {
  repeated SubmitRelearnAnswerResponse responses = 1;
}
```

`GraphPrompt`, `WordDetail`, and `Example` are existing messages reused as-is. The batch variant exists for parity with the other quiz modes, though the client-driven loop primarily uses the single-shot `SubmitRelearnAnswer` (one card resolved at a time — see [Frontend Design]({{< relref "frontend-design" >}})).

## Handler Flow

The `QuizHandler` (`backend/internal/server/quiz_handler.go:25-39`) already holds per-session in-memory card stores (`noteStore`, `reverseStore`, `etymologyOriginStore`, …) plus `nextID` and a mutex. Add a `relearnStore map[int64]quiz.RelearnCard` and a `RelearnClears` dependency.

### StartRelearnQuiz

1. Normalize/clamp `window_hours`.
2. Ask the pool selector (DB or YAML) for candidate `(note_id, source_quiz_type, entry)` rows within the window whose most-recent log is `misunderstood` and that are not more-recently cleared.
3. Build a `RelearnCard` per row (loading examples/meaning/etymology metadata for grading and context), store it in `relearnStore` keyed by `note_id`, and return the cards.

### SubmitRelearnAnswer — the critical path

The existing standard handler pattern is **validate → look up card → Grade → Save → GetLatestLearnedInfo → respond** (`quiz_handler.go:117-139`). The Relearn handler uses the **same first three steps and then stops before any Save/read-back**:

```go
func (h *QuizHandler) SubmitRelearnAnswer(ctx, req) (*SubmitRelearnAnswerResponse, error) {
    validateRequest(req.Msg)
    card := h.relearnStore[req.Msg.GetNoteId()]   // under h.mu; NotFound if missing

    var grade quiz.GradeResult
    switch {
    case req.Msg.GetIsSkipped():
        grade = skippedGradeResult()              // reuse quiz_handler_batch.go:22-29
    case card.IsEtymologyOrigin:
        // etymology-origin words graded by the etymology meaning grader
        grade, err = h.svc.GradeEtymologyStandardAnswer(ctx, card.AsEtymologyOriginCard(),
            req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
    default:
        // all other words graded by the notebook meaning grader
        grade, err = h.svc.GradeNotebookAnswer(ctx, card.AsCard(),
            req.Msg.GetAnswer(), req.Msg.GetResponseTimeMs())
    }

    // Record NOTHING to learning history. The ONLY persistence is the non-SR clear marker.
    if grade.Correct {
        _ = h.relearnClears.MarkCleared(ctx, card.NoteID, time.Now())
    }

    return connect.NewResponse(&apiv1.SubmitRelearnAnswerResponse{
        Correct:       grade.Correct,
        Meaning:       card.Meaning,
        Reason:        grade.Reason,
        WordDetail:    toProtoWordDetail(card.WordDetail),
        Images:        card.Images,
        ContextScenes: buildRelearnContext(card),   // conversations/statements
        GraphContext:  card.GraphContext,            // etymology graph (nil if none)
        ExampleWords:  card.ExampleWords,            // related words (empty if none)
        // no NextReviewDate, no LearnedAt — not fields on this message
    }), nil
}
```

Guarantees this path upholds:

- It calls **only** `GradeNotebookAnswer` / `GradeEtymologyStandardAnswer` (both pure — `service.go:464`, `etymology.go:159` — they call `AnswerMeanings` / `ValidateWordForm` and return `GradeResult{Correct, Reason, Quality, Classification}` with no repository access).
- It **never** calls `SaveResult` / `SaveReverseResult` / `SaveFreeformResult` / `SaveEtymologyOriginResult`, so `learningRepository.Create` and the etymology YAML write are never reached.
- It **never** calls `GetLatestLearnedInfo`, so no `learned_at` / `next_review_date` is computed or returned.
- The only write is `relearn_clears`, which no SM-2 or analytics code reads.

`grade.Quality` is computed by the graders but simply **discarded** here — it exists only so we could reuse the grader unchanged; nothing consumes it.

### Grading source selection

Whether a word is graded by the etymology grader or the notebook grader is decided by the card metadata captured at `StartRelearnQuiz` time (`IsEtymologyOrigin`, derived from the word's `source_quiz_type` / notebook kind), not by anything the client sends — keeping the grading rule server-side and in one place.

### Context assembly

`buildRelearnContext(card)` and the `graph_context` / `example_words` fields are populated from the same notebook and etymology data the app already loads:

- **Conversations/statements** come from the word's scene data (the same source the reverse quiz uses to fill `SubmitReverseAnswerResponse.contexts`), shaped into `RelearnContextScene` so the frontend can highlight the expression.
- **`example_words` and `graph_context`** are exactly what etymology cards/responses already carry (e.g. `SubmitEtymologyStandardAnswerResponse.graph_context` / `example_words` at `quiz.proto:483-503`); for non-etymology words they are empty/absent.

Because this context is assembled from read-only notebook data and returned in the response, it involves no learning-history access and cannot affect SM-2 or analytics.

## Files to Modify / Add

```
proto/api/v1/quiz.proto                                   # ADD RPCs, RelearnCard, Submit/Start messages, QUIZ_TYPE_RELEARN
backend/schemas/migrations/017_add_relearn_clears.up.sql  # NEW
backend/schemas/migrations/017_add_relearn_clears.down.sql# NEW
backend/internal/quiz/relearn.go                          # NEW: RelearnCard, pool selector, context builder
backend/internal/quiz/relearn_clears.go                   # NEW: RelearnClears interface + DB & memory impls
backend/internal/server/quiz_handler_relearn.go           # NEW: Start/Submit/Batch handlers
backend/internal/server/quiz_handler.go                   # MODIFY: add relearnStore + relearnClears to QuizHandler
backend/cmd/langner-server/main.go                        # MODIFY: wire RelearnClears (DB vs memory), like learningRepo
backend/cmd/langner/*                                     # MODIFY (optional): a CLI relearn entrypoint for parity
```

No changes to `learning_logs`, to `learning.LearningRepository`, to the SM-2 `IntervalCalculator`, or to any analytics code — the Relearn Quiz sits entirely alongside them.

## Testing

- **Pool selection (DB)**: seed `learning_logs` with a word wrong then later correct → excluded; wrong only → included; wrong in two quiz types → one card; a `relearn_clears` row newer than the last wrong log → excluded; older → included. Uses the live-DB integration test harness the repo already has for the Postgres path.
- **Pool selection (YAML)**: same cases over on-disk history + the in-memory clears map.
- **No-write guarantee**: after a full Relearn session (mix of correct/wrong/skip), assert `learning_logs` row count is unchanged, no YAML learning-history file mtime changed, and analytics `DayDetail`/`DailySummaries` for the day are identical to before. This is the load-bearing test for the feature's core promise.
- **Clear marker is not a log**: assert a cleared word has a `relearn_clears` row but **no** new `learning_logs` row, and that its `next_review_date` from a subsequent real quiz is unaffected.
- **Grading dispatch**: an etymology-origin word routes to `GradeEtymologyStandardAnswer`; a notebook word routes to `GradeNotebookAnswer`; a skip yields `skippedGradeResult()` and re-queues client-side.
