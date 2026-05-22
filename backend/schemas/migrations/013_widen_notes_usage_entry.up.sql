-- Flashcard-style entries can carry a full sentence (e.g. a joke) in the
-- `expression:` YAML field, which becomes both notes.usage and notes.entry
-- at import time. VARCHAR(255) is too tight for those (one Word Power
-- joke at ~280 chars trips Error 1406 "Data too long for column 'usage'").
--
-- Widen both columns to VARCHAR(380). The existing UNIQUE (`usage`, entry)
-- key occupies (380 + 380) * 4 = 3040 bytes under utf8mb4, comfortably
-- below the 3072-byte InnoDB / TiDB key limit, so we don't need to
-- rebuild or prefix any index. The single-column INDEX(`usage`) and
-- INDEX(entry) each occupy 1520 bytes, also fine.

ALTER TABLE notes MODIFY COLUMN `usage` VARCHAR(380) NOT NULL
    COMMENT 'The expression or phrase as used in context';

ALTER TABLE notes MODIFY COLUMN entry VARCHAR(380) NOT NULL
    COMMENT 'The dictionary form or base word';
