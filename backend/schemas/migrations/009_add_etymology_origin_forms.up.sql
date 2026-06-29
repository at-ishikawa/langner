CREATE TABLE etymology_origin_forms (
    id BIGSERIAL PRIMARY KEY,
    origin_id BIGINT NOT NULL,
    form VARCHAR(255) NOT NULL,
    role VARCHAR(100) NOT NULL,
    note TEXT,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_etymology_origin_forms_origin_id
        FOREIGN KEY (origin_id) REFERENCES etymology_origins(id) ON DELETE CASCADE,
    UNIQUE (origin_id, role, form)
);
CREATE INDEX idx_etymology_origin_forms_origin_id ON etymology_origin_forms (origin_id);
CREATE TRIGGER etymology_origin_forms_set_updated_at BEFORE UPDATE ON etymology_origin_forms
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE note_origin_parts
    ADD COLUMN origin_form_id BIGINT NULL;

ALTER TABLE note_origin_parts
    ADD CONSTRAINT fk_note_origin_parts_origin_form_id
        FOREIGN KEY (origin_form_id) REFERENCES etymology_origin_forms(id) ON DELETE SET NULL;
