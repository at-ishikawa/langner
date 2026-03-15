ALTER TABLE learning_logs ADD COLUMN source_notebook_id VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'Notebook ID from which this log was imported' AFTER easiness_factor;
ALTER TABLE learning_logs ADD UNIQUE KEY uniq_note_quiz_learned_notebook (note_id, quiz_type, learned_at, source_notebook_id);
ALTER TABLE learning_logs DROP INDEX note_id;
