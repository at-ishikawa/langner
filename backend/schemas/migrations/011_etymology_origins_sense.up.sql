ALTER TABLE etymology_origins
    ADD COLUMN sense VARCHAR(50) NOT NULL DEFAULT '';

-- IF EXISTS so a drifted schema does not hard-fail (SQLSTATE 42704) and leave
-- schema_migrations dirty. Unchanged on a fresh DB from migration 008.
ALTER TABLE etymology_origins
    DROP CONSTRAINT IF EXISTS uniq_session_origin;

ALTER TABLE etymology_origins
    ADD CONSTRAINT uniq_session_origin UNIQUE (notebook_id, session_title, origin, language, sense);
