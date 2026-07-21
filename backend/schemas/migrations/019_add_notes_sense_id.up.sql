-- Add the stable source-entry identity (sense_id) to notes.
--
-- sense_id is the canonical key for a note's identity — the readable slug
-- assigned to each source vocabulary entry (see `langner migrate assign-ids`).
-- The DB serial id stays the primary key; sense_id is the business identity.
--
-- Legacy rows (imported before ids were assigned) carry the '' default and
-- keep keying by the existing UNIQUE(usage, entry). Only id-bearing rows are
-- constrained to be unique by sense_id, via a PARTIAL unique index that
-- excludes the empty default — a plain UNIQUE(sense_id) would collide on the
-- many '' rows. Two distinct ids sharing an (usage, entry) spelling can then
-- coexist as two rows.
ALTER TABLE notes ADD COLUMN sense_id VARCHAR(128) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX notes_sense_id_key ON notes (sense_id) WHERE sense_id <> '';
