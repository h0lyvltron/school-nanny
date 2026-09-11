-- Labels are personal colour tags on a grown-up's calendar: "Medical", "Travel",
-- "School" and so on. They belong to one adult so Mom's labels never spill onto
-- someone else's profile, and deleting a label clears it from her events rather
-- than deleting the events themselves.
CREATE TABLE adult_event_labels (
    id         INTEGER PRIMARY KEY,
    adult_id   INTEGER NOT NULL REFERENCES adults(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX adult_event_labels_by_adult ON adult_event_labels (adult_id, sort_order, id);

-- An event may wear at most one label. The column is nullable so existing
-- events stay unlabeled until she picks one, and ON DELETE SET NULL keeps the
-- event when she retires a label she no longer uses.
ALTER TABLE adult_events ADD COLUMN label_id INTEGER
    REFERENCES adult_event_labels(id) ON DELETE SET NULL;
