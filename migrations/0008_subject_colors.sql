-- A subject carries a color of its own so a day's work can be told apart at a
-- glance instead of reading as one block of black text.
--
-- The column starts empty rather than with a color every subject shares:
-- BackfillSubjectColors hands the existing subjects distinct colors from the
-- same palette the settings form offers, which keeps that list in one place.
ALTER TABLE subjects ADD COLUMN color TEXT NOT NULL DEFAULT '';
