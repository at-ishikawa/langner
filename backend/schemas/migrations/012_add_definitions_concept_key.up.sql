ALTER TABLE notes
    ADD COLUMN concept_key VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX idx_notes_concept_key ON notes (concept_key);

ALTER TABLE learning_logs
    ADD COLUMN concept_key VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX idx_learning_logs_concept_key ON learning_logs (concept_key);
