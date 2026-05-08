ALTER TABLE notes
    MODIFY COLUMN level VARCHAR(50) NULL DEFAULT NULL
        COMMENT 'Proficiency level (e.g., beginner, intermediate)';
