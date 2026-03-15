ALTER TABLE learning_logs ADD INDEX idx_note_id (note_id);
ALTER TABLE learning_logs DROP INDEX uniq_note_quiz_learned_notebook;
