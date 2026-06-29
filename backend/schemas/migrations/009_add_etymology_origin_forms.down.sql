ALTER TABLE note_origin_parts
    DROP CONSTRAINT IF EXISTS fk_note_origin_parts_origin_form_id;

ALTER TABLE note_origin_parts
    DROP COLUMN IF EXISTS origin_form_id;

DROP TABLE IF EXISTS etymology_origin_forms;
