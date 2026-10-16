package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
}

type Grant struct {
	ID              int64
	UserID          int64
	ClientID        string
	Resource        string
	AccessExpiresAt time.Time
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Store) SaveClient(ctx context.Context, c OAuthClient) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_clients (client_id, redirect_uris, client_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (client_id) DO UPDATE SET redirect_uris = EXCLUDED.redirect_uris, client_name = EXCLUDED.client_name
	`, c.ClientID, c.RedirectURIs, c.ClientName)
	if err != nil {
		return fmt.Errorf("saving client: %w", err)
	}
	return nil
}

func (s *Store) GetClient(ctx context.Context, clientID string) (OAuthClient, error) {
	var c OAuthClient
	err := s.pool.QueryRow(ctx, `
		SELECT client_id, redirect_uris, client_name FROM oauth_clients WHERE client_id = $1
	`, clientID).Scan(&c.ClientID, &c.RedirectURIs, &c.ClientName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OAuthClient{}, ErrNotFound
		}
		return OAuthClient{}, fmt.Errorf("getting client: %w", err)
	}
	return c, nil
}

func (s *Store) SaveCode(ctx context.Context, code string, c AuthCode) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, HashSecret(code), c.ClientID, c.UserID, c.RedirectURI, c.CodeChallenge, c.Resource, c.Scope, c.ExpiresAt)
	if err != nil {
		return fmt.Errorf("saving code: %w", err)
	}
	return nil
}

func (s *Store) ConsumeCode(ctx context.Context, code string) (AuthCode, error) {
	var c AuthCode
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_codes SET used_at = now()
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at
	`, HashSecret(code)).Scan(&c.ClientID, &c.UserID, &c.RedirectURI, &c.CodeChallenge, &c.Resource, &c.Scope, &c.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthCode{}, ErrNotFound
		}
		return AuthCode{}, fmt.Errorf("consuming code: %w", err)
	}
	return c, nil
}

func insertGrant(ctx context.Context, q querier, userID int64, clientID, resource string, accessTTL, refreshTTL time.Duration) (string, string, Grant, error) {
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
	err = q.QueryRow(ctx, `
		INSERT INTO oauth_grants (user_id, client_id, access_hash, access_expires_at, refresh_hash, refresh_expires_at, resource)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, client_id, resource, access_expires_at
	`, userID, clientID, HashSecret(access), now.Add(accessTTL), HashSecret(refresh), now.Add(refreshTTL), resource,
	).Scan(&g.ID, &g.UserID, &g.ClientID, &g.Resource, &g.AccessExpiresAt)
	if err != nil {
		return "", "", Grant{}, fmt.Errorf("creating grant: %w", err)
	}
	return access, refresh, g, nil
}

func (s *Store) CreateGrant(ctx context.Context, userID int64, clientID, resource string, accessTTL, refreshTTL time.Duration) (string, string, Grant, error) {
	return insertGrant(ctx, s.pool, userID, clientID, resource, accessTTL, refreshTTL)
}

func (s *Store) RotateRefresh(ctx context.Context, refresh, clientID string, accessTTL, refreshTTL time.Duration) (string, string, Grant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", Grant{}, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var id, userID int64
	var resource string
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, resource
		FROM oauth_grants
		WHERE refresh_hash = $1 AND client_id = $2 AND revoked_at IS NULL AND refresh_expires_at > now()
		FOR UPDATE
	`, HashSecret(refresh), clientID).Scan(&id, &userID, &resource)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", Grant{}, ErrNotFound
		}
		return "", "", Grant{}, fmt.Errorf("looking up grant: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE oauth_grants SET revoked_at = now() WHERE id = $1`, id); err != nil {
		return "", "", Grant{}, fmt.Errorf("revoking grant: %w", err)
	}

	access, newRefresh, g, err := insertGrant(ctx, tx, userID, clientID, resource, accessTTL, refreshTTL)
	if err != nil {
		return "", "", Grant{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", Grant{}, fmt.Errorf("committing tx: %w", err)
	}

	return access, newRefresh, g, nil
}

func (s *Store) GrantByAccess(ctx context.Context, access string) (Grant, User, error) {
	var g Grant
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT g.id, g.user_id, g.client_id, g.resource, g.access_expires_at, u.id, u.username, u.is_admin, u.created_at
		FROM oauth_grants g JOIN users u ON u.id = g.user_id
		WHERE g.access_hash = $1 AND g.revoked_at IS NULL AND g.access_expires_at > now()
	`, HashSecret(access)).Scan(
		&g.ID, &g.UserID, &g.ClientID, &g.Resource, &g.AccessExpiresAt,
		&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Grant{}, User{}, ErrNotFound
		}
		return Grant{}, User{}, fmt.Errorf("looking up grant: %w", err)
	}
	return g, u, nil
}
