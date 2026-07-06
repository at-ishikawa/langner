CREATE TABLE semantic_concepts (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    concept_key VARCHAR(255) NOT NULL,
    meaning VARCHAR(255) NOT NULL,
    note TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, concept_key)
);
CREATE INDEX idx_semantic_concepts_notebook_id ON semantic_concepts (notebook_id);
CREATE TRIGGER semantic_concepts_set_updated_at BEFORE UPDATE ON semantic_concepts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE semantic_concept_members (
    id BIGSERIAL PRIMARY KEY,
    concept_id BIGINT NOT NULL,
    origin_id BIGINT NOT NULL,
    session_title VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (concept_id) REFERENCES semantic_concepts(id) ON DELETE CASCADE,
    FOREIGN KEY (origin_id)  REFERENCES etymology_origins(id)  ON DELETE CASCADE,
    UNIQUE (concept_id, origin_id)
);
CREATE INDEX idx_semantic_concept_members_origin_id ON semantic_concept_members (origin_id);

CREATE TABLE concept_relations (
    id BIGSERIAL PRIMARY KEY,
    notebook_id VARCHAR(255) NOT NULL,
    type VARCHAR(64) NOT NULL,
    from_concept_id BIGINT NOT NULL,
    to_concept_id BIGINT NOT NULL,
    is_directed BOOLEAN NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (from_concept_id) REFERENCES semantic_concepts(id) ON DELETE CASCADE,
    FOREIGN KEY (to_concept_id)   REFERENCES semantic_concepts(id) ON DELETE CASCADE,
    UNIQUE (type, from_concept_id, to_concept_id)
);
CREATE INDEX idx_concept_relations_notebook_id ON concept_relations (notebook_id);
CREATE INDEX idx_concept_relations_from_concept_id ON concept_relations (from_concept_id);
CREATE TRIGGER concept_relations_set_updated_at BEFORE UPDATE ON concept_relations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
