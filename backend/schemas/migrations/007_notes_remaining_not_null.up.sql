UPDATE notes SET meaning = '' WHERE meaning IS NULL;
UPDATE notes SET dictionary_number = 0 WHERE dictionary_number IS NULL;

ALTER TABLE notes
    ALTER COLUMN meaning SET NOT NULL,
    ALTER COLUMN dictionary_number SET DEFAULT 0,
    ALTER COLUMN dictionary_number SET NOT NULL;

UPDATE notebook_notes SET "group" = '' WHERE "group" IS NULL;
UPDATE notebook_notes SET subgroup = '' WHERE subgroup IS NULL;

ALTER TABLE notebook_notes
    ALTER COLUMN "group" SET DEFAULT '',
    ALTER COLUMN "group" SET NOT NULL,
    ALTER COLUMN subgroup SET NOT NULL;
