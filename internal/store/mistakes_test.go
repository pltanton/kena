package store_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func mistakesFixture(ctx context.Context, t *testing.T, s *store.Store) (store.User, []store.Set, [][]store.Task) {
	t.Helper()
	owner, err := s.CreateUser(ctx, "author", false)
	if err != nil {
		t.Fatal(err)
	}
	learner, err := s.CreateUser(ctx, "learner", false)
	if err != nil {
		t.Fatal(err)
	}
	var sets []store.Set
	var tasks [][]store.Task
	for _, title := range []string{"Alpha", "Beta"} {
		st, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: title, Topic: "aorist", Level: "A2", Visibility: store.Public})
		if err != nil {
			t.Fatal(err)
		}
		ts, err := s.AddTasks(ctx, st.ID, []greek.TaskInput{
			{Text: "Χθες ___ γράμμα.", Blanks: []greek.Blank{{Answers: []string{"έγραψα"}, Hint: "γράφω"}}, Explanation: "αόριστος"},
			{Text: "Εμείς ___ καφέ.", Blanks: []greek.Blank{{Answers: []string{"ήπιαμε"}, Hint: "πίνω"}}},
			{Text: "Αυτός ___ γρήγορα.", Blanks: []greek.Blank{{Answers: []string{"έτρεξε"}, Hint: "τρέχω"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		sets = append(sets, st)
		tasks = append(tasks, ts)
	}
	return learner, sets, tasks
}

func attempt(ctx context.Context, t *testing.T, s *store.Store, device string, taskID int64, answer string, correct bool, ago time.Duration) {
	t.Helper()
	if err := s.RecordAttempt(ctx, device, taskID, []string{answer}, correct); err != nil {
		t.Fatal(err)
	}
	_, err := s.Pool().Exec(ctx, `
		UPDATE attempts SET created_at = now() - make_interval(secs => $1)
		WHERE id = (SELECT max(id) FROM attempts)
	`, ago.Seconds())
	if err != nil {
		t.Fatal(err)
	}
}

func TestMistakesReport(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	learner, sets, tasks := mistakesFixture(ctx, t, s)
	me := learner.ProgressID
	other := "99999999-9999-9999-9999-999999999999"
	since := time.Now().Add(-30 * 24 * time.Hour)

	a0, a1, a2 := tasks[0][0], tasks[0][1], tasks[0][2]
	for i := range 7 {
		attempt(ctx, t, s, me, a0.ID, "εγραψα"+string(rune('0'+i)), false, time.Duration(10-i)*time.Hour)
	}
	attempt(ctx, t, s, me, a1.ID, "πίναμε", false, 3*time.Hour)
	attempt(ctx, t, s, me, a1.ID, "ήπιαμε", true, 2*time.Hour)
	attempt(ctx, t, s, me, a2.ID, "έτρεξε", true, 2*time.Hour)
	attempt(ctx, t, s, me, a2.ID, "ετρεξε", false, 40*24*time.Hour)
	attempt(ctx, t, s, me, tasks[1][0].ID, "γραφω", false, time.Hour)
	attempt(ctx, t, s, other, tasks[1][1].ID, "λάθος", false, time.Hour)

	rep, err := s.MistakesReport(ctx, learner.ID, me, nil, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sets) != 2 || rep.Sets[0].Set.ID != sets[0].ID || rep.Sets[1].Set.ID != sets[1].ID {
		t.Fatalf("sets = %+v", rep.Sets)
	}
	alpha := rep.Sets[0]
	if alpha.Set.Slug != sets[0].Slug || alpha.Set.Title != "Alpha" || alpha.Set.Topic != "aorist" || alpha.Set.Level != "A2" {
		t.Errorf("set = %+v", alpha.Set)
	}
	if alpha.TasksTried != 3 || alpha.Attempts != 10 || alpha.Correct != 2 {
		t.Errorf("alpha totals: tried %d attempts %d correct %d", alpha.TasksTried, alpha.Attempts, alpha.Correct)
	}
	if len(alpha.Tasks) != 2 {
		t.Fatalf("alpha tasks = %+v", alpha.Tasks)
	}
	first, second := alpha.Tasks[0], alpha.Tasks[1]
	if first.Task.ID != a0.ID || first.WrongCount != 7 || first.AttemptCount != 7 || first.LastCorrect {
		t.Errorf("first = %+v", first)
	}
	if first.Task.Text != a0.Text || first.Task.Explanation != "αόριστος" || first.Task.Blanks[0].Hint != "γράφω" {
		t.Errorf("first task = %+v", first.Task)
	}
	if len(first.RecentWrong) != 5 || first.RecentWrong[0].Answers[0] != "εγραψα6" || first.RecentWrong[4].Answers[0] != "εγραψα2" {
		t.Errorf("recent wrong = %+v", first.RecentWrong)
	}
	if first.RecentWrong[0].CreatedAt.Before(first.RecentWrong[1].CreatedAt) {
		t.Errorf("recent wrong not newest first")
	}
	if second.Task.ID != a1.ID || second.WrongCount != 1 || second.AttemptCount != 2 || !second.LastCorrect {
		t.Errorf("second = %+v", second)
	}

	beta := rep.Sets[1]
	if len(beta.Tasks) != 1 || beta.Tasks[0].Task.ID != tasks[1][0].ID {
		t.Errorf("beta = %+v (other device's attempts leaked?)", beta.Tasks)
	}

	filtered, err := s.MistakesReport(ctx, learner.ID, me, &sets[1].ID, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Sets) != 1 || filtered.Sets[0].Set.ID != sets[1].ID {
		t.Fatalf("filtered = %+v", filtered.Sets)
	}

	wide, err := s.MistakesReport(ctx, learner.ID, me, &sets[0].ID, time.Now().Add(-60*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var a2Row *store.TaskMistakes
	for i := range wide.Sets[0].Tasks {
		if wide.Sets[0].Tasks[i].Task.ID == a2.ID {
			a2Row = &wide.Sets[0].Tasks[i]
		}
	}
	if a2Row == nil || !a2Row.LastCorrect || a2Row.WrongCount != 1 {
		t.Errorf("60-day window: a2 = %+v", a2Row)
	}

	none, err := s.MistakesReport(ctx, learner.ID, other, &sets[0].ID, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Sets) != 0 {
		t.Errorf("other device sees %+v", none.Sets)
	}
}

func TestMistakesReportInvalidProgressID(t *testing.T) {
	s := store.New(dbtest.New(t))
	if _, err := s.MistakesReport(context.Background(), 1, "nope", nil, time.Now()); err == nil {
		t.Fatal("expected error")
	}
}

func TestMistakesReportHidesPrivateSets(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	learner, sets, tasks := mistakesFixture(ctx, t, s)
	since := time.Now().Add(-time.Hour * 24)
	attempt(ctx, t, s, learner.ProgressID, tasks[0][0].ID, "λάθος", false, time.Hour)
	attempt(ctx, t, s, learner.ProgressID, tasks[1][0].ID, "λάθος", false, time.Hour)

	titles := func(viewer int64, progress string) []string {
		t.Helper()
		rep, err := s.MistakesReport(ctx, viewer, progress, nil, since)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, sm := range rep.Sets {
			out = append(out, sm.Set.Title)
		}
		return out
	}
	friends, private := store.Friends, store.Private
	if _, err := s.UpdateSet(ctx, sets[0].ID, store.SetPatch{Visibility: &friends}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSet(ctx, sets[1].ID, store.SetPatch{Visibility: &private}); err != nil {
		t.Fatal(err)
	}
	if got := titles(learner.ID, learner.ProgressID); !slices.Equal(got, []string{"Alpha"}) {
		t.Errorf("learner sees %v, want only the friends set", got)
	}

	author, err := s.UserByName(ctx, "author")
	if err != nil {
		t.Fatal(err)
	}
	attempt(ctx, t, s, author.ProgressID, tasks[1][1].ID, "λάθος", false, time.Hour)
	if got := titles(author.ID, author.ProgressID); !slices.Equal(got, []string{"Beta"}) {
		t.Errorf("owner sees %v, want own private set", got)
	}
}
