ALTER TABLE etymology_origins
    ADD COLUMN session_title VARCHAR(100) NOT NULL DEFAULT '';

ALTER TABLE etymology_origins
    DROP CONSTRAINT etymology_origins_notebook_origin_lang_key;

ALTER TABLE etymology_origins
    ADD CONSTRAINT uniq_session_origin UNIQUE (notebook_id, session_title, origin, language);
