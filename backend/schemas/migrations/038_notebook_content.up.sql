-- Notebook content as relational rows.
--
-- Every notebook family (stories, books, journals, flashcards, definitions,
-- etymology, grammars) is stored as rows, so the server queries only the
-- notebooks a request needs instead of reading whole YAML files. The YAML
-- bundle a learner pushes (or the CLI imports) is parsed once, on write, into
-- this hierarchy:
--
--   content_indexes   one family of a notebook (its index.yml: kind, name)
--   content_files     a file the index lists, in index order, with its shape
--   content_entries   one top-level item of a file: a story episode, a
--                     flashcard deck, a definitions / etymology session, a
--                     grammar entry
--   content_scenes    a scene of an entry (decks and legacy etymology sessions
--                     have one untitled scene)
--   content_lines     a scene's conversation lines and statements
--   content_notes     a word in a scene (story definitions, deck cards,
--                     definitions expressions, etymology definitions), plus
--                     its examples / origin parts / phrases / references
--   content_origins   an etymology origin in a scene, plus its forms
--   content_*concepts, content_relations, content_corrections
--
-- Rows are keyed by their position in the bundle (notebook, family, file,
-- entry, scene, item numbers), so every table is read with
-- WHERE notebook_id = ANY(...) on its primary key, and replacing a notebook is
-- one DELETE that cascades.
CREATE TABLE content_indexes (
    notebook_id VARCHAR(255) NOT NULL REFERENCES notebooks(notebook_id) ON DELETE CASCADE,
    family      VARCHAR(32)  NOT NULL,
    kind        TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family)
);

CREATE TABLE content_files (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    path        TEXT NOT NULL,
    -- story | flashcard | definitions | etymology | etymology_legacy | grammar
    shape       VARCHAR(32) NOT NULL,
    PRIMARY KEY (notebook_id, family, file_no),
    FOREIGN KEY (notebook_id, family) REFERENCES content_indexes ON DELETE CASCADE
);

CREATE TABLE content_entries (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    -- story event / deck title / definitions, etymology or grammar title
    title       TEXT NOT NULL DEFAULT '',
    -- definitions metadata.notebook (the story file the session annotates)
    session_key TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    series      TEXT NOT NULL DEFAULT '',
    season      INT  NOT NULL DEFAULT 0,
    episode     INT  NOT NULL DEFAULT 0,
    date        TIMESTAMPTZ NULL,
    PRIMARY KEY (notebook_id, family, file_no, entry_no),
    FOREIGN KEY (notebook_id, family, file_no) REFERENCES content_files ON DELETE CASCADE
);

CREATE TABLE content_scenes (
    notebook_id  VARCHAR(255) NOT NULL,
    family       VARCHAR(32)  NOT NULL,
    file_no      INT  NOT NULL,
    entry_no     INT  NOT NULL,
    scene_no     INT  NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL DEFAULT '',
    -- definitions / grammar scene metadata: index, and the alternative `scene`
    scene_index  INT  NOT NULL DEFAULT 0,
    scene_number INT  NULL,
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no) REFERENCES content_entries ON DELETE CASCADE
);

CREATE TABLE content_lines (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    scene_no    INT  NOT NULL,
    line_no     INT  NOT NULL,
    -- NULL for a statement; the speaker of a conversation line otherwise
    speaker     TEXT NULL,
    text        TEXT NOT NULL,
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, line_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no) REFERENCES content_scenes ON DELETE CASCADE
);

CREATE TABLE content_notes (
    notebook_id          VARCHAR(255) NOT NULL,
    family               VARCHAR(32)  NOT NULL,
    file_no              INT  NOT NULL,
    entry_no             INT  NOT NULL,
    scene_no             INT  NOT NULL,
    note_no              INT  NOT NULL,
    sense_id             TEXT NOT NULL DEFAULT '',
    expression           TEXT NOT NULL DEFAULT '',
    definition           TEXT NOT NULL DEFAULT '',
    level                TEXT NOT NULL DEFAULT '',
    meaning              TEXT NOT NULL DEFAULT '',
    origin               TEXT NOT NULL DEFAULT '',
    part_of_speech       TEXT NOT NULL DEFAULT '',
    pronunciation        TEXT NOT NULL DEFAULT '',
    memo                 TEXT NOT NULL DEFAULT '',
    note                 TEXT NOT NULL DEFAULT '',
    reference            TEXT NOT NULL DEFAULT '',
    youtube_url          TEXT NOT NULL DEFAULT '',
    youtube_time_seconds INT  NOT NULL DEFAULT 0,
    dictionary_number    INT  NOT NULL DEFAULT 0,
    not_used             BOOLEAN NOT NULL DEFAULT FALSE,
    images               TEXT[] NOT NULL DEFAULT '{}',
    synonyms             TEXT[] NOT NULL DEFAULT '{}',
    antonyms             TEXT[] NOT NULL DEFAULT '{}',
    links                TEXT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, note_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no) REFERENCES content_scenes ON DELETE CASCADE
);

CREATE TABLE content_note_examples (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT NOT NULL,
    entry_no    INT NOT NULL,
    scene_no    INT NOT NULL,
    note_no     INT NOT NULL,
    item_no     INT NOT NULL,
    text        TEXT NOT NULL,
    highlight   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, note_no, item_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no, note_no) REFERENCES content_notes ON DELETE CASCADE
);

CREATE TABLE content_note_origin_parts (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT NOT NULL,
    entry_no    INT NOT NULL,
    scene_no    INT NOT NULL,
    note_no     INT NOT NULL,
    item_no     INT NOT NULL,
    origin      TEXT NOT NULL,
    language    TEXT NOT NULL DEFAULT '',
    sense       TEXT NOT NULL DEFAULT '',
    from_form   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, note_no, item_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no, note_no) REFERENCES content_notes ON DELETE CASCADE
);

-- A note's `statements` (actor + remarks).
CREATE TABLE content_note_phrases (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT NOT NULL,
    entry_no    INT NOT NULL,
    scene_no    INT NOT NULL,
    note_no     INT NOT NULL,
    item_no     INT NOT NULL,
    actor       TEXT NOT NULL DEFAULT '',
    remarks     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, note_no, item_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no, note_no) REFERENCES content_notes ON DELETE CASCADE
);

CREATE TABLE content_note_references (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT NOT NULL,
    entry_no    INT NOT NULL,
    scene_no    INT NOT NULL,
    note_no     INT NOT NULL,
    item_no     INT NOT NULL,
    url         TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, note_no, item_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no, note_no) REFERENCES content_notes ON DELETE CASCADE
);

CREATE TABLE content_origins (
    notebook_id   VARCHAR(255) NOT NULL,
    family        VARCHAR(32)  NOT NULL,
    file_no       INT  NOT NULL,
    entry_no      INT  NOT NULL,
    scene_no      INT  NOT NULL,
    origin_no     INT  NOT NULL,
    origin        TEXT NOT NULL,
    type          TEXT NOT NULL DEFAULT '',
    language      TEXT NOT NULL DEFAULT '',
    meaning       TEXT NOT NULL DEFAULT '',
    sense         TEXT NOT NULL DEFAULT '',
    note          TEXT NOT NULL DEFAULT '',
    english_forms TEXT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, origin_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no) REFERENCES content_scenes ON DELETE CASCADE
);

CREATE TABLE content_origin_forms (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    scene_no    INT  NOT NULL,
    origin_no   INT  NOT NULL,
    item_no     INT  NOT NULL,
    form        TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, origin_no, item_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no, origin_no) REFERENCES content_origins ON DELETE CASCADE
);

-- A definitions session's concept groups (family / synonym / antonym / ...).
CREATE TABLE content_definition_concepts (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    concept_no  INT  NOT NULL,
    head        TEXT NOT NULL DEFAULT '',
    meaning     TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL DEFAULT '',
    expressions TEXT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, concept_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no) REFERENCES content_entries ON DELETE CASCADE
);

-- An etymology session's semantic concepts and their origin members.
CREATE TABLE content_origin_concepts (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    concept_no  INT  NOT NULL,
    key         TEXT NOT NULL DEFAULT '',
    meaning     TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, concept_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no) REFERENCES content_entries ON DELETE CASCADE
);

CREATE TABLE content_origin_concept_members (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    concept_no  INT  NOT NULL,
    item_no     INT  NOT NULL,
    origin      TEXT NOT NULL DEFAULT '',
    language    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, concept_no, item_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, concept_no) REFERENCES content_origin_concepts ON DELETE CASCADE
);

CREATE TABLE content_relations (
    notebook_id VARCHAR(255) NOT NULL,
    family      VARCHAR(32)  NOT NULL,
    file_no     INT  NOT NULL,
    entry_no    INT  NOT NULL,
    relation_no INT  NOT NULL,
    type        TEXT NOT NULL DEFAULT '',
    between_keys TEXT[] NOT NULL DEFAULT '{}',
    from_key    TEXT NOT NULL DEFAULT '',
    to_key      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, relation_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no) REFERENCES content_entries ON DELETE CASCADE
);

CREATE TABLE content_corrections (
    notebook_id   VARCHAR(255) NOT NULL,
    family        VARCHAR(32)  NOT NULL,
    file_no       INT  NOT NULL,
    entry_no      INT  NOT NULL,
    scene_no      INT  NOT NULL,
    correction_no INT  NOT NULL,
    correction_id TEXT NOT NULL DEFAULT '',
    incorrect     TEXT NOT NULL DEFAULT '',
    correct       TEXT NOT NULL DEFAULT '',
    category      TEXT NOT NULL DEFAULT '',
    reason        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (notebook_id, family, file_no, entry_no, scene_no, correction_no),
    FOREIGN KEY (notebook_id, family, file_no, entry_no, scene_no) REFERENCES content_scenes ON DELETE CASCADE
);

-- Size of the bundle as pushed, for the notebook list (it used to be summed
-- from the stored files).
ALTER TABLE notebooks ADD COLUMN content_bytes INT NOT NULL DEFAULT 0;
