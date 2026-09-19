-- Notebook ownership + public/private visibility (auth Phase 3). An OVERLAY
-- table over the existing denormalized notebook_id strings: content stays a
-- shared global catalog, and access is enforced at the notebook layer.
--
-- A notebook_id ABSENT from this table is treated as PUBLIC and unowned
-- (unlisted = public), so importing/seeding notebooks before any ownership is
-- assigned leaves every book visible to all logged-in users — the pre-Phase-3
-- behavior. Assigning ownership is a deliberate admin/config action
-- (`notebook_ownership:` config, `langner notebooks set-owner`).
--
-- visibility is 'public' (visible to every logged-in user) or 'private'
-- (visible only to owner_user_id). owner_user_id is NULLABLE: a private
-- notebook with a NULL owner is visible to nobody but stays in the catalog
-- (ON DELETE SET NULL keeps the row when the owning account is removed).
--
-- Phase 3 deliberately uses migration 027: Phase 2 took 026, and golang-migrate
-- orders by version, so a lower number added after 026 would never re-run on an
-- already-migrated database. The 025 gap the 026 comment reserved is left
-- unused rather than filled out of order.
--
-- No foreign key is added FROM the existing notebook_id string columns
-- (learning_logs.source_notebook_id, notebook_notes.notebook_id, …) — they stay
-- denormalized strings, exactly as before.
CREATE TABLE notebooks (
    notebook_id VARCHAR(255) PRIMARY KEY,
    owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    visibility VARCHAR(16) NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_notebooks_owner_user_id ON notebooks (owner_user_id);
CREATE TRIGGER notebooks_set_updated_at BEFORE UPDATE ON notebooks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
