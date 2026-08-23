-- DB-only state tables. The runtime stops reading and writing learning
-- history, definition session/scene structure, flashcard deck metadata,
-- per-quiz-type skip flags, and etymology quiz logs from YAML. Story,
-- ebook, and etymology-reference-notebook YAML stays read-only.
--
-- PostgreSQL port of the original MySQL migration: BIGSERIAL for
-- auto-increment, per-table set_updated_at() triggers (there is no
-- ON UPDATE CURRENT_TIMESTAMP column attribute), and plain UNIQUE
-- constraints on TEXT columns (real session/scene/deck titles are
-- short; Postgres indexes the full value with no prefix dance).

-- definitions_sessions: replaces the YAML Definitions.Metadata block.
-- One row per session in a definitions notebook (e.g. "Session 13:
-- graphein"). date drives the notebook-detail sort order; sort_order
-- preserves the YAML's original ordering for ties. title and
-- notebook_file are TEXT because real definitions YAML carries lesson
-- headings past the VARCHAR(255) cap.
CREATE TABLE definitions_sessions (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    title TEXT NOT NULL,
    notebook_file TEXT NOT NULL DEFAULT '',
    "date" TIMESTAMP NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, title, notebook_file)
);
CREATE INDEX idx_definitions_sessions_notebook_id ON definitions_sessions (notebook_id);
CREATE TRIGGER definitions_sessions_set_updated_at BEFORE UPDATE ON definitions_sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- definitions_scenes: replaces the YAML DefinitionsScene.Metadata block.
-- One row per scene under a session. scene_index is the YAML field; it
-- only has to be unique within one session.
CREATE TABLE definitions_scenes (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    scene_index INT NOT NULL DEFAULT 0,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES definitions_sessions(id) ON DELETE CASCADE,
    UNIQUE (session_id, scene_index)
);
CREATE INDEX idx_definitions_scenes_session_id ON definitions_scenes (session_id);
CREATE TRIGGER definitions_scenes_set_updated_at BEFORE UPDATE ON definitions_scenes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- flashcard_decks: replaces the YAML FlashcardNotebook outer struct.
-- One row per flashcard deck within a flashcard index. The cards
-- themselves are still stored as notes + notebook_notes(group=title).
CREATE TABLE flashcard_decks (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    "date" TIMESTAMP NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, title)
);
CREATE INDEX idx_flashcard_decks_notebook_id ON flashcard_decks (notebook_id);
CREATE TRIGGER flashcard_decks_set_updated_at BEFORE UPDATE ON flashcard_decks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- note_skip_flags: replaces the YAML SkippedAtMap on
-- LearningHistoryExpression. One row per (note, quiz_type) the user has
-- excluded. quiz_type is the same string set used in learning_logs.
CREATE TABLE note_skip_flags (
    id BIGSERIAL PRIMARY KEY,
    note_id BIGINT NOT NULL,
    quiz_type VARCHAR(50) NOT NULL,
    skipped_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE,
    UNIQUE (note_id, quiz_type)
);
CREATE INDEX idx_note_skip_flags_note_id ON note_skip_flags (note_id);
CREATE TRIGGER note_skip_flags_set_updated_at BEFORE UPDATE ON note_skip_flags
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- origin_skip_flags: same role as note_skip_flags but for etymology
-- origins. Etymology quizzes target origins, not notes, so they need
-- their own skip table; keeping them separate keeps note_skip_flags
-- queries note-only.
CREATE TABLE origin_skip_flags (
    id BIGSERIAL PRIMARY KEY,
    origin_id BIGINT NOT NULL,
    quiz_type VARCHAR(50) NOT NULL,
    skipped_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (origin_id) REFERENCES etymology_origins(id) ON DELETE CASCADE,
    UNIQUE (origin_id, quiz_type)
);
CREATE INDEX idx_origin_skip_flags_origin_id ON origin_skip_flags (origin_id);
CREATE TRIGGER origin_skip_flags_set_updated_at BEFORE UPDATE ON origin_skip_flags
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- learning_logs extension: etymology quizzes target origins, not notes,
-- so note_id must become nullable and origin_id needs to exist. Exactly
-- one of the two is set per row. source_notebook_id already carries the
-- etymology notebook ID; the new origin_id column lets analytics filter
-- etymology logs directly without joining notes.
ALTER TABLE learning_logs ALTER COLUMN note_id DROP NOT NULL;
ALTER TABLE learning_logs ADD COLUMN origin_id BIGINT NULL;
ALTER TABLE learning_logs
    ADD CONSTRAINT fk_learning_logs_origin_id
    FOREIGN KEY (origin_id) REFERENCES etymology_origins(id) ON DELETE CASCADE;
CREATE INDEX idx_learning_logs_origin_id ON learning_logs (origin_id);
