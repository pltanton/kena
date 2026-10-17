package store

import (
	"context"
	"fmt"
	"time"
)

const recentWrongLimit = 5

type SetRef struct {
	ID    int64
	Slug  string
	Title string
	Topic string
	Level string
}

type WrongAttempt struct {
	Answers   []string
	CreatedAt time.Time
}

type TaskMistakes struct {
	Task         Task
	WrongCount   int
	AttemptCount int
	LastCorrect  bool
	RuleText     string
	RecentWrong  []WrongAttempt
}

type SetMistakes struct {
	Set        SetRef
	TasksTried int
	Attempts   int
	Correct    int
	Tasks      []TaskMistakes
}

type Report struct {
	Sets []SetMistakes
}

func (s *Store) MistakesReport(ctx context.Context, viewerID int64, progressID string, setID *int64, since time.Time) (Report, error) {
	if err := validateDeviceID(progressID); err != nil {
		return Report{}, err
	}

	rows, err := s.pool.Query(ctx, `
		WITH w AS (
			SELECT a.task_id, count(*) AS attempts, count(*) FILTER (WHERE NOT a.correct) AS wrong
			FROM attempts a
			JOIN tasks t ON t.id = a.task_id
			JOIN sets vs ON vs.id = t.set_id
			WHERE a.device_id = $1 AND a.created_at >= $2 AND ($3::bigint IS NULL OR t.set_id = $3)
				AND (vs.owner_id = $4 OR vs.visibility <> 'private')
			GROUP BY a.task_id
		), last AS (
			SELECT DISTINCT ON (a.task_id) a.task_id, a.correct
			FROM attempts a
			JOIN w ON w.task_id = a.task_id
			WHERE a.device_id = $1
			ORDER BY a.task_id, a.created_at DESC, a.id DESC
		)
		SELECT s.id, s.slug, s.title, s.topic, s.level,
			t.id, t.set_id, t.position, t.text, t.blanks, t.explanation, COALESCE(t.rule, ''),
			COALESCE((SELECT r->>'text' FROM jsonb_array_elements(s.rules) r WHERE r->>'key' = t.rule LIMIT 1), ''),
			w.attempts, w.wrong, last.correct
		FROM w
		JOIN last ON last.task_id = w.task_id
		JOIN tasks t ON t.id = w.task_id
		JOIN sets s ON s.id = t.set_id
		ORDER BY s.title, s.id, w.wrong DESC, t.position
	`, progressID, since, setID, viewerID)
	if err != nil {
		return Report{}, fmt.Errorf("getting mistakes: %w", err)
	}
	defer rows.Close()

	var rep Report
	byTask := map[int64]*TaskMistakes{}
	for rows.Next() {
		var ref SetRef
		var tm TaskMistakes
		t := &tm.Task
		if err := rows.Scan(&ref.ID, &ref.Slug, &ref.Title, &ref.Topic, &ref.Level,
			&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation, &t.Rule, &tm.RuleText,
			&tm.AttemptCount, &tm.WrongCount, &tm.LastCorrect); err != nil {
			return Report{}, fmt.Errorf("scanning mistakes: %w", err)
		}
		if n := len(rep.Sets); n == 0 || rep.Sets[n-1].Set.ID != ref.ID {
			rep.Sets = append(rep.Sets, SetMistakes{Set: ref})
		}
		sm := &rep.Sets[len(rep.Sets)-1]
		sm.TasksTried++
		sm.Attempts += tm.AttemptCount
		sm.Correct += tm.AttemptCount - tm.WrongCount
		if tm.WrongCount > 0 {
			sm.Tasks = append(sm.Tasks, tm)
		}
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("getting mistakes: %w", err)
	}

	out := rep.Sets[:0]
	for _, sm := range rep.Sets {
		if len(sm.Tasks) > 0 {
			out = append(out, sm)
		}
	}
	rep.Sets = out
	for i := range rep.Sets {
		for j := range rep.Sets[i].Tasks {
			tm := &rep.Sets[i].Tasks[j]
			byTask[tm.Task.ID] = tm
		}
	}
	if len(byTask) == 0 {
		return rep, nil
	}

	rows, err = s.pool.Query(ctx, `
		SELECT task_id, answers, created_at FROM (
			SELECT a.task_id, a.answers, a.created_at,
				row_number() OVER (PARTITION BY a.task_id ORDER BY a.created_at DESC, a.id DESC) AS rn
			FROM attempts a
			JOIN tasks t ON t.id = a.task_id
			WHERE a.device_id = $1 AND a.created_at >= $2 AND NOT a.correct AND ($3::bigint IS NULL OR t.set_id = $3)
		) x
		WHERE rn <= $4
		ORDER BY task_id, rn
	`, progressID, since, setID, recentWrongLimit)
	if err != nil {
		return Report{}, fmt.Errorf("getting wrong attempts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID int64
		var wa WrongAttempt
		if err := rows.Scan(&taskID, &wa.Answers, &wa.CreatedAt); err != nil {
			return Report{}, fmt.Errorf("scanning wrong attempt: %w", err)
		}
		if tm, ok := byTask[taskID]; ok {
			tm.RecentWrong = append(tm.RecentWrong, wa)
		}
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("getting wrong attempts: %w", err)
	}
	return rep, nil
}
