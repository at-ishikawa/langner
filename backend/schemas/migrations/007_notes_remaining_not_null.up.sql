-- Same fix as 006 but for the remaining columns whose Go fields are plain
-- (non-nullable) types. Splitting from 006 because that migration was
-- already applied with only the level column on some installations.
--
-- subgroup is left as TEXT (it was widened in migration 002 to hold long
-- scene titles); we just add NOT NULL without redefining the type so we
-- don't truncate existing rows.

UPDATE notes SET meaning = '' WHERE meaning IS NULL;
UPDATE notes SET dictionary_number = 0 WHERE dictionary_number IS NULL;

ALTER TABLE notes
    MODIFY COLUMN meaning TEXT NOT NULL
        COMMENT 'Definition or meaning of the word/phrase',
    MODIFY COLUMN dictionary_number INT NOT NULL DEFAULT 0
        COMMENT 'Index of the selected dictionary definition';

UPDATE notebook_notes SET `group` = '' WHERE `group` IS NULL;
UPDATE notebook_notes SET subgroup = '' WHERE subgroup IS NULL;

ALTER TABLE notebook_notes
    MODIFY COLUMN `group` VARCHAR(255) NOT NULL DEFAULT ''
        COMMENT 'Group within the notebook (e.g., episode title)',
    MODIFY COLUMN subgroup TEXT NOT NULL
        COMMENT 'Subgroup within the group (e.g., scene title)';
