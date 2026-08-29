ALTER TABLE etymology_origins
    ADD COLUMN session_title VARCHAR(100) NOT NULL DEFAULT '';

-- IF EXISTS so a drifted schema (older/renumbered chain that named this
-- constraint differently or already dropped it) does not hard-fail here and
-- leave schema_migrations dirty. Unchanged on a fresh DB from migration 005.
ALTER TABLE etymology_origins
    DROP CONSTRAINT IF EXISTS etymology_origins_notebook_origin_lang_key;

ALTER TABLE etymology_origins
    ADD CONSTRAINT uniq_session_origin UNIQUE (notebook_id, session_title, origin, language);
