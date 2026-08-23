-- Revert notes.sense_id back to VARCHAR(128). Mirrors migration 013's down.
-- This narrows the column, so it will fail if any existing sense_id exceeds
-- 128 chars — expected for a rollback, same as 013.
ALTER TABLE notes ALTER COLUMN sense_id TYPE VARCHAR(128);
