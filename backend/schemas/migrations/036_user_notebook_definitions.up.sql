-- Per-user private highlight registrations (Project Gutenberg books feature).
--
-- A shared, public, owner-less book (ImportShippedBundle, owner_user_id=NULL,
-- visibility='public') is read by everyone, but each learner's REGISTERED
-- highlights — the words they chose to study, their own chosen meaning, and
-- WHERE in the book — must be private to them. The existing notes /
-- notebook_notes tables carry no user_id (migration 001), so a definition
-- written there is global and would leak to every reader of a shared book;
-- and in DB/server mode a registered definition never surfaced as a card or a
-- bolded highlight at all, because the reader + quiz build their word lists
-- only from scene.Definitions embedded in content. This table is the per-user
-- store the serve-time overlay (MergeDefinitionsIntoNotebooks seam) reads to
-- bridge both gaps — privately, per user.
--
-- note_id references the existing global notes row (find-or-create by
-- sense/expression) purely as a STABLE WORD IDENTITY so per-user learning_logs
-- (already user_id-scoped) bind correctly. The per-user `meaning` here is
-- authoritative — NOT notes.meaning (two users may save different meanings for
-- the same word in the same book; each keeps their own copy).
--
-- notebook_id is a plain string with NO foreign key, matching the established
-- notebook_notes.notebook_id / learning_logs.source_notebook_id convention.
-- `event` holds the notebook_file (or event title) key and scene_index the
-- 0-based scene, so the overlay can rebuild a definitionsMap keyed exactly like
-- MergeDefinitionsIntoNotebooks expects.
CREATE TABLE user_notebook_definitions (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notebook_id    VARCHAR(255) NOT NULL,
    note_id        BIGINT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    event          TEXT NOT NULL DEFAULT '',   -- notebook_file (or event title) key
    scene_index    INT NOT NULL DEFAULT 0,     -- 0-based scene within the file
    meaning        TEXT NOT NULL DEFAULT '',   -- the user's own authoritative meaning
    part_of_speech TEXT NOT NULL DEFAULT '',
    examples       TEXT NOT NULL DEFAULT '',   -- JSON-encoded []string
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- One registration per (user, notebook, word): re-registering the same word
    -- updates the user's private copy in place (upsert), never duplicates it.
    UNIQUE (user_id, notebook_id, note_id)
);

-- The serve-time overlay reads a single user's rows for one notebook.
CREATE INDEX idx_user_notebook_definitions_user_notebook
    ON user_notebook_definitions (user_id, notebook_id);

-- LookupWord's shared-suggestion query scans every user's rows for a
-- (notebook_id, note_id) across the book.
CREATE INDEX idx_user_notebook_definitions_notebook_note
    ON user_notebook_definitions (notebook_id, note_id);

CREATE TRIGGER user_notebook_definitions_set_updated_at BEFORE UPDATE ON user_notebook_definitions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
