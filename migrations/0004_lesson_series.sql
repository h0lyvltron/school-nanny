CREATE TABLE lesson_series (
    id               INTEGER PRIMARY KEY,
    kid_id           INTEGER NOT NULL REFERENCES kids(id) ON DELETE CASCADE,
    subject_id       INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    school_year_id   INTEGER REFERENCES school_years(id) ON DELETE SET NULL,
    title            TEXT NOT NULL,
    minutes          INTEGER NOT NULL DEFAULT 0,
    notes            TEXT NOT NULL DEFAULT '',
    weekdays         TEXT NOT NULL,
    starts_on        TEXT NOT NULL,
    ends_on          TEXT NOT NULL DEFAULT '',
    occurrence_count INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL
);

ALTER TABLE lessons ADD COLUMN series_id INTEGER REFERENCES lesson_series(id) ON DELETE SET NULL;

CREATE INDEX lessons_by_series ON lessons (series_id);
