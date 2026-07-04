ALTER TABLE notebook_notes
    DROP CONSTRAINT IF EXISTS notebook_notes_note_id_notebook_type_notebook_id_group_subgroup_key;

ALTER TABLE notebook_notes
    ADD CONSTRAINT notebook_notes_note_id_notebook_type_notebook_id_group_key
    UNIQUE (note_id, notebook_type, notebook_id, "group");
