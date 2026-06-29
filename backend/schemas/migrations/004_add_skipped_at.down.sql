DROP INDEX IF EXISTS idx_notes_skipped_at;
ALTER TABLE notes DROP COLUMN skipped_at;
