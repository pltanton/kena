UPDATE sets SET rules = (
	SELECT COALESCE(jsonb_agg(
		CASE WHEN r ? 'body' THEN r
		ELSE jsonb_build_object('key', r->'key', 'title', '', 'body', COALESCE(r->'text', '""'::jsonb), 'general', false)
		END ORDER BY ord), '[]'::jsonb)
	FROM jsonb_array_elements(sets.rules) WITH ORDINALITY AS e(r, ord)
)
WHERE rules <> '[]'::jsonb;
