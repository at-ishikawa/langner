-- Reverse 026: drop the per-user indexes and the user_id columns.
DROP INDEX IF EXISTS idx_origin_skip_flags_user_id;
DROP INDEX IF EXISTS idx_note_skip_flags_user_id;
DROP INDEX IF EXISTS idx_learning_logs_user_notebook;
DROP INDEX IF EXISTS idx_learning_logs_user_id;

ALTER TABLE origin_skip_flags DROP COLUMN IF EXISTS user_id;
ALTER TABLE note_skip_flags DROP COLUMN IF EXISTS user_id;
ALTER TABLE learning_logs DROP COLUMN IF EXISTS user_id;
