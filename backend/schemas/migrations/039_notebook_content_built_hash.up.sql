-- The bundle hash a notebook's content rows were built from. A notebook is
-- unchanged only when both content_hash (the stored bundle) and this match the
-- incoming bundle. NULL means the rows predate this column: `migrate
-- import-content` rebuilds them, and `migrate schema` rebuilds them from the
-- stored notebook_files when those are the notebook's current bundle.
ALTER TABLE notebooks ADD COLUMN content_built_hash CHAR(64) NULL;
