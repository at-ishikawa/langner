-- Enforce that every learning-history row belongs to a user (auth Phase 2
-- follow-up). Migration 026 added user_id as NULLABLE so pre-auth rows could
-- survive and be backfilled; now that the cutover is complete (all pre-auth
-- rows claimed to their owner) the column becomes NOT NULL, so Postgres — not
-- just application code — guarantees no ownerless history can exist.
--
-- Writers that must satisfy this: the runtime save path already rejects a zero
-- user_id (the Create guard), and the import/seed path now attributes every row
-- to the resolved owner AT INSERT (StateSeeder.ownerID); the table-dump restore
-- carries each row's original user_id verbatim.
--
-- This ALTER fails if any NULL row remains — that is intentional: it forces a
-- claim (`langner auth backfill --user-id <id>`) before the constraint lands,
-- rather than silently dropping unowned history.
ALTER TABLE learning_logs ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE note_skip_flags ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE origin_skip_flags ALTER COLUMN user_id SET NOT NULL;
