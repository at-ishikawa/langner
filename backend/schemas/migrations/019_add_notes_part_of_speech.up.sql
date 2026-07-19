ALTER TABLE notes ADD COLUMN part_of_speech VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE notes DROP CONSTRAINT notes_usage_entry_key;
ALTER TABLE notes ADD CONSTRAINT notes_usage_entry_pos_key
    UNIQUE ("usage", entry, part_of_speech);
