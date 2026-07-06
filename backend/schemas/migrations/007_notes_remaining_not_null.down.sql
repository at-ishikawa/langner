ALTER TABLE notes
    ALTER COLUMN meaning DROP NOT NULL,
    ALTER COLUMN dictionary_number DROP DEFAULT,
    ALTER COLUMN dictionary_number DROP NOT NULL;

ALTER TABLE notebook_notes
    ALTER COLUMN "group" DROP DEFAULT,
    ALTER COLUMN "group" DROP NOT NULL,
    ALTER COLUMN subgroup DROP NOT NULL;
