package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

type Progress struct {
	Total    int
	Solved   int
	Mistakes int
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validateDeviceID(deviceID string) error {
	if !uuidRe.MatchString(deviceID) {
		return fmt.Errorf("invalid device id %q: %w", deviceID, ErrInvalid)
	}
	return nil
}

func (s *Store) RecordAttempt(ctx context.Context, deviceID string, taskID int64, answers []string, correct bool) error {
	if err := validateDeviceID(deviceID); err != nil {
		return err
	}

	if answers == nil {
		answers = []string{}
	}
	answersJSON, err := json.Marshal(answers)
	if err != nil {
		return fmt.Errorf("marshaling answers: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO attempts (device_id, task_id, answers, correct) VALUES ($1, $2, $3, $4)
	`, deviceID, taskID, answersJSON, correct)
	if err != nil {
		return fmt.Errorf("recording attempt: %w", err)
	}
	return nil
}

type LastAttempt struct {
	Answers []string
	Correct bool
}

func (s *Store) LastAttempts(ctx context.Context, deviceID string, setID int64) (map[int64]LastAttempt, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (a.task_id) a.task_id, a.answers, a.correct
		FROM attempts a
		JOIN tasks t ON t.id = a.task_id
		WHERE a.device_id = $1 AND t.set_id = $2
		ORDER BY a.task_id, a.created_at DESC, a.id DESC
	`, deviceID, setID)
	if err != nil {
		return nil, fmt.Errorf("getting last attempts: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]LastAttempt)
	for rows.Next() {
		var taskID int64
		var la LastAttempt
		if err := rows.Scan(&taskID, &la.Answers, &la.Correct); err != nil {
			return nil, fmt.Errorf("scanning last attempt: %w", err)
		}
		out[taskID] = la
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getting last attempts: %w", err)
	}
	return out, nil
}

func (s *Store) ProgressBySet(ctx context.Context, deviceID string) (map[int64]Progress, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		WITH last_attempts AS (
			SELECT DISTINCT ON (task_id) task_id, correct
			FROM attempts
			WHERE device_id = $1
			ORDER BY task_id, created_at DESC, id DESC
		)
		SELECT t.set_id, COUNT(*),
			COUNT(*) FILTER (WHERE la.correct),
			COUNT(*) FILTER (WHERE la.correct = false)
		FROM tasks t
		LEFT JOIN last_attempts la ON la.task_id = t.id
		GROUP BY t.set_id
	`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("getting progress: %w", err)
	}
	defer rows.Close()

	progress := make(map[int64]Progress)
	for rows.Next() {
		var setID int64
		var p Progress
		if err := rows.Scan(&setID, &p.Total, &p.Solved, &p.Mistakes); err != nil {
			return nil, fmt.Errorf("scanning progress: %w", err)
		}
		progress[setID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getting progress: %w", err)
	}
	return progress, nil
}
