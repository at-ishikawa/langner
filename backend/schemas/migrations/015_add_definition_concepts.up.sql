CREATE TABLE IF NOT EXISTS definition_concepts (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    head VARCHAR(255) NOT NULL,
    meaning VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, head)
);
CREATE INDEX IF NOT EXISTS idx_definition_concepts_notebook_id ON definition_concepts (notebook_id);

DROP TRIGGER IF EXISTS definition_concepts_set_updated_at ON definition_concepts;
CREATE TRIGGER definition_concepts_set_updated_at BEFORE UPDATE ON definition_concepts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS definition_concept_members (
    id BIGSERIAL PRIMARY KEY,
    concept_id BIGINT NOT NULL,
    expression VARCHAR(380) NOT NULL,
    session_title VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (concept_id) REFERENCES definition_concepts(id) ON DELETE CASCADE,
    UNIQUE (concept_id, expression)
);
CREATE INDEX IF NOT EXISTS idx_definition_concept_members_expression ON definition_concept_members (expression);
