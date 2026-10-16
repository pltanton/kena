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
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO stars (user_id, set_id) VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, userID, setID); err != nil {
		return fmt.Errorf("starring set: %w", err)
	}
	return nil
}

func (s *Store) Unstar(ctx context.Context, userID, setID int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM stars WHERE user_id = $1 AND set_id = $2`, userID, setID); err != nil {
		return fmt.Errorf("unstarring set: %w", err)
	}
	return nil
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
	rows, err := s.pool.Query(ctx, `
		SELECT `+setColumns+`, st.set_id IS NOT NULL`+setFromJoin+`
		LEFT JOIN stars st ON st.set_id = s.id AND st.user_id = $1
		WHERE `+where+`
		ORDER BY lower(s.title), s.id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing decks: %w", err)
	}
	defer rows.Close()

	var decks []Deck
	for rows.Next() {
		var d Deck
		if err := rows.Scan(append(d.dest(), &d.Starred)...); err != nil {
			return nil, fmt.Errorf("scanning deck: %w", err)
		}
		decks = append(decks, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing decks: %w", err)
	}
	return decks, nil
}
