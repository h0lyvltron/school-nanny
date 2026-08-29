CREATE TABLE attendance (
    id          INTEGER PRIMARY KEY,
    kid_id      INTEGER NOT NULL REFERENCES kids(id) ON DELETE CASCADE,
    attended_on TEXT NOT NULL,
    status      TEXT NOT NULL,
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    UNIQUE (kid_id, attended_on)
);

CREATE INDEX attendance_by_day ON attendance (attended_on);
CREATE INDEX attendance_by_kid ON attendance (kid_id, attended_on);
