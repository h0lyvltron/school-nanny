-- A photo is a profile field rather than a general attachment: there is at most
-- one per child, and clearing it deletes the file. The value is a path relative
-- to the uploads folder, so the data folder stays movable.
ALTER TABLE kids ADD COLUMN avatar_path TEXT NOT NULL DEFAULT '';
