-- Roll back 019: drop the partial unique index and the sense_id column.
-- UNIQUE(usage, entry) from migration 001 is left in place and resumes as the
-- sole note identity.
DROP INDEX IF EXISTS notes_sense_id_key;
ALTER TABLE notes DROP COLUMN IF EXISTS sense_id;
