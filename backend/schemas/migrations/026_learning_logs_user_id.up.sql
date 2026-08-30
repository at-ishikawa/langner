-- Per-user learning history (auth Phase 2). Adds an owning user_id to the
-- three per-user STATE tables so every learning attempt and every exclude
-- marker belongs to exactly one account. Phase 1 added the users table
-- (migration 024); Phase 3 will add notebook ownership/visibility (migration
-- 025 — deliberately reserved, hence the gap here).
--
-- user_id is NULLABLE on purpose: existing rows (imported/seeded before auth
-- existed) survive the migration with user_id = NULL and are backfilled to the
-- configured initial-admin account by `langner auth provision` (also wired into
-- `migrate import-db`). ON DELETE CASCADE removes an account's history when the
-- account is deleted.
ALTER TABLE learning_logs
    ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE note_skip_flags
    ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE origin_skip_flags
    ADD COLUMN user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;

-- Per-user read paths filter by user_id; the composite index matches the
-- analytics/quiz read shape (a user's logs within one notebook).
CREATE INDEX idx_learning_logs_user_id ON learning_logs (user_id);
CREATE INDEX idx_learning_logs_user_notebook ON learning_logs (user_id, source_notebook_id);
CREATE INDEX idx_note_skip_flags_user_id ON note_skip_flags (user_id);
CREATE INDEX idx_origin_skip_flags_user_id ON origin_skip_flags (user_id);
