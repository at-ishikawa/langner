DROP INDEX IF EXISTS idx_notes_skipped_at;
ALTER TABLE notes DROP COLUMN IF EXISTS skipped_at;
