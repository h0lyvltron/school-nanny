-- A subject carries a colour of its own so a day's work can be told apart at a
-- glance instead of reading as one block of black text.
--
-- The column starts empty rather than with a colour every subject shares:
-- BackfillSubjectColors hands the existing subjects distinct colours from the
-- same palette the settings form offers, which keeps that list in one place.
ALTER TABLE subjects ADD COLUMN color TEXT NOT NULL DEFAULT '';
