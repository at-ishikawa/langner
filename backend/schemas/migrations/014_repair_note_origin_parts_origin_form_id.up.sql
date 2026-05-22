-- Repair migration. golang-migrate marked 009 as applied on TiDB even
-- though its original combined ADD COLUMN + ADD CONSTRAINT in one
-- ALTER TABLE failed mid-way (Error 1072 from the FK check running
-- before the column was visible). The partial run left
-- etymology_origin_forms created but note_origin_parts.origin_form_id
-- missing. PR's split-009 fix unblocks fresh installs, but golang-migrate
-- does not replay an already-applied version when its file changes —
-- so existing instances still miss the column. This migration adds it
-- forward-only, idempotent against installations where 009 succeeded
-- normally (e.g. local MySQL).
--
-- The foreign key constraint is intentionally not recreated here:
-- MySQL doesn't accept "ADD CONSTRAINT IF NOT EXISTS", and the
-- application correctness path doesn't depend on the FK (the ingestion
-- layer manages referential integrity via the reconcile pass). The
-- only behaviour we'd lose is ON DELETE SET NULL when an
-- etymology_origin_form is removed; in practice this codepath isn't
-- exercised after a re-import (origins are recreated by ImportEtymology,
-- and origin_form_id values are repopulated then).

ALTER TABLE note_origin_parts
    ADD COLUMN IF NOT EXISTS origin_form_id BIGINT NULL AFTER origin_id;
