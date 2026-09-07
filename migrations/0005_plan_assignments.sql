CREATE TABLE plan_assignments (
    id             INTEGER PRIMARY KEY,
    kid_id         INTEGER NOT NULL REFERENCES kids(id) ON DELETE CASCADE,
    subject_id     INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    school_year_id INTEGER REFERENCES school_years(id) ON DELETE SET NULL,
    plan_id        INTEGER REFERENCES curriculum_plans(id) ON DELETE SET NULL,
    name           TEXT NOT NULL,
    weekdays       TEXT NOT NULL,
    starts_on      TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE INDEX plan_assignments_by_kid ON plan_assignments (kid_id, subject_id);
CREATE INDEX plan_assignments_by_plan ON plan_assignments (plan_id);

ALTER TABLE lessons ADD COLUMN assignment_id INTEGER REFERENCES plan_assignments(id) ON DELETE SET NULL;
ALTER TABLE lessons ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;

CREATE INDEX lessons_by_assignment ON lessons (assignment_id, sequence);
