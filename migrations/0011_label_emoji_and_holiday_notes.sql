-- Labels can carry a little emoji next to the color, so "Birthday" can show a
-- cake and "Travel" a plane without reading the name twice.
ALTER TABLE adult_event_labels ADD COLUMN emoji TEXT NOT NULL DEFAULT '';

-- Holidays themselves stay computed (Thanksgiving always lands on the right
-- Thursday). What she customizes - a note, a different emoji, a color label -
-- is stored per adult against that year's date and name, so scrolling to next
-- December still shows Christmas with her ornament unless she changes it.
CREATE TABLE adult_holiday_notes (
    id           INTEGER PRIMARY KEY,
    adult_id     INTEGER NOT NULL REFERENCES adults(id) ON DELETE CASCADE,
    observed_on  TEXT NOT NULL,
    holiday_name TEXT NOT NULL,
    emoji        TEXT NOT NULL DEFAULT '',
    notes        TEXT NOT NULL DEFAULT '',
    label_id     INTEGER REFERENCES adult_event_labels(id) ON DELETE SET NULL,
    UNIQUE (adult_id, observed_on, holiday_name)
);

CREATE INDEX adult_holiday_notes_by_range
    ON adult_holiday_notes (adult_id, observed_on);
