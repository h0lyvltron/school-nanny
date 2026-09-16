-- First-class PDF page ranges for curriculum sequences and scheduled lessons.
ALTER TABLE curriculum_items ADD COLUMN page_start INTEGER;
ALTER TABLE curriculum_items ADD COLUMN page_end INTEGER;

ALTER TABLE lessons ADD COLUMN page_start INTEGER;
ALTER TABLE lessons ADD COLUMN page_end INTEGER;
