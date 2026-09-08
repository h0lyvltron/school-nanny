-- An event is something on a grown-up's own calendar: an appointment, a trip,
-- a week away. It is deliberately not a lesson, because none of the schedule
-- machinery -- subjects, minutes, done/skipped -- means anything for a dentist
-- visit, and none of it should have to be faked to write one down.
--
-- Both dates are stored, and both are inclusive, so a single day is simply a
-- row where they match. Keeping the end date rather than a length means the
-- span a parent dragged out on the calendar is the span that comes back, with
-- no arithmetic in between to get wrong.
CREATE TABLE adult_events (
    id         INTEGER PRIMARY KEY,
    adult_id   INTEGER NOT NULL REFERENCES adults(id) ON DELETE CASCADE,
    starts_on  TEXT NOT NULL,
    ends_on    TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    CHECK (ends_on >= starts_on)
);

-- A month view asks for everything overlapping a range, which reads the rows
-- for one person in start order and stops early once past the end of the view.
CREATE INDEX adult_events_by_range ON adult_events (adult_id, starts_on, ends_on);
