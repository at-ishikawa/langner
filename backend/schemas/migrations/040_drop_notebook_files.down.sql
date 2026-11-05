-- Recreate the empty notebook_files table (migrations 031 + 032). The dropped
-- files are not restored.
CREATE TABLE notebook_files (
    id             BIGSERIAL PRIMARY KEY,
    notebook_id    VARCHAR(255) NOT NULL,
    family         VARCHAR(32) NOT NULL DEFAULT 'flashcards',
    path           TEXT NOT NULL,
    content        BYTEA NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    byte_size      INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT notebook_files_notebook_id_family_path_key UNIQUE (notebook_id, family, path)
);
CREATE INDEX idx_notebook_files_notebook_id ON notebook_files (notebook_id);
CREATE TRIGGER notebook_files_set_updated_at BEFORE UPDATE ON notebook_files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
