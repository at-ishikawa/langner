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
