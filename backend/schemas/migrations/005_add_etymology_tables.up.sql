CREATE TABLE etymology_origins (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    origin VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    language VARCHAR(100) NOT NULL,
    meaning TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT etymology_origins_notebook_origin_lang_key UNIQUE (notebook_id, origin, language)
);
CREATE INDEX idx_etymology_origins_notebook_id ON etymology_origins (notebook_id);
CREATE INDEX idx_etymology_origins_origin_lang ON etymology_origins (origin, language);
CREATE INDEX idx_etymology_origins_meaning ON etymology_origins (meaning);
CREATE TRIGGER etymology_origins_set_updated_at BEFORE UPDATE ON etymology_origins
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE note_origin_parts (
    id BIGSERIAL PRIMARY KEY,
    note_id BIGINT NOT NULL,
    origin_id BIGINT NOT NULL,
    sort_order INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id),
    FOREIGN KEY (origin_id) REFERENCES etymology_origins(id),
    UNIQUE (note_id, sort_order)
);
CREATE INDEX idx_note_origin_parts_note_id ON note_origin_parts (note_id);
CREATE INDEX idx_note_origin_parts_origin_id ON note_origin_parts (origin_id);
CREATE TRIGGER note_origin_parts_set_updated_at BEFORE UPDATE ON note_origin_parts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
