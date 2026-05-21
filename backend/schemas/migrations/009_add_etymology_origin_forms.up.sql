-- etymology_origin_forms records inflectional / morphological variants of
-- a source-language origin. Each row is one (role, form) pair scoped to an
-- origin. Used to pin a definition's origin_part reference to the specific
-- form it derives from (e.g. the supine `missum` of Latin `mittere`).

CREATE TABLE etymology_origin_forms (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    origin_id BIGINT NOT NULL COMMENT 'Reference to the etymology origin',
    form VARCHAR(255) NOT NULL COMMENT 'The form string (e.g. mittere, missum)',
    role VARCHAR(100) NOT NULL COMMENT 'Free-form role label (e.g. infinitive, supine)',
    note TEXT COMMENT 'Optional commentary on this form',
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_etymology_origin_forms_origin_id
        FOREIGN KEY (origin_id) REFERENCES etymology_origins(id) ON DELETE CASCADE,
    UNIQUE (origin_id, role, form),
    INDEX (origin_id)
) COMMENT='Inflectional / morphological forms of etymology origins';

-- note_origin_parts.origin_form_id pins the part reference to a specific
-- form on the referenced origin. NULL means the reference is generic.
ALTER TABLE note_origin_parts
    ADD COLUMN origin_form_id BIGINT NULL AFTER origin_id,
    ADD CONSTRAINT fk_note_origin_parts_origin_form_id
        FOREIGN KEY (origin_form_id) REFERENCES etymology_origin_forms(id) ON DELETE SET NULL;
