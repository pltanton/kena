ALTER TABLE sets ADD COLUMN visibility text NOT NULL DEFAULT 'private'
  CHECK (visibility IN ('private', 'friends', 'public'));
UPDATE sets SET visibility = CASE WHEN shared THEN 'public' ELSE 'private' END;
ALTER TABLE sets DROP COLUMN shared;
