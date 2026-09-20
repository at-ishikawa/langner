---
title: "Design"
weight: 1
---

# Per-user notebooks: user-authored content, pushed via CLI

Status: design proposal (no code in this doc)
Audience: langner maintainers
Companion doc: [CLI Authentication (Device Flow)]({{< relref "../cli-authentication/design" >}}) — the unified **bearer access-token** auth (web + CLI) that `notebooks push` authenticates with. This doc references that flow and does not re-specify it.

---

## 1. Goal and fixed decisions

Let each signed-in user manage and quiz **their own** notebooks, on top of the shared global catalog that ships with the app. The agreed, non-negotiable decisions this design is built around:

- **No in-app editor.** Users are not engineers and cannot SSH. Power users author notebooks with their own tools (their editor, their YAML) and deliver them through an **authenticated CLI push**: `langner notebooks push`. Authoring ergonomics stay entirely client-side.
- **The web app owns notebook IDs.** A new user notebook gets a **server-minted opaque `nb_…` id**. Existing production notebook ids are **unchanged** — they are already the primary keys in prod DBs and on-disk `index.yml` files; nothing migrates them.
- **New user notebooks default to PRIVATE.** Pushing writes a `private` ownership row. The existing legacy rule is preserved: a notebook **absent** from the ownership table is **public**.
- **Content lives in the DB.** The target host is Vercel — no writable/persistent filesystem for uploads. User content must be DB-backed, coexisting with the shipped filesystem catalog.

---

## 2. How notebook content loads today (filesystem readers)

### 2.1 A notebook id = the `id:` in `index.yml`

Every notebook is a directory containing an `index.yml`. The parsed struct is `notebook.Index` (`backend/internal/notebook/textbook.go`):

```go
type Index struct {
    Path          string   `yaml:"-"`
    IsBook        bool     `yaml:"-"`
    Kind          string   `yaml:"kind"`
    ID            string   `yaml:"id"`
    Name          string   `yaml:"name"`
    NotebookPaths []string `yaml:"notebooks"`
    Notebooks     [][]StoryNotebook `yaml:"-"`
}
```

`Index.ID` (the `id:` field) is the notebook id used **everywhere**: it keys the reader's in-memory maps, it is the `source_notebook_id` on `learning_logs`, the `notebook_id` on `notebook_notes`, and the argument to every quiz/detail RPC. It is a **single, flat, globally-unique namespace** shared by all users — there is exactly one catalog today.

### 2.2 The Reader walks configured directories

`notebook.NewReader(...)` (`backend/internal/notebook/flashcard.go:79`) builds the read model by walking the filesystem. `walkIndexFiles[T]` (same file) `filepath.Walk`s each root directory, loads every `index.yml`, and registers it by `v.ID`:

```go
indexMap[v.ID] = any(*v).(T)   // story / book indexes
indexMap[v.ID] = any(*v).(T)   // flashcard indexes
```

Etymology indexes are pulled out via `walkEtymologyIndexFiles`; definitions are parsed by `NewDefinitionsMap(definitionsDirectories)`. The reader then exposes id-keyed reads:

- `Reader.ReadStoryNotebooks(storyID)` — `story.go:44`
- `Reader.ReadAllNotes(storyID, histories)` — `flashcard.go:298`
- `Reader.ReadFlashcardNotebooks(flashcardID)` — `flashcard.go:344`
- `Reader.GetDefinitionsNotes(bookID)` — `definitions.go:340`
- `Reader.ReadEtymologyNotebook(etymologyID)` — `etymology.go:250`

### 2.3 Where the directory list comes from

`config.NotebooksConfig` (`backend/internal/config/config.go:105`) carries one string-slice per notebook family:

```
stories_directories, journals_directories, flashcards_directories,
books_directories, definitions_directories, etymology_directories,
grammars_directories        (+ base_directory, learning_notes_directory)
```

`config.example.yml` wires these. The quiz service assembles the reader from exactly this config in `quiz.Service.newReader()` (`backend/internal/quiz/service.go:255`), which also calls `reader.LoadJournals(...)` and `reader.LoadGrammars(...)`.

**Consequence for Vercel:** the entire content read path is `filepath.Walk` over configured directories. On a serverless host there is no such writable tree for user uploads. This is the gap the new work fills.

### 2.4 What already lives in the DB (DB-only-state, migrations 020–027)

Migration `020_db_only_state.up.sql` moved runtime **state** and some **structural metadata** off YAML: learning history, per-quiz-type skip flags, definitions session/scene structure, flashcard deck metadata. Its own header notes that **"Story, ebook, and etymology-reference-notebook YAML stays read-only"** — i.e. the primary vocabulary/story **content** is still read from YAML through the Reader at runtime; the DB holds state plus a `notes` row per vocabulary entry (used to resolve `note_id` for logs/skip flags).

Content-bearing tables already present (see `internal/datasync/table_export.go` `DataTablesInDependencyOrder`):

- `notes` (`001_create_tables`): `id BIGSERIAL PK`, `usage`, `entry`, `meaning`, `level`, `sense_id` (added `019`, homograph-unique reworked in `022`).
- `notebook_notes`: `note_id`, `notebook_type`, **`notebook_id VARCHAR(255)` — a denormalized string**, `group`, `subgroup`.
- `note_origin_parts`, `note_images`, `note_references`, `etymology_origins`, `etymology_origin_forms`, `semantic_concepts`/`_members`, `concept_relations`, `definition_concepts`/`_members`, `definitions_sessions`, `definitions_scenes`, `flashcard_decks`, `dictionary_entries`.

Crucially, **`notebook_notes.notebook_id` is a denormalized string with no foreign key** — migration `027`'s header states explicitly that no FK is added from the `notebook_id` string columns. So the content tables are **already keyed by a notebook-id string** and are agnostic to whether that id is a shipped id or an `nb_` id. This is what makes user notebooks fit the existing tables without collision.

The YAML↔DB pipeline that populates these tables is `datasync.Importer` (`ImportNotes`, `ImportEtymologyOrigins`, `ImportDefinitionConcepts`, … in `internal/datasync/datasync.go`); the lossless per-table dump/restore is `TableDumpExporter`/`TableDumpImporter` (`internal/datasync/table_export.go`), driven by `export-db`/`import-db`.

---

## 3. The Phase-3 ownership overlay: what it is, and what it is NOT

Migration `027_notebooks_ownership.up.sql` added the `notebooks` overlay:

```sql
CREATE TABLE notebooks (
    notebook_id   VARCHAR(255) PRIMARY KEY,
    owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    visibility    VARCHAR(16) NOT NULL DEFAULT 'public'
                  CHECK (visibility IN ('public','private')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

`internal/notebook/visibility.go` resolves access:

- `NotebookVisibility` interface → `VisibleNotebookIDs(ctx, userID) (VisibilityPredicate, error)`.
- `NotebookACLRepository.VisibleNotebookIDs` computes the **hidden** set (`visibility='private' AND (owner_user_id IS NULL OR owner_user_id <> $userID)`) and returns `id ∉ hidden`. Framing it as "hidden" is what makes an **unlisted notebook fall through to visible for free** — the legacy `unlisted = public` rule.
- `AllVisible` is the YAML-only/no-DB predicate (everything visible).
- `FilterHistoriesByVisibility` drops hidden notebook ids from the history map.
- `UpsertOwnership` is the write path (used by `langner notebooks set-owner`, `newNotebooksSetOwnerCommand` in `cmd/langner/notebook.go`, and by `notebook_ownership:` config provisioning).

The quiz service composes this in one place: `Service.visibleNotebooks(userID)` (`service.go:140`) feeds `loadHistoriesForNotebooks` / `loadHistoriesForDateRange`, and the listing/by-id loaders gate each id with the same predicate. Per-user **state** isolation is migration `026` (`learning_logs.user_id`, `note_skip_flags.user_id`, `origin_skip_flags.user_id`), threaded as the `userID` parameter from the auth context.

**What the overlay does:** decides *who may SEE* a notebook that already exists in the **shared catalog**.

**What the overlay does NOT do:** it is **not** a per-user id namespace and **not** a per-user content store. Every `notebook_id` it references still points at a **shipped, filesystem-loaded** notebook. There is no mechanism today for a user to *introduce new content*. That authoring + content-storage layer is the new work; the overlay is the access-control primitive it builds on.

---

## 4. Design overview

Two cooperating layers, added on top of the existing catalog:

1. **Content-storage layer (server, DB).** User-pushed notebook content is stored in the DB, keyed by a server-minted `nb_` id, and made readable by the same Reader that serves shipped notebooks — with **zero read-path divergence** (a hard requirement, see `.claude/rules/verify-data-features-with-example-notebooks.md`).
2. **Authoring/delivery layer (CLI).** `langner notebooks push|pull|list` plus a `langner validate <file>` file-mode, authenticated by the **bearer access token** from the companion doc.

The registry/ownership decision uses the **existing `notebooks` table** (extended), so visibility "just works" for user notebooks: a private `nb_` notebook is hidden from everyone but its owner by the exact `VisibleNotebookIDs` query already in production.

```
author's machine                    langner server (Vercel)                  Postgres
----------------                    -----------------------                  --------
notebook dir (YAML)  --push-->  validate + mint nb_ id  --------------->  notebook_files (raw YAML blobs)
   index.yml, *.yml              write private ownership row ---------->  notebooks (owner, visibility, source=user)
                                 run existing import pipeline --------->  notes / notebook_notes / definitions_* / ...
                                                                           (keyed by the nb_ id string)
quiz / detail RPC   <--serve--  Reader = filesystem catalog
                                        ∪ DB blob source (nb_ ids)
```

---

## 5. Storage model (the core decision)

### 5.1 Principle: store raw YAML blobs, reuse the real parsers

The single most important architectural constraint in this repo is **"no path divergence"** — a hand-built fixture that bypasses the real reader/import path has repeatedly passed while production failed (the etymology-Relearn saga documented in `verify-data-features-with-example-notebooks.md`). Therefore user content must be read through the **same parsers** as filesystem content.

**Decision:** the canonical stored form of a user notebook is its **raw YAML files, byte-for-byte**, in a new blob table. At reader-construction time a DB-backed source feeds those exact bytes through the **same** `readYamlFile` / `walkIndexFiles` / `NewDefinitionsMap` code the filesystem walk uses. There is one parser, one set of in-memory structs, one code path — a shipped notebook and an `nb_` notebook differ only in **where the bytes came from**.

The structured content tables (`notes`, `notebook_notes`, `definitions_sessions/scenes`, `etymology_origins`, …) are populated from those same blobs by the **existing `datasync.Importer`** at push time — exactly as `migrate import-db` does today for shipped YAML — so that `note_id`/`origin_id` resolution, skip flags, and learning logs work for user notebooks with no new write path.

Why blobs *and* structured rows (not one or the other):

- **Blobs** guarantee lossless round-trip (`pull` returns exactly what was pushed) and, more importantly, guarantee the runtime read model is byte-identical to the filesystem path.
- **Structured rows** are what the DB-only-state runtime already needs (`note_id` for logs/skip flags; `definitions_scenes` for structure). Regenerating them from the blob via the proven importer avoids inventing a second ingestion path.

### 5.2 New table: `notebook_files` (raw content blobs)

```
CREATE TABLE notebook_files (
    id            BIGSERIAL PRIMARY KEY,
    notebook_id   VARCHAR(255) NOT NULL,          -- the nb_ id (FK-free string, like notebook_notes)
    path          TEXT NOT NULL,                  -- relative path within the pushed dir, e.g. "index.yml", "unit-01.yml"
    content       BYTEA NOT NULL,                 -- exact file bytes
    content_sha256 CHAR(64) NOT NULL,             -- integrity + dedup (see §12 open questions)
    byte_size     INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, path)
);
CREATE INDEX idx_notebook_files_notebook_id ON notebook_files (notebook_id);
```

Notes:
- `notebook_id` is a plain string with **no FK**, matching the established `notebook_notes.notebook_id` convention (migration 027 header). It carries the `nb_` id.
- `path` preserves the pushed directory layout so `index.yml`'s relative `notebooks: [ ... ]` references resolve against sibling blobs exactly as `Index.GetNotebookPath` resolves them against the filesystem (`textbook.go:16`).
- `content` is `BYTEA`; the table-dump layer already round-trips `bytea` losslessly (`binaryValuePrefix` handling in `table_export.go`), so `export-db`/`import-db` cover this table with no special-casing once it is added to `DataTablesInDependencyOrder`.

This new table MUST be added to `DataTablesInDependencyOrder()` (`table_export.go`) — a completeness guard test (`TestDataTablesCoverAllSchema`) derives the table set from the migrations and fails if the export list drifts.

### 5.3 Extend the `notebooks` registry (new migration 031)

> **Migration numbering (coordinated across in-flight work).** On-disk migrations end at 027. **028** = the `learning_logs.user_id NOT NULL` change (PR #77, in flight). **029/030** = the CLI-auth device-flow tables (companion doc). This proposal therefore takes **031** for `notebook_files` + the `notebooks` columns. Re-confirm the next free number at implementation time.

The `notebooks` table is already the per-notebook registry. Extend it (additively) so it also records provenance and the metadata the reader needs to enumerate DB notebooks without parsing every blob up front:

```
ALTER TABLE notebooks ADD COLUMN source       VARCHAR(16) NOT NULL DEFAULT 'shipped'
                                              CHECK (source IN ('shipped','user'));
ALTER TABLE notebooks ADD COLUMN kind         VARCHAR(32) NULL;   -- Story|Flashcard|Etymology|Definitions|Journal|Grammar
ALTER TABLE notebooks ADD COLUMN display_name TEXT NULL;          -- index.yml `name:`
ALTER TABLE notebooks ADD COLUMN content_hash CHAR(64) NULL;      -- hash of the pushed bundle, for pull/dedup/version
```

- Existing rows default to `source='shipped'` — no behavior change; shipped notebooks that already have an ownership row keep it, and unlisted shipped notebooks remain public/unlisted.
- A user push inserts a row with `source='user'`, `visibility='private'`, `owner_user_id=<pusher>`, `kind`/`display_name` read from the pushed `index.yml`.
- `VisibleNotebookIDs` is **unchanged** — a private `nb_` row is hidden from non-owners by the same query. No new visibility code.

### 5.4 How the content tables avoid collision with the shared catalog

They already do, structurally:

- `notebook_notes.notebook_id` and `learning_logs.source_notebook_id` are strings; an `nb_` id is just another value. A shipped id and an `nb_` id can never collide because §7 mints `nb_` ids from a namespace disjoint from any existing id.
- `notes.sense_id` uniqueness (`019`/`022`) is **global**, not per-notebook. Two users pushing a notebook that both contain the word `ubiquitous` would, if both authored a `sense_id: ubiquitous`, collide on `notes_sense_id_key`. **Resolution:** at push time the importer namespaces user-notebook `sense_id`s by prefixing the `nb_` id (e.g. `nb_7f3a…:ubiquitous`) before insert, OR treats id-less user notes by the legacy `(usage, entry)` partial index scoped per notebook. This is called out as an **open decision** in §12 (dedup vs. isolation) — the safe default is *isolation* (prefix), so no two users' content can ever interfere.

### 5.5 Runtime read resolution — one Reader, two sources

Introduce a small seam so `Reader` construction can union the filesystem catalog with DB blobs:

- Define a `notebook.ContentSource` that yields the same primitive the walk yields today: for each notebook, its `index.yml` bytes + the bytes of each file the index references, tagged with the notebook family (story/flashcard/etc.).
- `FilesystemContentSource` = the current `filepath.Walk` behavior (refactor `walkIndexFiles`/`NewDefinitionsMap` to accept a source).
- `DBContentSource` lists `notebooks WHERE source='user'` (optionally filtered to the requesting user's visible set) and streams the matching `notebook_files` rows.
- `NewReader` consumes both sources into the **same** `indexes` / `flashcardIndexes` / `etymologyIndexes` / `definitionsMap` maps. Downstream (`ReadStoryNotebooks`, `GetDefinitionsNotes`, quiz loaders) is unchanged and cannot tell the two apart.

Wiring: `quiz.Service` already gets DB seams injected at bootstrap when a database is configured (`SetHistoryStore`, `SetSkipStores`, `SetNoteEnsurer`, `SetNotebookACL`). Add `SetContentSource(db)` in the same place; `newReader()` (`service.go:255`) passes it through. In YAML-only/no-DB dev the DB source is nil and behavior is exactly today's.

Scoping note: because `newReader()` currently builds a **process-wide** reader, and user notebooks are per-user, the reader build must either (a) become request/user-scoped for the DB source, or (b) enumerate all `source='user'` notebooks and rely on the existing `visibleNotebooks(userID)` predicate to hide non-owned private ones at the loader/listing layer (the same defense-in-depth already used for shipped private notebooks). **(b) is the lower-risk default** — it reuses the exact filter path production already trusts. Called out in §12.

---

## 6. Server-minted `nb_` ids

- **Format:** `nb_` + 22 chars of base62 from 128 bits of CSPRNG randomness (e.g. `nb_4kQ2xR7mВ9…`), fitting the existing `VARCHAR(255)` id columns with room to spare. Opaque, non-guessable, URL-safe.
- **Where minted:** server-side in the `PushNotebook` RPC handler, before any write. Never client-supplied — the app owns the id namespace (fixed decision).
- **Uniqueness:** insert into `notebooks (notebook_id, …)`; the `PRIMARY KEY (notebook_id)` makes minting collision-safe by construction (retry on the astronomically-unlikely 23505). Because the prefix + entropy space is disjoint from every existing human-authored id, an `nb_` id can never collide with a shipped id.
- **Why existing ids stay as-is:** shipped ids are already the PK of `notebooks`, the value in `learning_logs.source_notebook_id`, `notebook_notes.notebook_id`, and the `id:` in on-disk `index.yml`. Rewriting them would rewrite production history and break every reference; there is no benefit. The PK is already globally unique, so shipped ids and `nb_` ids coexist in one namespace with no migration.
- The `index.yml` a user pushes **may** contain their own `id:`; the server **ignores it for identity** and substitutes the minted `nb_` id (rewriting the stored blob's `id:` field, or recording a mapping) so that display/storage/lookup stay symmetric (`learning-history-invariants` L3: display = storage = lookup). Whichever is chosen, the id the reader keys on, the id in `notebook_notes`, and the id the RPC returns MUST be the single `nb_` id.

---

## 7. Ownership and visibility for user notebooks

- **Composite semantics:** a user notebook is the pair `(owner_user_id, notebook_id)` in the `notebooks` registry. `owner_user_id` is set to the pusher; `visibility` defaults to `private`.
- **Private-by-default:** `PushNotebook` writes `visibility='private'`. Only its owner passes `VisibleNotebookIDs` (private + not-owned ⇒ hidden). No other user can list, load, quiz, or `pull` it.
- **State isolation is already guaranteed.** Learning state is keyed by `user_id` (migration `026`) and read scoped by the `userID` parameter (`loadHistoriesForNotebooks`). Even in the impossible-by-construction case of a shared id, two users' logs never mix. For private `nb_` notebooks the point is moot (only the owner sees it), but the invariant holds for any future "shared" user notebook: quizzing the same notebook_id yields per-user history, per-user skip flags, per-user analytics — there is no notebook-A-vs-notebook-B version of a word (`learning-history-invariants` L4).
- **Legacy rule preserved:** a notebook with **no** `notebooks` row is public. Shipped notebooks unlisted in `notebook_ownership:` stay public exactly as before. User notebooks are never row-less (push always writes a row), so they are never accidentally public.
- **Changing visibility** reuses `UpsertOwnership` / `langner notebooks set-owner` — no new mechanism. (Making a private user notebook public/shareable is an explicit action; see §12.)

---

## 8. CLI surface (in the user `langner` binary)

All three notebook subcommands authenticate with the **bearer access token** from [CLI Authentication]({{< relref "../cli-authentication/design" >}}) (the CLI holds a stored access token + refresh token, keyed per server; these commands attach the bearer to the RPC). They are added under the user `langner` binary's notebook command group (the personal generators like `stories`/`flashcards` are being removed in the two-binary split; `set-owner` moves to `langner-admin`).

### 8.1 `langner validate <file|dir>` — file-mode (prerequisite)

Today `validate` (`cmd/langner/validate.go`) is **config-driven only**: it calls `loadConfig()` and validates the directories named in `config.yml`. A user authoring a single notebook has no such config. Add a **file/dir positional argument** mode:

- `langner validate ./my-notebook/` (or a single `.yml`) constructs a `notebook.Validator` over **just that path** (reusing `NewValidator` with the one directory slotted into the right family by `kind:`), runs the same structural/consistency checks, and prints the same report — with **no** DB-consistency step (there is no configured DB for an author).
- Exit non-zero on errors so it is scriptable in a pre-push hook.
- `push` runs this validation **client-side first** and refuses to upload an invalid bundle (fail fast, before touching the server).

### 8.2 `langner notebooks push <dir|file>`

1. Client: resolve the bundle (an `index.yml` + the files it references, or a single self-contained file), run §8.1 validation locally, compute a bundle `content_hash`.
2. Upload the raw file bytes + declared `kind`/`name` to the server.
3. Server: authenticate; mint `nb_` id; write `notebooks` row (`source='user'`, `visibility='private'`, `owner_user_id`); store blobs in `notebook_files`; run the existing `datasync.Importer` over the blobs to populate `notes`/`notebook_notes`/`definitions_*`/`etymology_*`.
4. Respond with the minted `nb_` id and per-table import counts.
5. Re-pushing the same bundle to an existing `nb_` id (via `--id`) is an **update**: replace blobs, re-run the importer's additive/reconcile path (the same `EnsureNotesForNotebook`/`reconcileNotes` machinery that already exists), preserving the owner's learning state keyed by `user_id`.

### 8.3 `langner notebooks pull <id>` and `langner notebooks list`

- `pull <nb_id>` streams the stored `notebook_files` blobs back to disk (owner-only, gated by `VisibleNotebookIDs`) — lossless round-trip for local editing.
- `list` returns the caller's own notebooks (`source='user'` rows they own) with id, name, kind, visibility, size, updated-at.

### 8.4 RPC shape (Connect, `proto/api/v1/notebook.proto`)

Add to `service NotebookService` (which today has `GetNotebookDetail`, `ExportNotebookPDF`, `LookupWord`, `RegisterDefinition`, `DeleteDefinition`, `GetEtymologyNotebook`):

```proto
rpc PushNotebook(PushNotebookRequest) returns (PushNotebookResponse);
rpc PullNotebook(PullNotebookRequest) returns (PullNotebookResponse);
rpc ListMyNotebooks(ListMyNotebooksRequest) returns (ListMyNotebooksResponse);

message NotebookFile { string path = 1; bytes content = 2; }

message PushNotebookRequest {
  string notebook_id = 1;              // empty = create (mint nb_); set = update existing
  string kind = 2;                     // from index.yml `kind:`
  string name = 3;                     // from index.yml `name:`
  repeated NotebookFile files = 4;     // index.yml + referenced files
  string content_hash = 5;             // client-computed, server re-verifies
}
message PushNotebookResponse {
  string notebook_id = 1;              // the minted / confirmed nb_ id
  map<string,int32> imported_rows = 2; // per-table counts from the importer
}

message PullNotebookRequest  { string notebook_id = 1; }
message PullNotebookResponse { repeated NotebookFile files = 1; string content_hash = 2; }

message ListMyNotebooksRequest {}
message ListMyNotebooksResponse {
  message Entry { string notebook_id = 1; string name = 2; string kind = 3;
                  string visibility = 4; int32 byte_size = 5; int64 updated_at = 6; }
  repeated Entry notebooks = 1;
}
```

All three are gated by the existing auth interceptor (a real `userID > 0`) and enforce ownership: `Pull`/`List` through `VisibleNotebookIDs`; `Push` update-mode verifies the caller owns the target `nb_` id.

---

## 9. Push-time ingestion pipeline (server)

Reuse, don't reinvent. The server-side of `PushNotebook`:

1. Persist blobs to `notebook_files` (transactional with the registry row).
2. Build a `NoteSource` backed by those blobs — the **same `notebook.Reader`-derived `NoteSource`** the offline importer uses (`datasync.NoteSource`, `datasync.go:107`), just reading DB blobs instead of a directory. This is the DB-content-source from §5.5 pointed at one notebook.
3. Run the relevant `Importer.Import*` methods (`ImportNotes`, and per `kind`: `ImportEtymologyOrigins`/`…Forms`/`ImportNoteOriginParts`, `ImportDefinitionConcepts`, etc.) with `sense_id` namespaced by the `nb_` id (§5.4) so global uniqueness holds without cross-user interference.
4. The importer's existing homograph gate (`DuplicateWordsError`) and oversize-`sense_id` gate (`OversizedSenseIDError`) surface author mistakes as RPC errors with the same actionable messages.

No new SRS/log/skip write paths are introduced — those already key off `note_id`/`origin_id` + `user_id`, which now exist for user notebooks.

---

## 10. Migration and rollout (existing ids untouched)

- **Migration 031** adds `notebook_files` and the four additive `notebooks` columns (`source` default `'shipped'`, nullable `kind`/`display_name`/`content_hash`). All existing rows are valid unchanged; no data migration.
- Add `notebook_files` to `DataTablesInDependencyOrder()` (child of nothing content-wise; place near `notebooks`) so `export-db`/`import-db` and the drift/rebuild tooling cover it, and the `TestDataTablesCoverAllSchema` guard passes.
- Shipped catalog keeps loading from the filesystem via `FilesystemContentSource`; its ids, `index.yml`s, and prod DB references are untouched. The DB source is purely additive.
- Rollout order: (1) migration 028 + export-list update; (2) `ContentSource` refactor with the filesystem source behaving identically (pure refactor, guarded by the existing reader tests); (3) `DBContentSource` + `SetContentSource` wiring; (4) `PushNotebook`/`Pull`/`List` RPCs; (5) CLI `validate` file-mode + `notebooks push/pull/list`. Each step is independently shippable and dark until the RPCs land.

---

## 11. Verification plan (required by repo rule)

`.claude/rules/verify-data-features-with-example-notebooks.md` makes this **mandatory** for any feature that loads/writes/quizzes/groups/displays notebook data. This feature must ship, alongside the code:

1. **Example user-notebook bundles** in the real on-disk YAML shape under `examples/` (a story, a definitions book with a `concepts:` block, an etymology notebook with `origin_parts`, at least one bundle exercising the awkward conditions: a homograph needing `sense_id`, a word also present in a shipped notebook (cross-notebook dedup/isolation), a multi-file `index.yml` with referenced sibling files).
2. **A real-config integration test** that: loads `config.example.yml` to build the `Service`/`Reader` exactly as the server does; pushes an example bundle through the **real** push→blob-store→`Importer`→`DBContentSource`→`Reader` path (not a hand-built `Index`/`Note`); then drives a full quiz (standard + reverse), a miss, and Relearn grouping against the `nb_` notebook, asserting the **observable** behavior (the word quizzes, the miss folds into the right family/day-detail, the private notebook is hidden from a second `userID`, `pull` returns byte-identical files).
3. **Seed leftover state** where relevant (e.g. a pre-existing shipped note with the same spelling, a `skipped_at` marker) to prove isolation/dedup behavior, per the rule's "example notebooks are NOT enough — seed the state too" clause.

Because the DB path only runs against Postgres, the browser/PG-only portions are treated as **unverified until exercised on a real database** (`.claude/rules/no-unverified-main-merges.md`): CI green is not sufficient; drive `push` + a quiz against a live PG before any merge to `main`.

---

## 12. Open questions / decisions still needed

1. **Reader scoping (process-wide vs per-user).** §5.5 proposes enumerating all `source='user'` notebooks and relying on `visibleNotebooks(userID)` to hide non-owned private ones (reuses production's trusted filter). The alternative — a per-request/per-user reader build — is cleaner isolation but changes the current process-wide `newReader()` caching and cost profile. **Decision needed:** confirm the reuse-the-predicate default, or invest in a user-scoped reader.
2. **`sense_id` isolation vs. cross-user dedup.** Default proposed: **isolate** by prefixing user `sense_id`s with the `nb_` id, so no two users' content can collide or interfere. The alternative — dedup identical content across users into shared `notes` rows — saves storage but couples users' data and complicates ownership/state. Recommend isolation; confirm.
3. **Quotas and size limits.** Max files per push, max bytes per file/bundle, max notebooks per user. Needed to bound `notebook_files` growth and abuse on a shared Vercel deployment. No limit exists today.
4. **Sharing a private notebook.** The registry can express a public user notebook (`visibility='public'`), but "share with specific other users" would need a per-(notebook,user) grant table beyond the single `owner_user_id`. Out of scope for v1 (private-only + owner-public), but the schema shape should not preclude it.
5. **Update/versioning semantics.** Re-push replaces blobs and re-runs the importer's reconcile path. Do we keep version history of `notebook_files` (append-only) or overwrite in place? Overwrite is simplest; history aids recovery. Decide whether `content_hash` gates no-op re-pushes.
6. **Deletion.** `langner notebooks delete <nb_id>` (owner-only) must cascade: `notebook_files`, the `notebooks` row, and the content rows keyed by the id — plus a decision on whether to delete or tombstone the owner's learning history for that notebook.
7. **What `kind`s are accepted from users initially.** Recommend starting with story + definitions + flashcard + etymology; defer grammar/journal until their author ergonomics are validated.
8. **Validation parity.** Confirm `NewValidator` can run over a single ad-hoc path without the full config (§8.1) and that its consistency checks (orphan learning notes, duplicate expressions, missing scenes) are meaningful for a standalone bundle with no learning history yet.
