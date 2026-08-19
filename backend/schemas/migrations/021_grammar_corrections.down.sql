-- Reverse 021_grammar_corrections. Drop the learning_logs dependents before
-- the grammar_corrections table the FK references.
DROP INDEX IF EXISTS idx_learning_logs_correction_id;
ALTER TABLE learning_logs DROP CONSTRAINT IF EXISTS fk_learning_logs_correction_id;
ALTER TABLE learning_logs DROP COLUMN IF EXISTS correction_id;

DROP TABLE IF EXISTS grammar_corrections;
