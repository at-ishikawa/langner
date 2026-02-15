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
