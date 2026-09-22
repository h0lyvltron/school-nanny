-- CalDAV needs a stable VEVENT identity and a recurrence rule that survives a
-- round-trip to the phone. Existing rows get a generated uid so every event
-- can be addressed as /dav/calendars/{adult}/{uid}.ics.
--
-- all_day stays 1 for the dates we already store. Timed events fill start_at
-- and end_at (RFC3339) while starts_on/ends_on keep the inclusive calendar
-- dates the month grid uses for range queries.
--
-- Tombstones and a per-adult sync token let REPORT sync-collection tell the
-- phone which hrefs disappeared since its last poll.

ALTER TABLE adult_events ADD COLUMN uid TEXT NOT NULL DEFAULT '';
ALTER TABLE adult_events ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE adult_events ADD COLUMN modified_at TEXT NOT NULL DEFAULT '';
ALTER TABLE adult_events ADD COLUMN all_day INTEGER NOT NULL DEFAULT 1;
ALTER TABLE adult_events ADD COLUMN start_at TEXT NOT NULL DEFAULT '';
ALTER TABLE adult_events ADD COLUMN end_at TEXT NOT NULL DEFAULT '';
ALTER TABLE adult_events ADD COLUMN location TEXT NOT NULL DEFAULT '';
ALTER TABLE adult_events ADD COLUMN rrule TEXT NOT NULL DEFAULT '';
ALTER TABLE adult_events ADD COLUMN exdates TEXT NOT NULL DEFAULT '';

UPDATE adult_events
SET uid = printf('adult-event-%d@school-nanny', id),
    modified_at = CASE WHEN modified_at = '' THEN created_at ELSE modified_at END
WHERE uid = '';

CREATE UNIQUE INDEX adult_events_by_uid ON adult_events (adult_id, uid);

CREATE TABLE adult_event_tombstones (
    adult_id   INTEGER NOT NULL REFERENCES adults(id) ON DELETE CASCADE,
    uid        TEXT NOT NULL,
    deleted_at TEXT NOT NULL,
    PRIMARY KEY (adult_id, uid)
);

CREATE TABLE adult_calendar_meta (
    adult_id   INTEGER PRIMARY KEY REFERENCES adults(id) ON DELETE CASCADE,
    sync_token INTEGER NOT NULL DEFAULT 1
);

INSERT INTO adult_calendar_meta (adult_id, sync_token)
SELECT id, 1 FROM adults;

-- History undo must restore the new columns. Recreate the adult_events
-- triggers so before/after JSON carries them.
DROP TRIGGER IF EXISTS history_adult_events_insert;
DROP TRIGGER IF EXISTS history_adult_events_update;
DROP TRIGGER IF EXISTS history_adult_events_delete;

CREATE TRIGGER history_adult_events_insert AFTER INSERT ON adult_events
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_events', NEW.id, 'insert',
    json_object(
      'id', NEW.id, 'adult_id', NEW.adult_id, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on,
      'title', NEW.title, 'body', NEW.body, 'created_at', NEW.created_at, 'label_id', NEW.label_id,
      'uid', NEW.uid, 'sequence', NEW.sequence, 'modified_at', NEW.modified_at,
      'all_day', NEW.all_day, 'start_at', NEW.start_at, 'end_at', NEW.end_at,
      'location', NEW.location, 'rrule', NEW.rrule, 'exdates', NEW.exdates
    ));
END;

CREATE TRIGGER history_adult_events_update AFTER UPDATE ON adult_events
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object(
      'id', OLD.id, 'adult_id', OLD.adult_id, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on,
      'title', OLD.title, 'body', OLD.body, 'created_at', OLD.created_at, 'label_id', OLD.label_id,
      'uid', OLD.uid, 'sequence', OLD.sequence, 'modified_at', OLD.modified_at,
      'all_day', OLD.all_day, 'start_at', OLD.start_at, 'end_at', OLD.end_at,
      'location', OLD.location, 'rrule', OLD.rrule, 'exdates', OLD.exdates
    ) <> json_object(
      'id', NEW.id, 'adult_id', NEW.adult_id, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on,
      'title', NEW.title, 'body', NEW.body, 'created_at', NEW.created_at, 'label_id', NEW.label_id,
      'uid', NEW.uid, 'sequence', NEW.sequence, 'modified_at', NEW.modified_at,
      'all_day', NEW.all_day, 'start_at', NEW.start_at, 'end_at', NEW.end_at,
      'location', NEW.location, 'rrule', NEW.rrule, 'exdates', NEW.exdates
    )
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_events', NEW.id, 'update',
    json_object(
      'id', OLD.id, 'adult_id', OLD.adult_id, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on,
      'title', OLD.title, 'body', OLD.body, 'created_at', OLD.created_at, 'label_id', OLD.label_id,
      'uid', OLD.uid, 'sequence', OLD.sequence, 'modified_at', OLD.modified_at,
      'all_day', OLD.all_day, 'start_at', OLD.start_at, 'end_at', OLD.end_at,
      'location', OLD.location, 'rrule', OLD.rrule, 'exdates', OLD.exdates
    ),
    json_object(
      'id', NEW.id, 'adult_id', NEW.adult_id, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on,
      'title', NEW.title, 'body', NEW.body, 'created_at', NEW.created_at, 'label_id', NEW.label_id,
      'uid', NEW.uid, 'sequence', NEW.sequence, 'modified_at', NEW.modified_at,
      'all_day', NEW.all_day, 'start_at', NEW.start_at, 'end_at', NEW.end_at,
      'location', NEW.location, 'rrule', NEW.rrule, 'exdates', NEW.exdates
    ));
END;

CREATE TRIGGER history_adult_events_delete AFTER DELETE ON adult_events
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_events', OLD.id, 'delete',
    json_object(
      'id', OLD.id, 'adult_id', OLD.adult_id, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on,
      'title', OLD.title, 'body', OLD.body, 'created_at', OLD.created_at, 'label_id', OLD.label_id,
      'uid', OLD.uid, 'sequence', OLD.sequence, 'modified_at', OLD.modified_at,
      'all_day', OLD.all_day, 'start_at', OLD.start_at, 'end_at', OLD.end_at,
      'location', OLD.location, 'rrule', OLD.rrule, 'exdates', OLD.exdates
    ));
END;
