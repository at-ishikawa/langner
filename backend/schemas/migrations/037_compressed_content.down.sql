-- Reverse 037: drop the compressed copies (the original columns are untouched).
ALTER TABLE notebook_files DROP COLUMN IF EXISTS content_gz;
ALTER TABLE dictionary_entries DROP COLUMN IF EXISTS response_gz;
