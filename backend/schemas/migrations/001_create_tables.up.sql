-- Trigger function that keeps updated_at in sync with row changes. PostgreSQL
-- has no MySQL-style "ON UPDATE CURRENT_TIMESTAMP" column attribute, so we
-- emulate it with a single shared trigger function applied per table.
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE notes (
    id BIGSERIAL PRIMARY KEY,
    "usage" VARCHAR(255) NOT NULL,
    entry VARCHAR(255) NOT NULL,
    meaning TEXT,
    level VARCHAR(50),
    dictionary_number INT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE ("usage", entry)
);
CREATE INDEX idx_notes_usage ON notes ("usage");
CREATE INDEX idx_notes_entry ON notes (entry);
CREATE TRIGGER notes_set_updated_at BEFORE UPDATE ON notes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notebook_notes (
    id BIGSERIAL PRIMARY KEY,
    note_id BIGINT NOT NULL,
    notebook_type VARCHAR(50) NOT NULL,
    notebook_id VARCHAR(255) NOT NULL,
    "group" VARCHAR(255),
    subgroup VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id),
    UNIQUE (note_id, notebook_type, notebook_id, "group")
);
CREATE INDEX idx_notebook_notes_note_id ON notebook_notes (note_id);
CREATE INDEX idx_notebook_notes_type_id ON notebook_notes (notebook_type, notebook_id);
CREATE TRIGGER notebook_notes_set_updated_at BEFORE UPDATE ON notebook_notes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE note_images (
    id BIGSERIAL PRIMARY KEY,
    note_id BIGINT NOT NULL,
    url VARCHAR(2048) NOT NULL,
    sort_order INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);
CREATE TRIGGER note_images_set_updated_at BEFORE UPDATE ON note_images
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE note_references (
    id BIGSERIAL PRIMARY KEY,
    note_id BIGINT NOT NULL,
    link VARCHAR(2048) NOT NULL,
    description VARCHAR(1000),
    sort_order INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);
CREATE TRIGGER note_references_set_updated_at BEFORE UPDATE ON note_references
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE learning_logs (
    id BIGSERIAL PRIMARY KEY,
    note_id BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL,
    learned_at TIMESTAMP NOT NULL,
    quality INT,
    response_time_ms INT,
    quiz_type VARCHAR(50) NOT NULL,
    interval_days INT,
    easiness_factor DECIMAL(3,2),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id),
    CONSTRAINT learning_logs_note_quiz_learned_key UNIQUE (note_id, quiz_type, learned_at)
);
CREATE INDEX idx_learning_logs_learned_at ON learning_logs (learned_at);
CREATE TRIGGER learning_logs_set_updated_at BEFORE UPDATE ON learning_logs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE dictionary_entries (
    word VARCHAR(255) PRIMARY KEY,
    source_type VARCHAR(50) NOT NULL,
    source_url VARCHAR(2048),
    response JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER dictionary_entries_set_updated_at BEFORE UPDATE ON dictionary_entries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
