-- Composite multi-family notebooks (design per-user-notebooks §13).
--
-- A langner notebook id is registered under SEVERAL family folders at once
-- (e.g. `latin-roots-book` lives in both examples/definitions/ and
-- examples/etymology/; `frankenstein` in definitions/ and books/). That is how
-- a word's definition (definitions family) links to its origin (etymology
-- family). The original notebook_files model — UNIQUE (notebook_id, path) with
-- one `kind` per notebook — cannot represent it: the two family index.yml files
-- collide on path, and one `kind` cannot mean "definitions AND etymology".
--
-- Add a `family` dimension so each stored file records which family bucket it
-- belongs to, mirroring the filesystem layout. DBContentSource then materializes
-- <family>/<notebook_id>/<path> per file's family, and one id can register in
-- several reader family maps exactly like the filesystem walk. notebooks.kind
-- stays as primary/display metadata only; it no longer decides family placement.
ALTER TABLE notebook_files ADD COLUMN family VARCHAR(32) NOT NULL DEFAULT 'flashcards';

-- Backfill existing single-family (source='user') rows from their notebook kind,
-- using the same kind->family mapping familyDir() applies in code.
UPDATE notebook_files f SET family = CASE lower(coalesce((SELECT n.kind FROM notebooks n WHERE n.notebook_id = f.notebook_id), ''))
    WHEN 'story'       THEN 'stories'
    WHEN 'journal'     THEN 'journals'
    WHEN 'grammar'     THEN 'grammars'
    WHEN 'book'        THEN 'books'
    WHEN 'definitions' THEN 'definitions'
    WHEN 'definition'  THEN 'definitions'
    WHEN 'etymology'   THEN 'etymology'
    ELSE 'flashcards'
END;

-- One file is identified by (notebook_id, family, path): the same relative path
-- (e.g. index.yml) may exist once per family for a composite id.
ALTER TABLE notebook_files DROP CONSTRAINT IF EXISTS notebook_files_notebook_id_path_key;
ALTER TABLE notebook_files ADD  CONSTRAINT notebook_files_notebook_id_family_path_key UNIQUE (notebook_id, family, path);
