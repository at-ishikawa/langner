-- Reverse 022. Restore the plain UNIQUE("usage", entry) from migration 001.
--
-- NOTE: this ADD CONSTRAINT will FAIL if any homograph rows exist (two rows
-- sharing an (usage, entry) spelling with distinct sense_ids) — the whole point
-- of 022 was to allow them. That's the standard, acceptable behaviour for a
-- down migration that un-relaxes a constraint: you can only roll back if the
-- data still fits the stricter rule.
DROP INDEX IF EXISTS notes_usage_entry_legacy_key;
ALTER TABLE notes ADD CONSTRAINT notes_usage_entry_key UNIQUE ("usage", entry);
