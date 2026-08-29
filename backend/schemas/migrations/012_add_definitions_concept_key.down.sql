DROP INDEX IF EXISTS idx_learning_logs_concept_key;
ALTER TABLE learning_logs DROP COLUMN IF EXISTS concept_key;

DROP INDEX IF EXISTS idx_notes_concept_key;
ALTER TABLE notes DROP COLUMN IF EXISTS concept_key;
