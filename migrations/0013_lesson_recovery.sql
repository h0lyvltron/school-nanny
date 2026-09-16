-- Keep the curriculum source on lesson snapshots so one item can be scheduled
-- without applying the whole plan.
ALTER TABLE lessons ADD COLUMN curriculum_item_id INTEGER
    REFERENCES curriculum_items(id) ON DELETE SET NULL;

CREATE INDEX lessons_by_curriculum_item
    ON lessons (curriculum_item_id, kid_id, scheduled_on);

UPDATE lessons
SET curriculum_item_id = (
    SELECT ci.id
    FROM plan_assignments pa
    JOIN curriculum_items ci
      ON ci.plan_id = pa.plan_id
     AND ci.sort_order = lessons.sequence
    WHERE pa.id = lessons.assignment_id
    ORDER BY ci.id
    LIMIT 1
)
WHERE assignment_id IS NOT NULL
  AND sequence > 0;

-- Individual lesson deletion is undoable for seven days. The JSON snapshots
-- include relationship metadata that would otherwise be lost to FK actions.
CREATE TABLE deleted_lessons (
    token                 TEXT PRIMARY KEY,
    original_lesson_id    INTEGER NOT NULL,
    title                 TEXT NOT NULL,
    scheduled_on          TEXT NOT NULL,
    person_name           TEXT NOT NULL DEFAULT '',
    lesson_json           TEXT NOT NULL,
    attachments_json      TEXT NOT NULL DEFAULT '[]',
    assessment_ids_json   TEXT NOT NULL DEFAULT '[]',
    deleted_at            TEXT NOT NULL,
    expires_at            TEXT NOT NULL
);

CREATE INDEX deleted_lessons_by_expiry ON deleted_lessons (expires_at);
