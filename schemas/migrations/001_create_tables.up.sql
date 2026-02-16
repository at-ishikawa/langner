CREATE TABLE notes (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    `usage` VARCHAR(255) NOT NULL COMMENT 'The expression or phrase as used in context',
    entry VARCHAR(255) NOT NULL COMMENT 'The dictionary form or base word',
    meaning VARCHAR(1000) COMMENT 'Definition or meaning of the word/phrase',
    level VARCHAR(50) COMMENT 'Proficiency level (e.g., beginner, intermediate)',
    dictionary_number INT COMMENT 'Index of the selected dictionary definition',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_notes_usage (`usage`),
    INDEX idx_notes_entry (entry)
) COMMENT='Vocabulary notes for words and phrases';

CREATE TABLE notebook_notes (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL COMMENT 'Reference to the parent note',
    notebook_type VARCHAR(50) NOT NULL COMMENT 'Type of notebook: story, flashcard, or book',
    notebook_id VARCHAR(255) NOT NULL COMMENT 'Identifier of the source notebook',
    `group` VARCHAR(255) COMMENT 'Group within the notebook (e.g., episode title)',
    subgroup VARCHAR(255) COMMENT 'Subgroup within the group (e.g., scene title)',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id),
    INDEX idx_notebook_notes_note (note_id),
    INDEX idx_notebook_notes_notebook (notebook_type, notebook_id)
) COMMENT='Links notes to their source notebooks';

CREATE TABLE note_images (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL COMMENT 'Reference to the parent note',
    url VARCHAR(2048) NOT NULL COMMENT 'Image URL for visual vocabulary learning',
    sort_order INT NOT NULL COMMENT 'Display order of the image',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
) COMMENT='Images associated with vocabulary notes';

CREATE TABLE note_references (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL COMMENT 'Reference to the parent note',
    link VARCHAR(2048) NOT NULL COMMENT 'URL of the external reference',
    description VARCHAR(1000) COMMENT 'Description of the reference',
    sort_order INT NOT NULL COMMENT 'Display order of the reference',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
) COMMENT='External references for vocabulary notes';

CREATE TABLE learning_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL COMMENT 'Reference to the studied note',
    status VARCHAR(50) NOT NULL COMMENT 'Learning status: misunderstood, understood, usable, intuitive',
    learned_at DATETIME NOT NULL COMMENT 'When the study session occurred',
    quality INT COMMENT 'SM-2 quality grade (0-5)',
    response_time_ms INT COMMENT 'Response time in milliseconds',
    quiz_type VARCHAR(50) NOT NULL COMMENT 'Quiz type: notebook or reverse',
    interval_days INT COMMENT 'Days until next review (SM-2 spaced repetition)',
    easiness_factor DECIMAL(3,2) COMMENT 'SM-2 easiness factor (default 2.50)',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id),
    INDEX idx_learning_logs_note (note_id, quiz_type),
    INDEX idx_learning_logs_status (note_id, quiz_type, status),
    INDEX idx_learning_logs_date (learned_at)
) COMMENT='Spaced repetition learning history for vocabulary notes';

CREATE TABLE dictionary_entries (
    word VARCHAR(255) PRIMARY KEY COMMENT 'The looked-up word',
    source_type VARCHAR(50) NOT NULL COMMENT 'Dictionary API source (e.g., rapidapi)',
    source_url VARCHAR(2048) COMMENT 'API endpoint URL',
    response JSON COMMENT 'Full API response stored as JSON',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) COMMENT='Cached dictionary API responses';
