-- Resolve a latent schema contradiction from the homograph (#35) work.
--
-- Migration 001 created notes with a plain UNIQUE("usage", entry)
-- (constraint notes_usage_entry_key). Migration 019 added sense_id + a
-- PARTIAL unique notes_sense_id_key ON notes(sense_id) WHERE sense_id <> '',
-- and its comment describes the intended model: "Two distinct ids sharing an
-- (usage, entry) spelling can then coexist as two rows." But 019 never dropped
-- the plain UNIQUE(usage, entry), so a homograph (same spelling, distinct
-- sense_id) is still rejected — the importer dedups by (sense_id, usage, entry)
-- and keeps both rows, then BatchCreate's INSERT hits
-- "duplicate key value violates unique constraint notes_usage_entry_key"
-- (SQLSTATE 23505). The example config has no homographs, so CI never saw it.
--
-- Relax the uniqueness to what 019 intended: id-less LEGACY rows still can't
-- duplicate a spelling (they have no sense_id to tell them apart), but
-- id-bearing homographs are free to coexist — they're already kept distinct by
-- notes_sense_id_key.
ALTER TABLE notes DROP CONSTRAINT notes_usage_entry_key;

-- Partial unique for legacy id-less rows only. Named *_legacy_key to make its
-- scope clear (it is an index now, not a table constraint). The id-less
-- ensureNoteExists upsert infers this index via
-- ON CONFLICT ("usage", entry) WHERE sense_id = ''.
CREATE UNIQUE INDEX notes_usage_entry_legacy_key ON notes ("usage", entry) WHERE sense_id = '';
