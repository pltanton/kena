package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type OAuthClient struct {
	ClientID     string
	RedirectURIs []string
	ClientName   string
}

type AuthCode struct {
	ClientID      string
	UserID        int64
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	ExpiresAt     time.Time
	TokenID       int64
}

type Grant struct {
	ID              int64
	UserID          int64
	ClientID        string
	Resource        string
	AccessExpiresAt time.Time
	TokenID         int64
}

func (s *Store) SaveClient(ctx context.Context, c OAuthClient) error {
	_, err := exec(ctx, s.pool, "saving client", `
		INSERT INTO oauth_clients (client_id, redirect_uris, client_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (client_id) DO UPDATE SET redirect_uris = EXCLUDED.redirect_uris, client_name = EXCLUDED.client_name
	`, c.ClientID, c.RedirectURIs, c.ClientName)
	return err
}

func (s *Store) GetClient(ctx context.Context, clientID string) (OAuthClient, error) {
	var c OAuthClient
	if err := s.pool.QueryRow(ctx, `
		SELECT client_id, redirect_uris, client_name FROM oauth_clients WHERE client_id = $1
	`, clientID).Scan(&c.ClientID, &c.RedirectURIs, &c.ClientName); err != nil {
		return OAuthClient{}, noRows(err, "getting client")
	}
	return c, nil
}

// nullableID turns a zero token id into SQL NULL; ids start at 1, so 0 means "no token".
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func (s *Store) SaveCode(ctx context.Context, code string, c AuthCode) error {
	_, err := exec(ctx, s.pool, "saving code", `
		INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at, token_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, HashSecret(code), c.ClientID, c.UserID, c.RedirectURI, c.CodeChallenge, c.Resource, c.Scope, c.ExpiresAt, nullableID(c.TokenID))
	return err
}

func (s *Store) ConsumeCode(ctx context.Context, code string) (AuthCode, error) {
	var c AuthCode
	if err := s.pool.QueryRow(ctx, `
		UPDATE oauth_codes SET used_at = now()
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at, COALESCE(token_id, 0)
	`, HashSecret(code)).Scan(&c.ClientID, &c.UserID, &c.RedirectURI, &c.CodeChallenge, &c.Resource, &c.Scope, &c.ExpiresAt, &c.TokenID); err != nil {
		return AuthCode{}, noRows(err, "consuming code")
	}
	return c, nil
}

func insertGrant(ctx context.Context, q querier, userID int64, clientID, resource string, tokenID int64, accessTTL, refreshTTL time.Duration) (string, string, Grant, error) {
	access, err := NewSecret("kena_at_")
	if err != nil {
		return "", "", Grant{}, err
	}
	refresh, err := NewSecret("kena_rt_")
	if err != nil {
		return "", "", Grant{}, err
	}
	now := time.Now()
	var g Grant
	if err := q.QueryRow(ctx, `
		INSERT INTO oauth_grants (user_id, client_id, access_hash, access_expires_at, refresh_hash, refresh_expires_at, resource, token_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, client_id, resource, access_expires_at, COALESCE(token_id, 0)
	`, userID, clientID, HashSecret(access), now.Add(accessTTL), HashSecret(refresh), now.Add(refreshTTL), resource, nullableID(tokenID),
	).Scan(&g.ID, &g.UserID, &g.ClientID, &g.Resource, &g.AccessExpiresAt, &g.TokenID); err != nil {
		return "", "", Grant{}, fmt.Errorf("creating grant: %w", err)
	}
	return access, refresh, g, nil
}

func (s *Store) CreateGrant(ctx context.Context, userID int64, clientID, resource string, tokenID int64, accessTTL, refreshTTL time.Duration) (string, string, Grant, error) {
	return insertGrant(ctx, s.pool, userID, clientID, resource, tokenID, accessTTL, refreshTTL)
}

func (s *Store) RotateRefresh(ctx context.Context, refresh, clientID string, accessTTL, refreshTTL time.Duration) (access, newRefresh string, g Grant, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var id, userID, tokenID int64
		var resource string
		if err := tx.QueryRow(ctx, `
			SELECT id, user_id, resource, COALESCE(token_id, 0)
			FROM oauth_grants
			WHERE refresh_hash = $1 AND client_id = $2 AND revoked_at IS NULL AND refresh_expires_at > now()
			FOR UPDATE
		`, HashSecret(refresh), clientID).Scan(&id, &userID, &resource, &tokenID); err != nil {
			return noRows(err, "looking up grant")
		}
		if _, err := exec(ctx, tx, "revoking grant", `UPDATE oauth_grants SET revoked_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		var err error
		access, newRefresh, g, err = insertGrant(ctx, tx, userID, clientID, resource, tokenID, accessTTL, refreshTTL)
		return err
	})
	if err != nil {
		return "", "", Grant{}, err
	}
	return access, newRefresh, g, nil
}

func (s *Store) GrantByAccess(ctx context.Context, access string) (Grant, User, error) {
	var g Grant
	var u User
	if err := s.pool.QueryRow(ctx, `
		SELECT g.id, g.user_id, g.client_id, g.resource, g.access_expires_at, `+userColumns+`
		FROM oauth_grants g JOIN users u ON u.id = g.user_id
		WHERE g.access_hash = $1 AND g.revoked_at IS NULL AND g.access_expires_at > now()
	`, HashSecret(access)).Scan(append([]any{&g.ID, &g.UserID, &g.ClientID, &g.Resource, &g.AccessExpiresAt}, u.dest()...)...); err != nil {
		return Grant{}, User{}, noRows(err, "looking up grant")
	}
	return g, u, nil
}
