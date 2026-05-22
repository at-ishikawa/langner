-- Idempotent removal — only drop if present so installations where the
-- column was added by 009 (not this repair) can still rollback to the
-- pre-009 state cleanly.

ALTER TABLE note_origin_parts
    DROP COLUMN IF EXISTS origin_form_id;
