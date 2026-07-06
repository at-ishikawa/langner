ALTER TABLE etymology_origins
    DROP CONSTRAINT uniq_session_origin;

ALTER TABLE etymology_origins
    ADD CONSTRAINT etymology_origins_notebook_origin_lang_key UNIQUE (notebook_id, origin, language);

ALTER TABLE etymology_origins
    DROP COLUMN session_title;
