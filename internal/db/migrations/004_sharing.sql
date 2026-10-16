ALTER TABLE sets ADD COLUMN shared boolean NOT NULL DEFAULT false;
UPDATE sets SET shared = true;
