-- semantic_concepts groups origins that share the same underlying concept.
-- The same concept_key may appear in multiple sessions of the same book; the
-- ingestion layer unifies them by merging members. Meaning/note are taken
-- from the first declaration.
CREATE TABLE semantic_concepts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL,
    concept_key VARCHAR(255) NOT NULL,
    meaning VARCHAR(255) NOT NULL,
    note TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, concept_key),
    INDEX (notebook_id)
) COMMENT='Book-level semantic concepts grouping etymology origins';

-- semantic_concept_members links a semantic_concepts row to one or more
-- etymology_origins rows. session_title records which session contributed
-- the membership (origins are session-scoped). (concept_id, origin_id) is
-- unique so multiple sessions declaring the same origin dedupe.
CREATE TABLE semantic_concept_members (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    concept_id BIGINT NOT NULL,
    origin_id BIGINT NOT NULL,
    session_title VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (concept_id) REFERENCES semantic_concepts(id) ON DELETE CASCADE,
    FOREIGN KEY (origin_id)  REFERENCES etymology_origins(id)  ON DELETE CASCADE,
    UNIQUE (concept_id, origin_id),
    INDEX (origin_id)
) COMMENT='Junction table linking semantic concepts to etymology origins';

-- concept_relations represents typed edges between concept keys. Symmetric
-- relations (between: [A, B]) are materialised as two rows (A->B, B->A)
-- with is_directed=false; directed relations write one row with
-- is_directed=true.
CREATE TABLE concept_relations (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL,
    type VARCHAR(64) NOT NULL,
    from_concept_id BIGINT NOT NULL,
    to_concept_id BIGINT NOT NULL,
    is_directed BOOLEAN NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (from_concept_id) REFERENCES semantic_concepts(id) ON DELETE CASCADE,
    FOREIGN KEY (to_concept_id)   REFERENCES semantic_concepts(id) ON DELETE CASCADE,
    UNIQUE (type, from_concept_id, to_concept_id),
    INDEX (notebook_id),
    INDEX (from_concept_id)
) COMMENT='Typed relations between semantic concepts';
