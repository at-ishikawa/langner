ALTER TABLE learning_logs ADD UNIQUE KEY note_id (note_id, quiz_type, learned_at);
ALTER TABLE learning_logs DROP INDEX idx_note_id;
ALTER TABLE learning_logs DROP COLUMN source_notebook_id;
