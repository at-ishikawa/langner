-- Split for TiDB compatibility: see the .up counterpart for context.
-- Drop the dependents (index, FK) before the column they reference.
ALTER TABLE learning_logs DROP INDEX idx_learning_logs_origin_id;
ALTER TABLE learning_logs DROP FOREIGN KEY fk_learning_logs_origin_id;
ALTER TABLE learning_logs DROP COLUMN origin_id;
ALTER TABLE learning_logs MODIFY COLUMN note_id BIGINT NOT NULL;

DROP TABLE origin_skip_flags;
DROP TABLE note_skip_flags;
DROP TABLE flashcard_decks;
DROP TABLE definitions_scenes;
DROP TABLE definitions_sessions;
