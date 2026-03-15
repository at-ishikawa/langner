ALTER TABLE learning_logs ADD UNIQUE KEY uniq_note_quiz_learned_notebook (note_id, quiz_type, learned_at, source_notebook_id);
ALTER TABLE learning_logs DROP INDEX idx_note_id;
