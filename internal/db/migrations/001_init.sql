CREATE TABLE users (
  id bigserial PRIMARY KEY,
  username text NOT NULL UNIQUE,
  is_admin boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE tokens (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label text NOT NULL DEFAULT '',
  hash bytea NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  revoked_at timestamptz
);
CREATE TABLE oauth_clients (
  client_id text PRIMARY KEY,
  redirect_uris text[] NOT NULL,
  client_name text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE oauth_codes (
  code_hash bytea PRIMARY KEY,
  client_id text NOT NULL,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  redirect_uri text NOT NULL,
  code_challenge text NOT NULL,
  resource text NOT NULL DEFAULT '',
  scope text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  used_at timestamptz
);
CREATE TABLE oauth_grants (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  client_id text NOT NULL,
  access_hash bytea NOT NULL UNIQUE,
  access_expires_at timestamptz NOT NULL,
  refresh_hash bytea NOT NULL UNIQUE,
  refresh_expires_at timestamptz NOT NULL,
  resource text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz
);
CREATE TABLE sets (
  id bigserial PRIMARY KEY,
  owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug text NOT NULL UNIQUE,
  title text NOT NULL,
  topic text NOT NULL DEFAULT '',
  level text NOT NULL DEFAULT '',
  instructions text NOT NULL DEFAULT '',
  source_ref text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE tasks (
  id bigserial PRIMARY KEY,
  set_id bigint NOT NULL REFERENCES sets(id) ON DELETE CASCADE,
  position int NOT NULL,
  text text NOT NULL,
  blanks jsonb NOT NULL,
  explanation text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_set_position ON tasks(set_id, position);
CREATE TABLE attempts (
  id bigserial PRIMARY KEY,
  device_id uuid NOT NULL,
  task_id bigint NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  answers jsonb NOT NULL,
  correct boolean NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attempts_device_task ON attempts(device_id, task_id, created_at DESC);
