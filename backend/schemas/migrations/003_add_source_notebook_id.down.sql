ALTER TABLE learning_logs ADD UNIQUE KEY note_id (note_id, quiz_type, learned_at);
ALTER TABLE learning_logs DROP INDEX uniq_note_quiz_learned_notebook;
ALTER TABLE learning_logs DROP COLUMN source_notebook_id;
