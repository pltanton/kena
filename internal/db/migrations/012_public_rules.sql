CREATE TABLE rule_categories (
  id bigserial PRIMARY KEY,
  name text NOT NULL UNIQUE,
  created_by bigint REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public_rules (
  id bigserial PRIMARY KEY,
  category_id bigint NOT NULL REFERENCES rule_categories(id) ON DELETE RESTRICT,
  name text NOT NULL,
  title text NOT NULL DEFAULT '',
  body text NOT NULL,
  owner_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (category_id, name)
);
CREATE INDEX public_rules_owner ON public_rules(owner_id);

ALTER TABLE tasks ADD COLUMN public_rule_id bigint REFERENCES public_rules(id) ON DELETE RESTRICT;
ALTER TABLE tasks ADD CONSTRAINT tasks_one_rule CHECK (rule IS NULL OR public_rule_id IS NULL);
CREATE INDEX tasks_public_rule ON tasks(public_rule_id) WHERE public_rule_id IS NOT NULL;

CREATE TABLE set_public_rules (
  set_id bigint NOT NULL REFERENCES sets(id) ON DELETE CASCADE,
  rule_id bigint NOT NULL REFERENCES public_rules(id) ON DELETE RESTRICT,
  position int NOT NULL,
  PRIMARY KEY (set_id, rule_id)
);
CREATE INDEX set_public_rules_rule ON set_public_rules(rule_id);

CREATE FUNCTION public_rule_key(rule_id bigint) RETURNS text LANGUAGE sql STABLE AS $$
  SELECT c.name || '/' || p.name FROM public_rules p JOIN rule_categories c ON c.id = p.category_id WHERE p.id = rule_id
$$;

CREATE FUNCTION set_public_rules_json(sid bigint) RETURNS jsonb LANGUAGE sql STABLE AS $$
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
      'key', c.name || '/' || p.name, 'title', p.title, 'body', p.body, 'general', a.set_id IS NOT NULL)
      ORDER BY a.position NULLS LAST, c.name, p.name), '[]'::jsonb)
  FROM public_rules p
  JOIN rule_categories c ON c.id = p.category_id
  LEFT JOIN set_public_rules a ON a.rule_id = p.id AND a.set_id = sid
  WHERE a.set_id IS NOT NULL OR EXISTS (SELECT 1 FROM tasks t WHERE t.set_id = sid AND t.public_rule_id = p.id)
$$;

CREATE FUNCTION set_public_stamp(sid bigint) RETURNS timestamptz LANGUAGE sql STABLE AS $$
  SELECT COALESCE(max(p.updated_at), 'epoch'::timestamptz)
  FROM public_rules p
  WHERE EXISTS (SELECT 1 FROM set_public_rules a WHERE a.set_id = sid AND a.rule_id = p.id)
    OR EXISTS (SELECT 1 FROM tasks t WHERE t.set_id = sid AND t.public_rule_id = p.id)
$$;
