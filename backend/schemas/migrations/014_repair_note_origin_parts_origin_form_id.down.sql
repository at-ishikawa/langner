-- Idempotent removal — only drop if present so installations where the
-- column was added by 009 (not this repair) can still roll back to the
-- pre-009 state cleanly. Uses the same INFORMATION_SCHEMA precheck
-- pattern as the up-migration since DROP COLUMN IF EXISTS isn't
-- supported on vanilla MySQL.

SET @col_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'note_origin_parts'
      AND COLUMN_NAME = 'origin_form_id'
);

SET @stmt := IF(@col_exists = 1,
    'ALTER TABLE note_origin_parts DROP COLUMN origin_form_id',
    'DO 0'
);

PREPARE repair_014_down FROM @stmt;
EXECUTE repair_014_down;
DEALLOCATE PREPARE repair_014_down;
