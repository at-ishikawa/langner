-- DB-only state tables. The runtime stops reading and writing learning
-- history, definition session/scene structure, flashcard deck metadata,
-- per-quiz-type skip flags, and etymology quiz logs from YAML. Story,
-- ebook, and etymology-reference-notebook YAML stays read-only.

-- definitions_sessions: replaces the YAML Definitions.Metadata block.
-- One row per session in a definitions notebook (e.g. "Session 13:
-- graphein"). Date drives the notebook-detail sort order; sort_order
-- preserves the YAML's original ordering for ties.
-- title and notebook_file use TEXT because real definitions YAML
-- carries lesson headings well over the 255-char VARCHAR cap (e.g.
-- the Word Power Made Easy session titles). The UNIQUE index uses a
-- 191-char prefix on title and notebook_file so the three-column key
-- stays under InnoDB's 3072-byte index limit on utf8mb4: 191*4 +
-- 191*4 + 255*4 = 2548 bytes.
CREATE TABLE definitions_sessions (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL COMMENT 'Definitions notebook this session belongs to',
    title TEXT NOT NULL COMMENT 'Session title (was Metadata.Title in YAML)',
    notebook_file TEXT NOT NULL COMMENT 'Original YAML filename, kept as a stable identifier when the title is missing',
    date DATETIME NULL COMMENT 'Optional session date for sorting',
    sort_order INT NOT NULL DEFAULT 0 COMMENT 'Tie-breaker when date is null or identical',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, title(191), notebook_file(191)),
    INDEX (notebook_id)
) COMMENT='Sessions inside a definitions notebook';

-- definitions_scenes: replaces the YAML DefinitionsScene.Metadata block.
-- One row per scene under a session. scene_index is the YAML field; it
-- doesn't have to be unique across sessions, only within one.
-- Scene title is TEXT to hold real lesson scene names — some entries
-- in the user's definitions YAML run hundreds of characters. Nothing
-- indexes the title column, so no prefix dance is needed here.
CREATE TABLE definitions_scenes (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    session_id BIGINT NOT NULL,
    title TEXT NOT NULL COMMENT 'Scene title (was metadata.title)',
    scene_index INT NOT NULL DEFAULT 0 COMMENT 'Scene index within the session (was metadata.scene/index)',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES definitions_sessions(id) ON DELETE CASCADE,
    UNIQUE (session_id, scene_index),
    INDEX (session_id)
) COMMENT='Scenes inside a definitions session';

-- flashcard_decks: replaces the YAML FlashcardNotebook outer struct.
-- One row per flashcard deck within a flashcard index. The cards
-- themselves are still stored as notes + notebook_notes(group=title).
-- Deck title is TEXT for the same reason definitions session/scene
-- titles are TEXT — user YAML can carry long names. UNIQUE uses a
-- 191-char prefix to keep the index under InnoDB's 3072-byte cap.
CREATE TABLE flashcard_decks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL COMMENT 'Flashcard index ID this deck belongs to',
    title TEXT NOT NULL COMMENT 'Deck title; matches notebook_notes.group for its cards',
    description TEXT NOT NULL,
    date DATETIME NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, title(191)),
    INDEX (notebook_id)
) COMMENT='Flashcard deck metadata';

-- note_skip_flags: replaces the YAML SkippedAtMap on
-- LearningHistoryExpression. One row per (note, quiz_type) the user has
-- excluded. quiz_type is the same string set used in learning_logs.
CREATE TABLE note_skip_flags (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL,
    quiz_type VARCHAR(50) NOT NULL COMMENT 'notebook | reverse | freeform | etymology_standard | etymology_reverse | etymology_freeform',
    skipped_at DATETIME NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE,
    UNIQUE (note_id, quiz_type),
    INDEX (note_id)
) COMMENT='Per-quiz-type skip flags for vocabulary notes';

-- origin_skip_flags: same role as note_skip_flags but for etymology
-- origins. Etymology quizzes target origins, not notes, so they need
-- their own skip table; keeping them separate keeps note_skip_flags
-- queries note-only.
CREATE TABLE origin_skip_flags (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    origin_id BIGINT NOT NULL,
    quiz_type VARCHAR(50) NOT NULL,
    skipped_at DATETIME NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (origin_id) REFERENCES etymology_origins(id) ON DELETE CASCADE,
    UNIQUE (origin_id, quiz_type),
    INDEX (origin_id)
) COMMENT='Per-quiz-type skip flags for etymology origins';

-- learning_logs extension: etymology quizzes target origins, not notes,
-- so note_id must become nullable and origin_id needs to exist. Exactly
-- one of the two is set per row. source_notebook_id already carries the
-- etymology notebook ID; the new origin_id column lets analytics filter
-- etymology logs directly without joining notes.
--
-- These are split into four statements (instead of a single multi-clause
-- ALTER TABLE) because TiDB processes the clauses of a multi-clause
-- ALTER independently; the ADD INDEX clause errored with "Key column
-- 'origin_id' doesn't exist in table" because it ran before the ADD
-- COLUMN that created the column.
ALTER TABLE learning_logs
    MODIFY COLUMN note_id BIGINT NULL COMMENT 'Vocab note this log targets (null for etymology logs)';
ALTER TABLE learning_logs
    ADD COLUMN origin_id BIGINT NULL COMMENT 'Etymology origin this log targets (null for vocab logs)' AFTER note_id;
ALTER TABLE learning_logs
    ADD CONSTRAINT fk_learning_logs_origin_id FOREIGN KEY (origin_id) REFERENCES etymology_origins(id) ON DELETE CASCADE;
ALTER TABLE learning_logs
    ADD INDEX idx_learning_logs_origin_id (origin_id);
