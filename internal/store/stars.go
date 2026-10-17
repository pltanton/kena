package store

import (
	"context"
	"fmt"
)

type Deck struct {
	Set
	Starred bool
}

func (s *Store) Star(ctx context.Context, userID, setID int64) error {
	set, err := s.setByID(ctx, setID)
	if err != nil {
		return err
	}
	if set.OwnerID == userID {
		return fmt.Errorf("starring own set: %w", ErrInvalid)
	}
	if !CanView(userID, set) {
		return ErrNotFound
	}
	_, err = exec(ctx, s.pool, "starring set", `INSERT INTO stars (user_id, set_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, setID)
	return err
}

func (s *Store) Unstar(ctx context.Context, userID, setID int64) error {
	_, err := exec(ctx, s.pool, "unstarring set", `DELETE FROM stars WHERE user_id = $1 AND set_id = $2`, userID, setID)
	return err
}

func (s *Store) IsStarred(ctx context.Context, userID, setID int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM stars WHERE user_id = $1 AND set_id = $2)
	`, userID, setID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("checking star: %w", err)
	}
	return ok, nil
}

func (s *Store) MyDecks(ctx context.Context, userID int64) ([]Deck, error) {
	return s.decks(ctx, `s.owner_id = $1 OR (st.set_id IS NOT NULL AND s.visibility <> 'private')`, userID)
}

func (s *Store) BrowseDecks(ctx context.Context, viewerID int64) ([]Deck, error) {
	return s.decks(ctx, `s.owner_id <> $1 AND (s.visibility = 'public' OR (s.visibility = 'friends' AND $1 <> 0))`, viewerID)
}

func (s *Store) decks(ctx context.Context, where string, userID int64) ([]Deck, error) {
	return queryAll(ctx, s.pool, "listing decks", func(d *Deck) []any { return append(d.dest(), &d.Starred) }, `
		SELECT `+setColumns+`, st.set_id IS NOT NULL`+setFromJoin+`
		LEFT JOIN stars st ON st.set_id = s.id AND st.user_id = $1
		WHERE `+where+`
		ORDER BY lower(s.title), s.id
	`, userID)
}
