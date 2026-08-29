CREATE TABLE curriculum_plans (
    id             INTEGER PRIMARY KEY,
    name           TEXT NOT NULL,
    subject_id     INTEGER NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL DEFAULT 'authored',
    source_kid_id  INTEGER REFERENCES kids(id) ON DELETE SET NULL,
    source_year_id INTEGER REFERENCES school_years(id) ON DELETE SET NULL,
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
);

CREATE INDEX curriculum_plans_by_subject ON curriculum_plans (subject_id);

CREATE TABLE curriculum_items (
    id          INTEGER PRIMARY KEY,
    plan_id     INTEGER NOT NULL REFERENCES curriculum_plans(id) ON DELETE CASCADE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    title       TEXT NOT NULL,
    notes       TEXT NOT NULL DEFAULT '',
    minutes     INTEGER NOT NULL DEFAULT 0,
    week_number INTEGER,
    created_at  TEXT NOT NULL
);

CREATE INDEX curriculum_items_by_plan ON curriculum_items (plan_id, sort_order);

ALTER TABLE attachments ADD COLUMN curriculum_plan_id INTEGER REFERENCES curriculum_plans(id) ON DELETE CASCADE;

CREATE INDEX attachments_by_curriculum ON attachments (curriculum_plan_id);
