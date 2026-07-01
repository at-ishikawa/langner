ALTER TABLE learning_logs
    ADD CONSTRAINT learning_logs_note_quiz_learned_key UNIQUE (note_id, quiz_type, learned_at);
DROP INDEX IF EXISTS idx_note_id;
ALTER TABLE learning_logs DROP COLUMN source_notebook_id;
