CREATE TABLE stars (
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  set_id bigint NOT NULL REFERENCES sets(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, set_id)
);
CREATE INDEX stars_set ON stars(set_id);
