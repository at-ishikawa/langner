-- Reverse 017_db_only_state. Drop the dependents on learning_logs
-- before the tables the origin FK references.
DROP INDEX IF EXISTS idx_learning_logs_origin_id;
ALTER TABLE learning_logs DROP CONSTRAINT IF EXISTS fk_learning_logs_origin_id;
ALTER TABLE learning_logs DROP COLUMN IF EXISTS origin_id;
ALTER TABLE learning_logs ALTER COLUMN note_id SET NOT NULL;

DROP TABLE IF EXISTS origin_skip_flags;
DROP TABLE IF EXISTS note_skip_flags;
DROP TABLE IF EXISTS flashcard_decks;
DROP TABLE IF EXISTS definitions_scenes;
DROP TABLE IF EXISTS definitions_sessions;
