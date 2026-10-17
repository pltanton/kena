package store

import (
	"context"
	"fmt"
	"regexp"
	"time"
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
	_, err := s.Record(ctx, deviceID, Attempt{TaskID: taskID, Answers: answers, Correct: correct})
	return err
}

func (s *Store) RecordGiveUp(ctx context.Context, deviceID string, taskID int64, answers []string) error {
	_, err := s.Record(ctx, deviceID, Attempt{TaskID: taskID, Answers: answers, GaveUp: true})
	return err
}

type Attempt struct {
	ClientID string
	TaskID   int64
	Answers  []string
	Correct  bool
	GaveUp   bool
	At       time.Time
}

func (s *Store) Record(ctx context.Context, deviceID string, a Attempt) (bool, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return false, err
	}
	var clientID, at any
	if a.ClientID != "" {
		if !uuidRe.MatchString(a.ClientID) {
			return false, fmt.Errorf("invalid client id %q: %w", a.ClientID, ErrInvalid)
		}
		clientID = a.ClientID
	}
	if !a.At.IsZero() {
		at = a.At
	}
	answersJSON, err := jsonList(a.Answers)
	if err != nil {
		return false, err
	}
	n, err := exec(ctx, s.pool, "recording attempt", `
		INSERT INTO attempts (device_id, task_id, answers, correct, gave_up, client_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, now()))
		ON CONFLICT (client_id) DO NOTHING
	`, deviceID, a.TaskID, answersJSON, a.Correct, a.GaveUp, clientID, at)
	return n == 1, err
}

type LastAttempt struct {
	Answers []string
	Correct bool
	GaveUp  bool
}

func (s *Store) LastAttempts(ctx context.Context, deviceID string, setID int64) (map[int64]LastAttempt, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	type row struct {
		id int64
		la LastAttempt
	}
	rows, err := queryAll(ctx, s.pool, "getting last attempts", func(r *row) []any {
		return []any{&r.id, &r.la.Answers, &r.la.Correct, &r.la.GaveUp}
	}, `
		SELECT DISTINCT ON (a.task_id) a.task_id, a.answers, a.correct, a.gave_up
		FROM attempts a
		JOIN tasks t ON t.id = a.task_id
		WHERE a.device_id = $1 AND t.set_id = $2
		ORDER BY a.task_id, a.created_at DESC, a.id DESC
	`, deviceID, setID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]LastAttempt, len(rows))
	for _, r := range rows {
		out[r.id] = r.la
	}
	return out, nil
}

func (s *Store) ProgressBySet(ctx context.Context, deviceID string) (map[int64]Progress, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	type row struct {
		id int64
		p  Progress
	}
	rows, err := queryAll(ctx, s.pool, "getting progress", func(r *row) []any {
		return []any{&r.id, &r.p.Total, &r.p.Solved, &r.p.Mistakes}
	}, `
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
		return nil, err
	}
	out := make(map[int64]Progress, len(rows))
	for _, r := range rows {
		out[r.id] = r.p
	}
	return out, nil
}

func (s *Store) ResetProgress(ctx context.Context, deviceID string, setID int64) (int64, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return 0, err
	}
	return exec(ctx, s.pool, "resetting progress", `
		DELETE FROM attempts a USING tasks t
		WHERE a.task_id = t.id AND t.set_id = $2 AND a.device_id = $1
	`, deviceID, setID)
}

type AttemptStamp struct {
	Count int
	MaxID int64
}

func (s *Store) AttemptStamps(ctx context.Context, deviceID string, setIDs []int64) (map[int64]AttemptStamp, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}
	type row struct {
		id int64
		st AttemptStamp
	}
	rows, err := queryAll(ctx, s.pool, "getting attempt stamps", func(r *row) []any {
		return []any{&r.id, &r.st.Count, &r.st.MaxID}
	}, `
		SELECT t.set_id, count(*), max(a.id)
		FROM attempts a
		JOIN tasks t ON t.id = a.task_id
		WHERE a.device_id = $1 AND t.set_id = ANY($2)
		GROUP BY t.set_id
	`, deviceID, setIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]AttemptStamp, len(rows))
	for _, r := range rows {
		out[r.id] = r.st
	}
	return out, nil
}
