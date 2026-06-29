ALTER TABLE learning_logs
    ADD COLUMN IF NOT EXISTS source_notebook_id VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_note_id ON learning_logs (note_id);

ALTER TABLE learning_logs
    DROP CONSTRAINT IF EXISTS learning_logs_note_quiz_learned_key;
