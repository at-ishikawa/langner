ALTER TABLE note_origin_parts
    ADD COLUMN IF NOT EXISTS origin_form_id BIGINT NULL;
