-- Gzip-compressed copies of the two large payloads the server reads in bulk:
-- notebook file contents (every notebook is materialized when a server
-- instance starts) and dictionary responses (loaded whole at startup).
-- Postgres sends BYTEA/JSON uncompressed over the wire, so reading the
-- compressed copy cuts the transferred bytes several-fold. The original
-- columns stay authoritative (export, pull, hashing); the copies are NULL until
-- written by the server, which backfills them on first read.
ALTER TABLE notebook_files ADD COLUMN content_gz BYTEA;
ALTER TABLE dictionary_entries ADD COLUMN response_gz BYTEA;
