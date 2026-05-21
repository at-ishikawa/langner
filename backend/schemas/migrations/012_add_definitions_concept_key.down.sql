ALTER TABLE learning_logs DROP INDEX idx_learning_logs_concept_key;
ALTER TABLE learning_logs DROP COLUMN concept_key;

ALTER TABLE notes DROP INDEX idx_notes_concept_key;
ALTER TABLE notes DROP COLUMN concept_key;
