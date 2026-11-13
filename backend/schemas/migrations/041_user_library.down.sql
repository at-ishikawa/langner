DROP TABLE IF EXISTS user_library;
UPDATE notebooks SET source = 'shipped' WHERE source = 'library';
ALTER TABLE notebooks DROP CONSTRAINT IF EXISTS notebooks_source_check;
ALTER TABLE notebooks ADD CONSTRAINT notebooks_source_check CHECK (source IN ('shipped', 'user'));
