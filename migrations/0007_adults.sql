-- An adult is a person in the family who has a schedule and notes of her own,
-- but no grades, attendance, or curriculum. She is a separate table rather than
-- a flagged child so nothing kid-shaped ever has to be faked for her.
CREATE TABLE adults (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'Mom',
    color       TEXT NOT NULL DEFAULT '#8d78e0',
    avatar_path TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    archived    INTEGER NOT NULL DEFAULT 0
);

-- Cards are the pinboard on her profile: things worth keeping in view that are
-- not tied to a day, which is what separates them from notes and schedule.
CREATE TABLE adult_cards (
    id         INTEGER PRIMARY KEY,
    adult_id   INTEGER NOT NULL REFERENCES adults(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    pinned     INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX adult_cards_by_adult ON adult_cards (adult_id, pinned DESC, sort_order);

-- Lessons and notes now belong to exactly one of a child or an adult, which
-- means kid_id has to become nullable. SQLite cannot relax a column, so both
-- tables are rebuilt.
--
-- The replacement is built beside the original and renamed into place, rather
-- than the other way round: renaming a table rewrites every REFERENCES clause
-- that names it, so moving the original aside would drag the foreign keys in
-- assessments and attachments along with it. Dropping the original leaves
-- those clauses naming "lessons", which the rename then restores.
--
-- Migrations run with foreign keys switched off, so the drop takes nothing
-- with it, and foreign_key_check has the last word before any of this commits.
CREATE TABLE lessons_new (
    id             INTEGER PRIMARY KEY,
    kid_id         INTEGER REFERENCES kids(id) ON DELETE CASCADE,
    adult_id       INTEGER REFERENCES adults(id) ON DELETE CASCADE,
    subject_id     INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    school_year_id INTEGER REFERENCES school_years(id) ON DELETE SET NULL,
    series_id      INTEGER REFERENCES lesson_series(id) ON DELETE SET NULL,
    assignment_id  INTEGER REFERENCES plan_assignments(id) ON DELETE SET NULL,
    sequence       INTEGER NOT NULL DEFAULT 0,
    scheduled_on   TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'planned',
    title          TEXT NOT NULL,
    minutes        INTEGER NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    completed_at   TEXT,
    created_at     TEXT NOT NULL,
    CHECK ((kid_id IS NULL) <> (adult_id IS NULL))
);

INSERT INTO lessons_new (id, kid_id, adult_id, subject_id, school_year_id, series_id,
                         assignment_id, sequence, scheduled_on, status, title,
                         minutes, notes, completed_at, created_at)
SELECT id, kid_id, NULL, subject_id, school_year_id, series_id,
       assignment_id, sequence, scheduled_on, status, title,
       minutes, notes, completed_at, created_at
FROM lessons;

DROP TABLE lessons;
ALTER TABLE lessons_new RENAME TO lessons;

CREATE INDEX lessons_by_day ON lessons (scheduled_on);
CREATE INDEX lessons_by_kid ON lessons (kid_id, scheduled_on);
CREATE INDEX lessons_by_kid_subject ON lessons (kid_id, subject_id, scheduled_on);
CREATE INDEX lessons_by_series ON lessons (series_id);
CREATE INDEX lessons_by_assignment ON lessons (assignment_id, sequence);
CREATE INDEX lessons_by_adult ON lessons (adult_id, scheduled_on);

CREATE TABLE notes_new (
    id         INTEGER PRIMARY KEY,
    kid_id     INTEGER REFERENCES kids(id) ON DELETE CASCADE,
    adult_id   INTEGER REFERENCES adults(id) ON DELETE CASCADE,
    subject_id INTEGER REFERENCES subjects(id) ON DELETE CASCADE,
    noted_on   TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    CHECK ((kid_id IS NULL) <> (adult_id IS NULL))
);

INSERT INTO notes_new (id, kid_id, adult_id, subject_id, noted_on, body, created_at)
SELECT id, kid_id, NULL, subject_id, noted_on, body, created_at FROM notes;

DROP TABLE notes;
ALTER TABLE notes_new RENAME TO notes;

CREATE INDEX notes_by_kid ON notes (kid_id, noted_on);
CREATE INDEX notes_by_adult ON notes (adult_id, noted_on);
