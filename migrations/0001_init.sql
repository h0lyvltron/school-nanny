CREATE TABLE school_years (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    starts_on  TEXT NOT NULL,
    ends_on    TEXT NOT NULL,
    is_current INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE kids (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    grade      TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT '#5b8def',
    sort_order INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE subjects (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE lessons (
    id             INTEGER PRIMARY KEY,
    kid_id         INTEGER NOT NULL REFERENCES kids(id) ON DELETE CASCADE,
    subject_id     INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    school_year_id INTEGER REFERENCES school_years(id) ON DELETE SET NULL,
    scheduled_on   TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'planned',
    title          TEXT NOT NULL,
    minutes        INTEGER NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    completed_at   TEXT,
    created_at     TEXT NOT NULL
);

CREATE INDEX lessons_by_day ON lessons (scheduled_on);
CREATE INDEX lessons_by_kid ON lessons (kid_id, scheduled_on);
CREATE INDEX lessons_by_kid_subject ON lessons (kid_id, subject_id, scheduled_on);

CREATE TABLE assessments (
    id             INTEGER PRIMARY KEY,
    kid_id         INTEGER NOT NULL REFERENCES kids(id) ON DELETE CASCADE,
    subject_id     INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    lesson_id      INTEGER REFERENCES lessons(id) ON DELETE SET NULL,
    school_year_id INTEGER REFERENCES school_years(id) ON DELETE SET NULL,
    given_on       TEXT NOT NULL,
    name           TEXT NOT NULL,
    score          REAL,
    max_score      REAL,
    letter         TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
);

CREATE INDEX assessments_by_kid ON assessments (kid_id, given_on);

CREATE TABLE notes (
    id         INTEGER PRIMARY KEY,
    kid_id     INTEGER NOT NULL REFERENCES kids(id) ON DELETE CASCADE,
    subject_id INTEGER REFERENCES subjects(id) ON DELETE CASCADE,
    noted_on   TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX notes_by_kid ON notes (kid_id, noted_on);

-- owner_type is one of: lesson, assessment, resource.
-- A "resource" attachment is a reusable file filed under kid + subject.
CREATE TABLE attachments (
    id            INTEGER PRIMARY KEY,
    owner_type    TEXT NOT NULL,
    lesson_id     INTEGER REFERENCES lessons(id) ON DELETE CASCADE,
    assessment_id INTEGER REFERENCES assessments(id) ON DELETE CASCADE,
    kid_id        INTEGER REFERENCES kids(id) ON DELETE CASCADE,
    subject_id    INTEGER REFERENCES subjects(id) ON DELETE CASCADE,
    original_name TEXT NOT NULL,
    stored_path   TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    content_type  TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL
);

CREATE INDEX attachments_by_lesson ON attachments (lesson_id);
CREATE INDEX attachments_by_assessment ON attachments (assessment_id);
CREATE INDEX attachments_by_resource ON attachments (kid_id, subject_id);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO subjects (name, slug, sort_order) VALUES
    ('Math',           'math',           1),
    ('Language Arts',  'language-arts',  2),
    ('Science',        'science',        3),
    ('Social Studies', 'social-studies', 4),
    ('History',        'history',        5),
    ('Japanese',       'japanese',       6),
    ('Music & Art',    'music-art',      7),
    ('Other/Elective', 'other-elective', 8);
