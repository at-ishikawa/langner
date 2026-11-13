-- Shared books (Project Gutenberg imports) are stored once and read by every
-- user who imports them, but each user's library is their own: importing a
-- book adds it to the importer's library only. source='library' marks such a
-- notebook; user_library lists who has it. A library notebook is hidden from
-- users who have not added it.
ALTER TABLE notebooks DROP CONSTRAINT IF EXISTS notebooks_source_check;
ALTER TABLE notebooks ADD CONSTRAINT notebooks_source_check
    CHECK (source IN ('shipped', 'user', 'library'));

CREATE TABLE user_library (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notebook_id VARCHAR(255) NOT NULL REFERENCES notebooks(notebook_id) ON DELETE CASCADE,
    added_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, notebook_id)
);

UPDATE notebooks SET source = 'library' WHERE notebook_id LIKE 'gutenberg-%';

-- Keep every existing book where it is: a user who highlighted or studied a
-- book keeps it, and a book nobody has used stays with every existing user.
INSERT INTO user_library (user_id, notebook_id)
SELECT d.user_id, d.notebook_id FROM user_notebook_definitions d
JOIN notebooks n ON n.notebook_id = d.notebook_id AND n.source = 'library'
UNION
SELECT l.user_id, l.source_notebook_id FROM learning_logs l
JOIN notebooks n ON n.notebook_id = l.source_notebook_id AND n.source = 'library'
WHERE l.user_id IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO user_library (user_id, notebook_id)
SELECT u.id, n.notebook_id FROM users u CROSS JOIN notebooks n
WHERE n.source = 'library'
  AND NOT EXISTS (SELECT 1 FROM user_library ul WHERE ul.notebook_id = n.notebook_id)
ON CONFLICT DO NOTHING;
