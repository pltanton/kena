package store

import (
	"context"
	"slices"
	"time"

	"github.com/pltanton/kena/internal/greek"
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
	Rule         greek.Rule
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

	type row struct {
		ref SetRef
		tm  TaskMistakes
	}
	rows, err := queryAll(ctx, s.pool, "getting mistakes", func(r *row) []any {
		t := &r.tm.Task
		return []any{&r.ref.ID, &r.ref.Slug, &r.ref.Title, &r.ref.Topic, &r.ref.Level,
			&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation, &t.Rule, &r.tm.Rule,
			&r.tm.AttemptCount, &r.tm.WrongCount, &r.tm.LastCorrect}
	}, `
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
			t.id, t.set_id, t.position, t.text, t.blanks, t.explanation, COALESCE(t.rule, public_rule_key(t.public_rule_id), ''),
			COALESCE((SELECT r FROM jsonb_array_elements(s.rules) r WHERE r->>'key' = t.rule LIMIT 1),
				(SELECT jsonb_build_object('key', public_rule_key(p.id), 'title', p.title, 'body', p.body) FROM public_rules p WHERE p.id = t.public_rule_id),
				'{}'::jsonb),
			w.attempts, w.wrong, last.correct
		FROM w
		JOIN last ON last.task_id = w.task_id
		JOIN tasks t ON t.id = w.task_id
		JOIN sets s ON s.id = t.set_id
		ORDER BY s.title, s.id, w.wrong DESC, t.position
	`, progressID, since, setID, viewerID)
	if err != nil {
		return Report{}, err
	}

	var rep Report
	for _, r := range rows {
		if n := len(rep.Sets); n == 0 || rep.Sets[n-1].Set.ID != r.ref.ID {
			rep.Sets = append(rep.Sets, SetMistakes{Set: r.ref})
		}
		sm := &rep.Sets[len(rep.Sets)-1]
		sm.TasksTried++
		sm.Attempts += r.tm.AttemptCount
		sm.Correct += r.tm.AttemptCount - r.tm.WrongCount
		if r.tm.WrongCount > 0 {
			sm.Tasks = append(sm.Tasks, r.tm)
		}
	}
	rep.Sets = slices.DeleteFunc(rep.Sets, func(sm SetMistakes) bool { return len(sm.Tasks) == 0 })
	byTask := map[int64]*TaskMistakes{}
	for i := range rep.Sets {
		for j := range rep.Sets[i].Tasks {
			tm := &rep.Sets[i].Tasks[j]
			byTask[tm.Task.ID] = tm
		}
	}
	if len(byTask) == 0 {
		return rep, nil
	}

	type wrong struct {
		taskID int64
		wa     WrongAttempt
	}
	wrongs, err := queryAll(ctx, s.pool, "getting wrong attempts", func(w *wrong) []any {
		return []any{&w.taskID, &w.wa.Answers, &w.wa.CreatedAt}
	}, `
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
		return Report{}, err
	}
	for _, w := range wrongs {
		if tm, ok := byTask[w.taskID]; ok {
			tm.RecentWrong = append(tm.RecentWrong, w.wa)
		}
	}
	return rep, nil
}
