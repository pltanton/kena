ALTER TABLE users ADD COLUMN password_hash text;
ALTER TABLE users ADD COLUMN progress_id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid();
CREATE TABLE sessions (
  hash bytea PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
