DROP INDEX idx_notes_skipped_at ON notes;
ALTER TABLE notes DROP COLUMN skipped_at;
