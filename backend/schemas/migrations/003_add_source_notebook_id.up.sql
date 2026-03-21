SET @col_exists = (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_logs' AND COLUMN_NAME = 'source_notebook_id');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE learning_logs ADD COLUMN source_notebook_id VARCHAR(255) NOT NULL DEFAULT '''' COMMENT ''Notebook ID from which this log was imported'' AFTER easiness_factor', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_logs' AND INDEX_NAME = 'idx_note_id');
SET @sql = IF(@idx_exists = 0, 'ALTER TABLE learning_logs ADD INDEX idx_note_id (note_id)', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'learning_logs' AND INDEX_NAME = 'note_id');
SET @sql = IF(@idx_exists > 0, 'ALTER TABLE learning_logs DROP INDEX note_id', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
