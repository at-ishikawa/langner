ALTER TABLE notes ADD COLUMN skipped_at TIMESTAMP NULL COMMENT 'When this note was marked to skip from quizzes';
CREATE INDEX idx_notes_skipped_at ON notes (skipped_at);
