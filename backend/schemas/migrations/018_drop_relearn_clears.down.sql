-- Recreate relearn_clears to roll back 018. See 017 for the full rationale of
-- the original marker table.
CREATE TABLE relearn_clears (
    clear_key VARCHAR(512) NOT NULL,
    cleared_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (clear_key)
);
CREATE INDEX idx_relearn_clears_cleared_at ON relearn_clears (cleared_at);
