ALTER TABLE oauth_codes ADD COLUMN token_id bigint REFERENCES tokens(id) ON DELETE CASCADE;
ALTER TABLE oauth_grants ADD COLUMN token_id bigint REFERENCES tokens(id) ON DELETE CASCADE;
