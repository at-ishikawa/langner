ALTER TABLE notes ADD COLUMN skipped_at TIMESTAMP NULL;
CREATE INDEX idx_notes_skipped_at ON notes (skipped_at);
