UPDATE notes SET level = '' WHERE level IS NULL;

ALTER TABLE notes
    ALTER COLUMN level SET DEFAULT '',
    ALTER COLUMN level SET NOT NULL;
