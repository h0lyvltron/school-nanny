-- Persistent per-family branching undo history. Content triggers only record
-- while history_context names an active user action; history replay,
-- migrations, startup backfills, and excluded settings writes stay out.
CREATE TABLE history_nodes (
    id                 INTEGER PRIMARY KEY,
    parent_id          INTEGER REFERENCES history_nodes(id) ON DELETE SET NULL,
    preferred_child_id INTEGER REFERENCES history_nodes(id) ON DELETE SET NULL,
    label              TEXT NOT NULL,
    route              TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    visited_at         TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'pending'
);
CREATE INDEX history_nodes_by_parent ON history_nodes(parent_id, id);

CREATE TABLE history_changes (
    id          INTEGER PRIMARY KEY,
    node_id     INTEGER NOT NULL REFERENCES history_nodes(id) ON DELETE CASCADE,
    table_name  TEXT NOT NULL,
    row_id      INTEGER NOT NULL,
    operation   TEXT NOT NULL CHECK(operation IN ('insert','update','delete')),
    before_json TEXT,
    after_json  TEXT
);
CREATE INDEX history_changes_by_node ON history_changes(node_id, id);

CREATE TABLE history_state (
    id              INTEGER PRIMARY KEY CHECK(id = 1),
    current_node_id   INTEGER REFERENCES history_nodes(id) ON DELETE SET NULL,
    preferred_root_id INTEGER REFERENCES history_nodes(id) ON DELETE SET NULL,
    revision        INTEGER NOT NULL DEFAULT 0
);
INSERT INTO history_state(id) VALUES(1);

CREATE TABLE history_context (
    id      INTEGER PRIMARY KEY CHECK(id = 1),
    node_id INTEGER REFERENCES history_nodes(id) ON DELETE SET NULL
);
INSERT INTO history_context(id) VALUES(1);


CREATE TRIGGER history_lessons_insert AFTER INSERT ON lessons
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'lessons', NEW.id, 'insert', json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'adult_id', NEW.adult_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'series_id', NEW.series_id, 'assignment_id', NEW.assignment_id, 'curriculum_item_id', NEW.curriculum_item_id, 'sequence', NEW.sequence, 'scheduled_on', NEW.scheduled_on, 'status', NEW.status, 'title', NEW.title, 'minutes', NEW.minutes, 'notes', NEW.notes, 'page_start', NEW.page_start, 'page_end', NEW.page_end, 'completed_at', NEW.completed_at, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_lessons_update AFTER UPDATE ON lessons
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'adult_id', OLD.adult_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'series_id', OLD.series_id, 'assignment_id', OLD.assignment_id, 'curriculum_item_id', OLD.curriculum_item_id, 'sequence', OLD.sequence, 'scheduled_on', OLD.scheduled_on, 'status', OLD.status, 'title', OLD.title, 'minutes', OLD.minutes, 'notes', OLD.notes, 'page_start', OLD.page_start, 'page_end', OLD.page_end, 'completed_at', OLD.completed_at, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'adult_id', NEW.adult_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'series_id', NEW.series_id, 'assignment_id', NEW.assignment_id, 'curriculum_item_id', NEW.curriculum_item_id, 'sequence', NEW.sequence, 'scheduled_on', NEW.scheduled_on, 'status', NEW.status, 'title', NEW.title, 'minutes', NEW.minutes, 'notes', NEW.notes, 'page_start', NEW.page_start, 'page_end', NEW.page_end, 'completed_at', NEW.completed_at, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'lessons', NEW.id, 'update', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'adult_id', OLD.adult_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'series_id', OLD.series_id, 'assignment_id', OLD.assignment_id, 'curriculum_item_id', OLD.curriculum_item_id, 'sequence', OLD.sequence, 'scheduled_on', OLD.scheduled_on, 'status', OLD.status, 'title', OLD.title, 'minutes', OLD.minutes, 'notes', OLD.notes, 'page_start', OLD.page_start, 'page_end', OLD.page_end, 'completed_at', OLD.completed_at, 'created_at', OLD.created_at), json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'adult_id', NEW.adult_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'series_id', NEW.series_id, 'assignment_id', NEW.assignment_id, 'curriculum_item_id', NEW.curriculum_item_id, 'sequence', NEW.sequence, 'scheduled_on', NEW.scheduled_on, 'status', NEW.status, 'title', NEW.title, 'minutes', NEW.minutes, 'notes', NEW.notes, 'page_start', NEW.page_start, 'page_end', NEW.page_end, 'completed_at', NEW.completed_at, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_lessons_delete AFTER DELETE ON lessons
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'lessons', OLD.id, 'delete', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'adult_id', OLD.adult_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'series_id', OLD.series_id, 'assignment_id', OLD.assignment_id, 'curriculum_item_id', OLD.curriculum_item_id, 'sequence', OLD.sequence, 'scheduled_on', OLD.scheduled_on, 'status', OLD.status, 'title', OLD.title, 'minutes', OLD.minutes, 'notes', OLD.notes, 'page_start', OLD.page_start, 'page_end', OLD.page_end, 'completed_at', OLD.completed_at, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_lesson_series_insert AFTER INSERT ON lesson_series
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'lesson_series', NEW.id, 'insert', json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'title', NEW.title, 'minutes', NEW.minutes, 'notes', NEW.notes, 'weekdays', NEW.weekdays, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on, 'occurrence_count', NEW.occurrence_count, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_lesson_series_update AFTER UPDATE ON lesson_series
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'title', OLD.title, 'minutes', OLD.minutes, 'notes', OLD.notes, 'weekdays', OLD.weekdays, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on, 'occurrence_count', OLD.occurrence_count, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'title', NEW.title, 'minutes', NEW.minutes, 'notes', NEW.notes, 'weekdays', NEW.weekdays, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on, 'occurrence_count', NEW.occurrence_count, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'lesson_series', NEW.id, 'update', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'title', OLD.title, 'minutes', OLD.minutes, 'notes', OLD.notes, 'weekdays', OLD.weekdays, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on, 'occurrence_count', OLD.occurrence_count, 'created_at', OLD.created_at), json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'title', NEW.title, 'minutes', NEW.minutes, 'notes', NEW.notes, 'weekdays', NEW.weekdays, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on, 'occurrence_count', NEW.occurrence_count, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_lesson_series_delete AFTER DELETE ON lesson_series
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'lesson_series', OLD.id, 'delete', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'title', OLD.title, 'minutes', OLD.minutes, 'notes', OLD.notes, 'weekdays', OLD.weekdays, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on, 'occurrence_count', OLD.occurrence_count, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_plan_assignments_insert AFTER INSERT ON plan_assignments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'plan_assignments', NEW.id, 'insert', json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'plan_id', NEW.plan_id, 'name', NEW.name, 'weekdays', NEW.weekdays, 'starts_on', NEW.starts_on, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_plan_assignments_update AFTER UPDATE ON plan_assignments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'plan_id', OLD.plan_id, 'name', OLD.name, 'weekdays', OLD.weekdays, 'starts_on', OLD.starts_on, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'plan_id', NEW.plan_id, 'name', NEW.name, 'weekdays', NEW.weekdays, 'starts_on', NEW.starts_on, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'plan_assignments', NEW.id, 'update', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'plan_id', OLD.plan_id, 'name', OLD.name, 'weekdays', OLD.weekdays, 'starts_on', OLD.starts_on, 'created_at', OLD.created_at), json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'school_year_id', NEW.school_year_id, 'plan_id', NEW.plan_id, 'name', NEW.name, 'weekdays', NEW.weekdays, 'starts_on', NEW.starts_on, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_plan_assignments_delete AFTER DELETE ON plan_assignments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'plan_assignments', OLD.id, 'delete', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'school_year_id', OLD.school_year_id, 'plan_id', OLD.plan_id, 'name', OLD.name, 'weekdays', OLD.weekdays, 'starts_on', OLD.starts_on, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_curriculum_plans_insert AFTER INSERT ON curriculum_plans
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'curriculum_plans', NEW.id, 'insert', json_object('id', NEW.id, 'name', NEW.name, 'subject_id', NEW.subject_id, 'kind', NEW.kind, 'source_kid_id', NEW.source_kid_id, 'source_year_id', NEW.source_year_id, 'notes', NEW.notes, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_curriculum_plans_update AFTER UPDATE ON curriculum_plans
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'name', OLD.name, 'subject_id', OLD.subject_id, 'kind', OLD.kind, 'source_kid_id', OLD.source_kid_id, 'source_year_id', OLD.source_year_id, 'notes', OLD.notes, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'name', NEW.name, 'subject_id', NEW.subject_id, 'kind', NEW.kind, 'source_kid_id', NEW.source_kid_id, 'source_year_id', NEW.source_year_id, 'notes', NEW.notes, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'curriculum_plans', NEW.id, 'update', json_object('id', OLD.id, 'name', OLD.name, 'subject_id', OLD.subject_id, 'kind', OLD.kind, 'source_kid_id', OLD.source_kid_id, 'source_year_id', OLD.source_year_id, 'notes', OLD.notes, 'created_at', OLD.created_at), json_object('id', NEW.id, 'name', NEW.name, 'subject_id', NEW.subject_id, 'kind', NEW.kind, 'source_kid_id', NEW.source_kid_id, 'source_year_id', NEW.source_year_id, 'notes', NEW.notes, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_curriculum_plans_delete AFTER DELETE ON curriculum_plans
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'curriculum_plans', OLD.id, 'delete', json_object('id', OLD.id, 'name', OLD.name, 'subject_id', OLD.subject_id, 'kind', OLD.kind, 'source_kid_id', OLD.source_kid_id, 'source_year_id', OLD.source_year_id, 'notes', OLD.notes, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_curriculum_items_insert AFTER INSERT ON curriculum_items
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'curriculum_items', NEW.id, 'insert', json_object('id', NEW.id, 'plan_id', NEW.plan_id, 'sort_order', NEW.sort_order, 'title', NEW.title, 'notes', NEW.notes, 'minutes', NEW.minutes, 'week_number', NEW.week_number, 'created_at', NEW.created_at, 'page_start', NEW.page_start, 'page_end', NEW.page_end));
END;

CREATE TRIGGER history_curriculum_items_update AFTER UPDATE ON curriculum_items
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'plan_id', OLD.plan_id, 'sort_order', OLD.sort_order, 'title', OLD.title, 'notes', OLD.notes, 'minutes', OLD.minutes, 'week_number', OLD.week_number, 'created_at', OLD.created_at, 'page_start', OLD.page_start, 'page_end', OLD.page_end) <> json_object('id', NEW.id, 'plan_id', NEW.plan_id, 'sort_order', NEW.sort_order, 'title', NEW.title, 'notes', NEW.notes, 'minutes', NEW.minutes, 'week_number', NEW.week_number, 'created_at', NEW.created_at, 'page_start', NEW.page_start, 'page_end', NEW.page_end)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'curriculum_items', NEW.id, 'update', json_object('id', OLD.id, 'plan_id', OLD.plan_id, 'sort_order', OLD.sort_order, 'title', OLD.title, 'notes', OLD.notes, 'minutes', OLD.minutes, 'week_number', OLD.week_number, 'created_at', OLD.created_at, 'page_start', OLD.page_start, 'page_end', OLD.page_end), json_object('id', NEW.id, 'plan_id', NEW.plan_id, 'sort_order', NEW.sort_order, 'title', NEW.title, 'notes', NEW.notes, 'minutes', NEW.minutes, 'week_number', NEW.week_number, 'created_at', NEW.created_at, 'page_start', NEW.page_start, 'page_end', NEW.page_end));
END;

CREATE TRIGGER history_curriculum_items_delete AFTER DELETE ON curriculum_items
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'curriculum_items', OLD.id, 'delete', json_object('id', OLD.id, 'plan_id', OLD.plan_id, 'sort_order', OLD.sort_order, 'title', OLD.title, 'notes', OLD.notes, 'minutes', OLD.minutes, 'week_number', OLD.week_number, 'created_at', OLD.created_at, 'page_start', OLD.page_start, 'page_end', OLD.page_end));
END;


CREATE TRIGGER history_attendance_insert AFTER INSERT ON attendance
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'attendance', NEW.id, 'insert', json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'attended_on', NEW.attended_on, 'status', NEW.status, 'notes', NEW.notes, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_attendance_update AFTER UPDATE ON attendance
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'attended_on', OLD.attended_on, 'status', OLD.status, 'notes', OLD.notes, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'attended_on', NEW.attended_on, 'status', NEW.status, 'notes', NEW.notes, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'attendance', NEW.id, 'update', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'attended_on', OLD.attended_on, 'status', OLD.status, 'notes', OLD.notes, 'created_at', OLD.created_at), json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'attended_on', NEW.attended_on, 'status', NEW.status, 'notes', NEW.notes, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_attendance_delete AFTER DELETE ON attendance
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'attendance', OLD.id, 'delete', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'attended_on', OLD.attended_on, 'status', OLD.status, 'notes', OLD.notes, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_assessments_insert AFTER INSERT ON assessments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'assessments', NEW.id, 'insert', json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'lesson_id', NEW.lesson_id, 'school_year_id', NEW.school_year_id, 'given_on', NEW.given_on, 'name', NEW.name, 'score', NEW.score, 'max_score', NEW.max_score, 'letter', NEW.letter, 'notes', NEW.notes, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_assessments_update AFTER UPDATE ON assessments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'lesson_id', OLD.lesson_id, 'school_year_id', OLD.school_year_id, 'given_on', OLD.given_on, 'name', OLD.name, 'score', OLD.score, 'max_score', OLD.max_score, 'letter', OLD.letter, 'notes', OLD.notes, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'lesson_id', NEW.lesson_id, 'school_year_id', NEW.school_year_id, 'given_on', NEW.given_on, 'name', NEW.name, 'score', NEW.score, 'max_score', NEW.max_score, 'letter', NEW.letter, 'notes', NEW.notes, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'assessments', NEW.id, 'update', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'lesson_id', OLD.lesson_id, 'school_year_id', OLD.school_year_id, 'given_on', OLD.given_on, 'name', OLD.name, 'score', OLD.score, 'max_score', OLD.max_score, 'letter', OLD.letter, 'notes', OLD.notes, 'created_at', OLD.created_at), json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'lesson_id', NEW.lesson_id, 'school_year_id', NEW.school_year_id, 'given_on', NEW.given_on, 'name', NEW.name, 'score', NEW.score, 'max_score', NEW.max_score, 'letter', NEW.letter, 'notes', NEW.notes, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_assessments_delete AFTER DELETE ON assessments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'assessments', OLD.id, 'delete', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'lesson_id', OLD.lesson_id, 'school_year_id', OLD.school_year_id, 'given_on', OLD.given_on, 'name', OLD.name, 'score', OLD.score, 'max_score', OLD.max_score, 'letter', OLD.letter, 'notes', OLD.notes, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_notes_insert AFTER INSERT ON notes
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'notes', NEW.id, 'insert', json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'adult_id', NEW.adult_id, 'subject_id', NEW.subject_id, 'noted_on', NEW.noted_on, 'body', NEW.body, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_notes_update AFTER UPDATE ON notes
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'adult_id', OLD.adult_id, 'subject_id', OLD.subject_id, 'noted_on', OLD.noted_on, 'body', OLD.body, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'adult_id', NEW.adult_id, 'subject_id', NEW.subject_id, 'noted_on', NEW.noted_on, 'body', NEW.body, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'notes', NEW.id, 'update', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'adult_id', OLD.adult_id, 'subject_id', OLD.subject_id, 'noted_on', OLD.noted_on, 'body', OLD.body, 'created_at', OLD.created_at), json_object('id', NEW.id, 'kid_id', NEW.kid_id, 'adult_id', NEW.adult_id, 'subject_id', NEW.subject_id, 'noted_on', NEW.noted_on, 'body', NEW.body, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_notes_delete AFTER DELETE ON notes
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'notes', OLD.id, 'delete', json_object('id', OLD.id, 'kid_id', OLD.kid_id, 'adult_id', OLD.adult_id, 'subject_id', OLD.subject_id, 'noted_on', OLD.noted_on, 'body', OLD.body, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_attachments_insert AFTER INSERT ON attachments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'attachments', NEW.id, 'insert', json_object('id', NEW.id, 'owner_type', NEW.owner_type, 'lesson_id', NEW.lesson_id, 'assessment_id', NEW.assessment_id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'curriculum_plan_id', NEW.curriculum_plan_id, 'original_name', NEW.original_name, 'stored_path', NEW.stored_path, 'size_bytes', NEW.size_bytes, 'content_type', NEW.content_type, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_attachments_update AFTER UPDATE ON attachments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'owner_type', OLD.owner_type, 'lesson_id', OLD.lesson_id, 'assessment_id', OLD.assessment_id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'curriculum_plan_id', OLD.curriculum_plan_id, 'original_name', OLD.original_name, 'stored_path', OLD.stored_path, 'size_bytes', OLD.size_bytes, 'content_type', OLD.content_type, 'created_at', OLD.created_at) <> json_object('id', NEW.id, 'owner_type', NEW.owner_type, 'lesson_id', NEW.lesson_id, 'assessment_id', NEW.assessment_id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'curriculum_plan_id', NEW.curriculum_plan_id, 'original_name', NEW.original_name, 'stored_path', NEW.stored_path, 'size_bytes', NEW.size_bytes, 'content_type', NEW.content_type, 'created_at', NEW.created_at)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'attachments', NEW.id, 'update', json_object('id', OLD.id, 'owner_type', OLD.owner_type, 'lesson_id', OLD.lesson_id, 'assessment_id', OLD.assessment_id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'curriculum_plan_id', OLD.curriculum_plan_id, 'original_name', OLD.original_name, 'stored_path', OLD.stored_path, 'size_bytes', OLD.size_bytes, 'content_type', OLD.content_type, 'created_at', OLD.created_at), json_object('id', NEW.id, 'owner_type', NEW.owner_type, 'lesson_id', NEW.lesson_id, 'assessment_id', NEW.assessment_id, 'kid_id', NEW.kid_id, 'subject_id', NEW.subject_id, 'curriculum_plan_id', NEW.curriculum_plan_id, 'original_name', NEW.original_name, 'stored_path', NEW.stored_path, 'size_bytes', NEW.size_bytes, 'content_type', NEW.content_type, 'created_at', NEW.created_at));
END;

CREATE TRIGGER history_attachments_delete AFTER DELETE ON attachments
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'attachments', OLD.id, 'delete', json_object('id', OLD.id, 'owner_type', OLD.owner_type, 'lesson_id', OLD.lesson_id, 'assessment_id', OLD.assessment_id, 'kid_id', OLD.kid_id, 'subject_id', OLD.subject_id, 'curriculum_plan_id', OLD.curriculum_plan_id, 'original_name', OLD.original_name, 'stored_path', OLD.stored_path, 'size_bytes', OLD.size_bytes, 'content_type', OLD.content_type, 'created_at', OLD.created_at));
END;


CREATE TRIGGER history_adult_events_insert AFTER INSERT ON adult_events
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_events', NEW.id, 'insert', json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on, 'title', NEW.title, 'body', NEW.body, 'created_at', NEW.created_at, 'label_id', NEW.label_id));
END;

CREATE TRIGGER history_adult_events_update AFTER UPDATE ON adult_events
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on, 'title', OLD.title, 'body', OLD.body, 'created_at', OLD.created_at, 'label_id', OLD.label_id) <> json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on, 'title', NEW.title, 'body', NEW.body, 'created_at', NEW.created_at, 'label_id', NEW.label_id)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_events', NEW.id, 'update', json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on, 'title', OLD.title, 'body', OLD.body, 'created_at', OLD.created_at, 'label_id', OLD.label_id), json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'starts_on', NEW.starts_on, 'ends_on', NEW.ends_on, 'title', NEW.title, 'body', NEW.body, 'created_at', NEW.created_at, 'label_id', NEW.label_id));
END;

CREATE TRIGGER history_adult_events_delete AFTER DELETE ON adult_events
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_events', OLD.id, 'delete', json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'starts_on', OLD.starts_on, 'ends_on', OLD.ends_on, 'title', OLD.title, 'body', OLD.body, 'created_at', OLD.created_at, 'label_id', OLD.label_id));
END;


CREATE TRIGGER history_adult_event_labels_insert AFTER INSERT ON adult_event_labels
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_event_labels', NEW.id, 'insert', json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'name', NEW.name, 'color', NEW.color, 'sort_order', NEW.sort_order, 'created_at', NEW.created_at, 'emoji', NEW.emoji));
END;

CREATE TRIGGER history_adult_event_labels_update AFTER UPDATE ON adult_event_labels
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'name', OLD.name, 'color', OLD.color, 'sort_order', OLD.sort_order, 'created_at', OLD.created_at, 'emoji', OLD.emoji) <> json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'name', NEW.name, 'color', NEW.color, 'sort_order', NEW.sort_order, 'created_at', NEW.created_at, 'emoji', NEW.emoji)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_event_labels', NEW.id, 'update', json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'name', OLD.name, 'color', OLD.color, 'sort_order', OLD.sort_order, 'created_at', OLD.created_at, 'emoji', OLD.emoji), json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'name', NEW.name, 'color', NEW.color, 'sort_order', NEW.sort_order, 'created_at', NEW.created_at, 'emoji', NEW.emoji));
END;

CREATE TRIGGER history_adult_event_labels_delete AFTER DELETE ON adult_event_labels
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_event_labels', OLD.id, 'delete', json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'name', OLD.name, 'color', OLD.color, 'sort_order', OLD.sort_order, 'created_at', OLD.created_at, 'emoji', OLD.emoji));
END;


CREATE TRIGGER history_adult_holiday_notes_insert AFTER INSERT ON adult_holiday_notes
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_holiday_notes', NEW.id, 'insert', json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'observed_on', NEW.observed_on, 'holiday_name', NEW.holiday_name, 'emoji', NEW.emoji, 'notes', NEW.notes, 'label_id', NEW.label_id));
END;

CREATE TRIGGER history_adult_holiday_notes_update AFTER UPDATE ON adult_holiday_notes
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
 AND json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'observed_on', OLD.observed_on, 'holiday_name', OLD.holiday_name, 'emoji', OLD.emoji, 'notes', OLD.notes, 'label_id', OLD.label_id) <> json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'observed_on', NEW.observed_on, 'holiday_name', NEW.holiday_name, 'emoji', NEW.emoji, 'notes', NEW.notes, 'label_id', NEW.label_id)
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json, after_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_holiday_notes', NEW.id, 'update', json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'observed_on', OLD.observed_on, 'holiday_name', OLD.holiday_name, 'emoji', OLD.emoji, 'notes', OLD.notes, 'label_id', OLD.label_id), json_object('id', NEW.id, 'adult_id', NEW.adult_id, 'observed_on', NEW.observed_on, 'holiday_name', NEW.holiday_name, 'emoji', NEW.emoji, 'notes', NEW.notes, 'label_id', NEW.label_id));
END;

CREATE TRIGGER history_adult_holiday_notes_delete AFTER DELETE ON adult_holiday_notes
WHEN (SELECT node_id FROM history_context WHERE id = 1) IS NOT NULL
BEGIN
  INSERT INTO history_changes(node_id, table_name, row_id, operation, before_json)
  VALUES((SELECT node_id FROM history_context WHERE id = 1), 'adult_holiday_notes', OLD.id, 'delete', json_object('id', OLD.id, 'adult_id', OLD.adult_id, 'observed_on', OLD.observed_on, 'holiday_name', OLD.holiday_name, 'emoji', OLD.emoji, 'notes', OLD.notes, 'label_id', OLD.label_id));
END;
