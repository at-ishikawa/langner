CREATE TABLE etymology_origins (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    notebook_id VARCHAR(255) NOT NULL COMMENT 'Identifier of the etymology notebook',
    origin VARCHAR(255) NOT NULL COMMENT 'The root, prefix, or suffix',
    type VARCHAR(50) NOT NULL COMMENT 'Type: prefix, suffix, root',
    language VARCHAR(100) NOT NULL COMMENT 'Source language (e.g., Latin, Greek)',
    meaning TEXT NOT NULL COMMENT 'Meaning of this origin part',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE (notebook_id, origin, language),
    INDEX (notebook_id)
) COMMENT='Etymology origin parts (roots, prefixes, suffixes)';

CREATE TABLE note_origin_parts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id BIGINT NOT NULL COMMENT 'Reference to the note',
    origin_id BIGINT NOT NULL COMMENT 'Reference to the etymology origin',
    sort_order INT NOT NULL COMMENT 'Order of the origin part within the note',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (note_id) REFERENCES notes(id),
    FOREIGN KEY (origin_id) REFERENCES etymology_origins(id),
    INDEX (note_id),
    INDEX (origin_id)
) COMMENT='Junction table linking notes to their etymology origin parts';
