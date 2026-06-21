ALTER TABLE learning_logs
    DROP INDEX idx_learning_logs_origin_id,
    DROP FOREIGN KEY fk_learning_logs_origin_id,
    DROP COLUMN origin_id,
    MODIFY COLUMN note_id BIGINT NOT NULL;

DROP TABLE origin_skip_flags;
DROP TABLE note_skip_flags;
DROP TABLE flashcard_decks;
DROP TABLE definitions_scenes;
DROP TABLE definitions_sessions;
