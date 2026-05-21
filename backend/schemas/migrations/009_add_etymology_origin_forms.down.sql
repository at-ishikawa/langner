ALTER TABLE note_origin_parts
    DROP FOREIGN KEY fk_note_origin_parts_origin_form_id;

ALTER TABLE note_origin_parts
    DROP COLUMN origin_form_id;

DROP TABLE IF EXISTS etymology_origin_forms;
