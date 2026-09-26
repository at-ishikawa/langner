-- Per-user notebooks: DB-backed content storage (design
-- docs/content/proposals/per-user-notebooks). Two additive changes, no data
-- migration — every existing row stays valid unchanged.
--
-- 1. notebook_files: the raw YAML file bytes of a user-pushed notebook bundle,
--    one row per file (index.yml + the files it references). This is the
--    canonical stored form: at reader-construction time these exact bytes are
--    materialized and parsed by the SAME notebook parsers the filesystem
--    catalog uses, so a shipped notebook and a user (nb_) notebook differ only
--    in where the bytes came from — no second parse path (see
--    .claude/rules/verify-data-features-with-example-notebooks.md).
--
--    notebook_id is a plain string with NO foreign key, matching the
--    established notebook_notes.notebook_id / learning_logs.source_notebook_id
--    convention (migration 027 header). It carries the server-minted nb_ id.
CREATE TABLE notebook_files (
    id             BIGSERIAL PRIMARY KEY,
    notebook_id    VARCHAR(255) NOT NULL,
    path           TEXT NOT NULL,           -- relative path within the pushed dir (e.g. "index.yml")
    content        BYTEA NOT NULL,          -- exact file bytes
    content_sha256 CHAR(64) NOT NULL,       -- per-file integrity
    byte_size      INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, path)
);
CREATE INDEX idx_notebook_files_notebook_id ON notebook_files (notebook_id);
CREATE TRIGGER notebook_files_set_updated_at BEFORE UPDATE ON notebook_files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2. Extend the notebooks ownership overlay (migration 027) with provenance +
--    the metadata the reader needs to enumerate DB notebooks without parsing
--    every blob up front. All additive/nullable-or-defaulted: existing rows
--    become source='shipped' with NULL kind/display_name/content_hash, which
--    preserves their pre-existing behavior exactly.
ALTER TABLE notebooks ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT 'shipped'
    CHECK (source IN ('shipped', 'user'));
ALTER TABLE notebooks ADD COLUMN kind VARCHAR(32) NULL;          -- Story|Flashcard|Etymology|Definitions|Journal|Grammar|Book
ALTER TABLE notebooks ADD COLUMN display_name TEXT NULL;         -- index.yml `name:`
ALTER TABLE notebooks ADD COLUMN content_hash CHAR(64) NULL;     -- hash of the pushed bundle (pull/dedup/version)
