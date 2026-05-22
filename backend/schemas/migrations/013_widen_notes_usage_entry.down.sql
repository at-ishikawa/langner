-- Downgrade is best-effort: if rows now hold values longer than 255
-- characters this ALTER will fail. Callers are expected to clean up
-- oversized rows before reverting.

ALTER TABLE notes MODIFY COLUMN `usage` VARCHAR(255) NOT NULL
    COMMENT 'The expression or phrase as used in context';

ALTER TABLE notes MODIFY COLUMN entry VARCHAR(255) NOT NULL
    COMMENT 'The dictionary form or base word';
