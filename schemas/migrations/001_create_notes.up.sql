CREATE TABLE notes (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    `usage` VARCHAR(255) NOT NULL,
    entry VARCHAR(255) NOT NULL,
    meaning TEXT,
    level VARCHAR(50),
    dictionary_number INT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE INDEX idx_notes_usage ON notes(`usage`);
CREATE INDEX idx_notes_entry ON notes(entry);
