-- Restore the migration 008 unique key shape, dropping the sense column.
ALTER TABLE etymology_origins
    DROP INDEX uniq_session_origin;

ALTER TABLE etymology_origins
    DROP COLUMN sense;

ALTER TABLE etymology_origins
    ADD UNIQUE KEY uniq_session_origin (notebook_id, session_title, origin, language);
