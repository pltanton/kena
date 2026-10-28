ALTER TABLE tasks ADD COLUMN kind text NOT NULL DEFAULT 'gap' CHECK (kind IN ('gap', 'choice'));
