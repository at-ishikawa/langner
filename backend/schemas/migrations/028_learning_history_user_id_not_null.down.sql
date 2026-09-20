-- Reverse 028: allow user_id to be NULL again on the three per-user state
-- tables (restores the migration-026 nullable shape).
ALTER TABLE origin_skip_flags ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE note_skip_flags ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE learning_logs ALTER COLUMN user_id DROP NOT NULL;
