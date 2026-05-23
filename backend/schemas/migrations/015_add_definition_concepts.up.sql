-- definition_concepts persists the per-book word-concept declarations that
-- back the notes.concept_key cache column (migration 012). A concept_key
-- is the head expression — unique within a book — and meaning is the
-- umbrella sense shared by all member expressions. Forward-only safe:
-- IF NOT EXISTS lets re-runs on partially-created schemas succeed.
CREATE TABLE IF NOT EXISTS definition_concepts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL,
    head VARCHAR(255) NOT NULL,
    meaning VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, head),
    INDEX (notebook_id)
) COMMENT='Book-level word concepts grouping member expressions under one head';

-- definition_concept_members links a concept to its member expressions.
-- Expression is VARCHAR(380) to match notes.entry (migration 013); the
-- unique key (concept_id, expression) at 8+380 chars * 4-byte utf8mb4
-- stays under the 3072-byte InnoDB key limit. session_title records
-- which session declared the membership (concepts can span sessions in
-- the same book; the first declaration wins for meaning).
CREATE TABLE IF NOT EXISTS definition_concept_members (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    concept_id BIGINT NOT NULL,
    expression VARCHAR(380) NOT NULL,
    session_title VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (concept_id) REFERENCES definition_concepts(id) ON DELETE CASCADE,
    UNIQUE (concept_id, expression),
    INDEX (expression)
) COMMENT='Junction table linking definition concepts to member expressions';
