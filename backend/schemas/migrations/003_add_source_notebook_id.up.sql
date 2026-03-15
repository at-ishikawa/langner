ALTER TABLE learning_logs ADD COLUMN source_notebook_id VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'Notebook ID from which this log was imported' AFTER easiness_factor;
ALTER TABLE learning_logs ADD INDEX idx_note_id (note_id);
ALTER TABLE learning_logs DROP INDEX note_id;
