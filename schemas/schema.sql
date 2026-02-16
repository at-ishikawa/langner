-- Full schema for reference (maintained manually to match migrations)

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

CREATE TABLE notebook_notes (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL,
    notebook_type VARCHAR(50) NOT NULL,
    notebook_id VARCHAR(255) NOT NULL,
    `group` VARCHAR(255),
    subgroup VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);

CREATE INDEX idx_notebook_notes_note ON notebook_notes(note_id);
CREATE INDEX idx_notebook_notes_notebook ON notebook_notes(notebook_type, notebook_id);

CREATE TABLE note_images (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL,
    url TEXT NOT NULL,
    sort_order INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);

CREATE TABLE note_references (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL,
    link TEXT NOT NULL,
    description TEXT,
    sort_order INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);

CREATE TABLE learning_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL,
    learned_at DATETIME NOT NULL,
    quality INT,
    response_time_ms INT,
    quiz_type VARCHAR(50) NOT NULL,
    interval_days INT,
    easiness_factor DECIMAL(3,2),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id)
);

CREATE INDEX idx_learning_logs_note ON learning_logs(note_id, quiz_type);
CREATE INDEX idx_learning_logs_status ON learning_logs(note_id, quiz_type, status);
CREATE INDEX idx_learning_logs_date ON learning_logs(learned_at);

CREATE TABLE dictionary_entries (
    word VARCHAR(255) PRIMARY KEY,
    source_type VARCHAR(50) NOT NULL,
    source_url TEXT,
    response JSON,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
