-- Grammar becomes a first-class DB citizen, mirroring the etymology
-- origin_id pattern (migration 020). Grammar corrections are neither notes
-- nor etymology origins, so before this they had no DB home and the runtime
-- read them from a YAML merge; now their learning logs live in the database
-- like vocabulary and etymology.
--
-- grammar_corrections stores only the correction's IDENTITY — its stable
-- sense_id (e.g. a correction slug) within a grammar/journal notebook. The
-- correction's TEXT and "incorrect" span are reference content that stays in
-- the read-only grammars notebooks; the learning-history layer only needs to
-- key a correction's log series, exactly like etymology_origins keys an
-- origin's logs.
CREATE TABLE grammar_corrections (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    sense_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, sense_id)
);
CREATE INDEX idx_grammar_corrections_notebook_id ON grammar_corrections (notebook_id);
CREATE TRIGGER grammar_corrections_set_updated_at BEFORE UPDATE ON grammar_corrections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- learning_logs extension: grammar quizzes target corrections, not notes or
-- origins. correction_id joins the trio (note_id / origin_id / correction_id);
-- exactly one is set per row. Nullable + ON DELETE CASCADE, exactly like
-- origin_id in migration 020.
ALTER TABLE learning_logs ADD COLUMN correction_id BIGINT NULL;
ALTER TABLE learning_logs
    ADD CONSTRAINT fk_learning_logs_correction_id
    FOREIGN KEY (correction_id) REFERENCES grammar_corrections(id) ON DELETE CASCADE;
CREATE INDEX idx_learning_logs_correction_id ON learning_logs (correction_id);
