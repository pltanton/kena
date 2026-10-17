package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func newTaskForAttempts(ctx context.Context, t *testing.T, s *store.Store, username string) store.Task {
	t.Helper()
	owner, err := s.CreateUser(ctx, username, false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Attempts Set " + username})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	tasks, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "εγώ ___", Blanks: []greek.Blank{{Answers: []string{"τρέχω"}}}},
		{Text: "εσύ ___", Blanks: []greek.Blank{{Answers: []string{"τρέχεις"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}
	return tasks[0]
}

func TestRecordAttemptWrongThenRightIsSolved(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	task := newTaskForAttempts(ctx, t, s, "learner1")
	device := "11111111-1111-1111-1111-111111111111"

	if err := s.RecordAttempt(ctx, device, task.ID, []string{"wrong"}, false); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if err := s.RecordAttempt(ctx, device, task.ID, []string{"τρέχω"}, true); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	results, err := s.LastAttempts(ctx, device, task.SetID)
	if err != nil {
		t.Fatalf("LastAttempts: %v", err)
	}
	if last, ok := results[task.ID]; !ok || !last.Correct {
		t.Fatalf("expected last result correct=true, got %+v (ok=%v)", last, ok)
	}
}

func TestRecordAttemptRightThenWrongIsMistake(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	task := newTaskForAttempts(ctx, t, s, "learner2")
	device := "22222222-2222-2222-2222-222222222222"

	if err := s.RecordAttempt(ctx, device, task.ID, []string{"τρέχω"}, true); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if err := s.RecordAttempt(ctx, device, task.ID, []string{"wrong"}, false); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	results, err := s.LastAttempts(ctx, device, task.SetID)
	if err != nil {
		t.Fatalf("LastAttempts: %v", err)
	}
	if last, ok := results[task.ID]; !ok || last.Correct {
		t.Fatalf("expected last result correct=false, got %+v (ok=%v)", last, ok)
	}
}

func TestLastAttemptsUntouchedTaskAbsent(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	task := newTaskForAttempts(ctx, t, s, "learner3")
	device := "33333333-3333-3333-3333-333333333333"

	if err := s.RecordAttempt(ctx, device, task.ID, []string{"τρέχω"}, true); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	results, err := s.LastAttempts(ctx, device, task.SetID)
	if err != nil {
		t.Fatalf("LastAttempts: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected only the attempted task present, got %+v", results)
	}

	progress, err := s.ProgressBySet(ctx, device)
	if err != nil {
		t.Fatalf("ProgressBySet: %v", err)
	}
	p := progress[task.SetID]
	if p.Total != 2 || p.Solved != 1 || p.Mistakes != 0 {
		t.Fatalf("unexpected progress: %+v", p)
	}
}

func TestLastAttemptsOtherDeviceIsolated(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	task := newTaskForAttempts(ctx, t, s, "learner4")
	deviceA := "44444444-4444-4444-4444-444444444444"
	deviceB := "55555555-5555-5555-5555-555555555555"

	if err := s.RecordAttempt(ctx, deviceA, task.ID, []string{"τρέχω"}, true); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	resultsB, err := s.LastAttempts(ctx, deviceB, task.SetID)
	if err != nil {
		t.Fatalf("LastAttempts: %v", err)
	}
	if len(resultsB) != 0 {
		t.Fatalf("expected no results for unrelated device, got %+v", resultsB)
	}

	resultsA, err := s.LastAttempts(ctx, deviceA, task.SetID)
	if err != nil {
		t.Fatalf("LastAttempts: %v", err)
	}
	if len(resultsA) != 1 {
		t.Fatalf("expected one result for device A, got %+v", resultsA)
	}
}

func TestProgressBySetCoversTwoSets(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "learner5", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	setA, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Set A for progress"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	tasksA, err := s.AddTasks(ctx, setA.ID, []greek.TaskInput{
		{Text: "a1", Blanks: []greek.Blank{{Answers: []string{"a"}}}},
		{Text: "a2", Blanks: []greek.Blank{{Answers: []string{"b"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	setB, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Set B for progress"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	tasksB, err := s.AddTasks(ctx, setB.ID, []greek.TaskInput{
		{Text: "b1", Blanks: []greek.Blank{{Answers: []string{"c"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	device := "66666666-6666-6666-6666-666666666666"
	if err := s.RecordAttempt(ctx, device, tasksA[0].ID, []string{"a"}, true); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if err := s.RecordAttempt(ctx, device, tasksA[1].ID, []string{"wrong"}, false); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if err := s.RecordAttempt(ctx, device, tasksB[0].ID, []string{"wrong"}, false); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	progress, err := s.ProgressBySet(ctx, device)
	if err != nil {
		t.Fatalf("ProgressBySet: %v", err)
	}

	pA := progress[setA.ID]
	if pA.Total != 2 || pA.Solved != 1 || pA.Mistakes != 1 {
		t.Fatalf("unexpected progress for set A: %+v", pA)
	}

	pB := progress[setB.ID]
	if pB.Total != 1 || pB.Solved != 0 || pB.Mistakes != 1 {
		t.Fatalf("unexpected progress for set B: %+v", pB)
	}
}

func TestInvalidDeviceID(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	task := newTaskForAttempts(ctx, t, s, "learner6")

	if err := s.RecordAttempt(ctx, "not-a-uuid", task.ID, []string{"x"}, true); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("expected ErrInvalid from RecordAttempt, got %v", err)
	}
	if _, err := s.LastAttempts(ctx, "not-a-uuid", task.SetID); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("expected ErrInvalid from LastAttempts, got %v", err)
	}
	if _, err := s.ProgressBySet(ctx, "not-a-uuid"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("expected ErrInvalid from ProgressBySet, got %v", err)
	}
}

func TestLastAttempts(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	task := newTaskForAttempts(ctx, t, s, "learner9")
	device := "33333333-3333-3333-3333-333333333333"

	for _, a := range []struct {
		answer  string
		correct bool
	}{{"λάθος", false}, {"τρέχω", true}, {"τρεχω", false}} {
		if err := s.RecordAttempt(ctx, device, task.ID, []string{a.answer}, a.correct); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.LastAttempts(ctx, device, task.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("LastAttempts = %+v", got)
	}
	if la := got[task.ID]; la.Correct || len(la.Answers) != 1 || la.Answers[0] != "τρεχω" {
		t.Fatalf("last = %+v", la)
	}
	if other, err := s.LastAttempts(ctx, "44444444-4444-4444-4444-444444444444", task.SetID); err != nil || len(other) != 0 {
		t.Fatalf("other device = %+v, %v", other, err)
	}
	if _, err := s.LastAttempts(ctx, "bad", task.SetID); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("invalid device: %v", err)
	}
}

func TestResetProgressOnlyTouchesThatDeviceAndSet(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	task := newTaskForAttempts(ctx, t, s, "resetter")
	other := newTaskForAttempts(ctx, t, s, "other")
	me := "11111111-1111-1111-1111-111111111111"
	them := "22222222-2222-2222-2222-222222222222"
	for _, a := range []struct {
		device string
		task   int64
	}{{me, task.ID}, {me, task.ID}, {me, other.ID}, {them, task.ID}} {
		if err := s.RecordAttempt(ctx, a.device, a.task, []string{"x"}, false); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.ResetProgress(ctx, me, task.SetID)
	if err != nil || n != 2 {
		t.Fatalf("ResetProgress = %d, %v; want 2", n, err)
	}
	for _, c := range []struct {
		device string
		set    int64
		want   int
	}{{me, task.SetID, 0}, {me, other.SetID, 1}, {them, task.SetID, 1}} {
		got, err := s.LastAttempts(ctx, c.device, c.set)
		if err != nil || len(got) != c.want {
			t.Errorf("LastAttempts(%s, %d) = %d tasks, %v; want %d", c.device, c.set, len(got), err, c.want)
		}
	}
	if _, err := s.ResetProgress(ctx, "nope", task.SetID); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("bad device id: %v", err)
	}
}
