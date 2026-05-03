ALTER TABLE notes
    MODIFY COLUMN meaning TEXT NULL
        COMMENT 'Definition or meaning of the word/phrase',
    MODIFY COLUMN dictionary_number INT NULL DEFAULT NULL
        COMMENT 'Index of the selected dictionary definition';

ALTER TABLE notebook_notes
    MODIFY COLUMN `group` VARCHAR(255) NULL DEFAULT NULL
        COMMENT 'Group within the notebook (e.g., episode title)',
    MODIFY COLUMN subgroup TEXT NULL
        COMMENT 'Subgroup within the group (e.g., scene title)';
