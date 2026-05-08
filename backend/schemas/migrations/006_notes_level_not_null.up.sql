-- The Go NoteRecord.Level field is a plain string, but the original column
-- definition allowed NULL. Existing rows imported before the import path
-- always set level have NULL values, which break sql.Scan into string.
-- Backfill those rows with the empty string and tighten the constraint so
-- future inserts can't reintroduce NULL.

UPDATE notes SET level = '' WHERE level IS NULL;

ALTER TABLE notes
    MODIFY COLUMN level VARCHAR(50) NOT NULL DEFAULT ''
        COMMENT 'Proficiency level (e.g., beginner, intermediate)';
