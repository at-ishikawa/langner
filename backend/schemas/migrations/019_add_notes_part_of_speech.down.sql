ALTER TABLE notes DROP CONSTRAINT notes_usage_entry_pos_key;
ALTER TABLE notes ADD CONSTRAINT notes_usage_entry_key UNIQUE ("usage", entry);
ALTER TABLE notes DROP COLUMN part_of_speech;
