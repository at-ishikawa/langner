-- Repair migration. golang-migrate marked 009 as applied on TiDB even
-- though its original combined ADD COLUMN + ADD CONSTRAINT in one
-- ALTER TABLE failed mid-way (Error 1072 from the FK check running
-- before the column was visible). The partial run left
-- etymology_origin_forms created but note_origin_parts.origin_form_id
-- missing. The split-009 fix unblocks fresh installs, but golang-migrate
-- does not replay an already-applied version when its file changes —
-- so existing TiDB instances still miss the column. This migration adds
-- it forward-only, idempotent against installations where 009 succeeded
-- normally (e.g. CI's MySQL).
--
-- Idempotency strategy: precheck INFORMATION_SCHEMA via session
-- variables + PREPARE/EXECUTE. The column-existence check + conditional
-- ALTER works on both MySQL (no ADD COLUMN IF NOT EXISTS support) and
-- TiDB. `DO 0` is the no-op branch.
--
-- The foreign key constraint is intentionally not recreated here:
-- referential integrity is managed by the ingestion reconcile pass at
-- the application layer; the only behaviour we'd lose is ON DELETE SET
-- NULL when an etymology_origin_form is removed, which isn't a code
-- path the application exercises after a re-import.

SET @col_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'note_origin_parts'
      AND COLUMN_NAME = 'origin_form_id'
);

SET @stmt := IF(@col_exists = 0,
    'ALTER TABLE note_origin_parts ADD COLUMN origin_form_id BIGINT NULL AFTER origin_id',
    'DO 0'
);

PREPARE repair_014 FROM @stmt;
EXECUTE repair_014;
DEALLOCATE PREPARE repair_014;
