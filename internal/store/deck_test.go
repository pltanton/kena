package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func ti(text, answer string) greek.TaskInput {
	return greek.TaskInput{Text: text, Blanks: []greek.Blank{{Answers: []string{answer}}}}
}

func TestSaveDeck(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	ann := mkUser(t, s, "ann", false)
	set := mkSet(t, s, ann.ID, store.SetInput{Title: "Deck"})
	orig := mkTasks(t, s, set.ID, []greek.TaskInput{ti("Α ___.", "α"), ti("Β ___.", "β"), ti("Γ ___.", "γ"), ti("Δ ___.", "δ")})
	const device = "11111111-1111-4111-8111-111111111111"
	for _, task := range orig {
		if err := s.RecordAttempt(ctx, device, task.ID, task.Blanks[0].Answers, true); err != nil {
			t.Fatal(err)
		}
	}

	title, topic := "Deck v2", "aorist"
	changed := ti("Β ___ αλλαγή.", "β")
	changed.Explanation = "νέο"
	res, err := s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{Title: &title, Topic: &topic}, []greek.TaskInput{ti("Α ___.", "α"), changed})
	if err != nil {
		t.Fatal(err)
	}
	if res != (store.DeckChanges{Kept: 1, Updated: 1, Deleted: 2}) {
		t.Fatalf("changes = %+v", res)
	}
	got, err := s.GetSet(ctx, set.Slug)
	if err != nil || got.Title != title || got.Topic != topic || got.TaskCount != 2 {
		t.Fatalf("set after save: %+v, %v", got, err)
	}
	tasks, err := s.ListTasks(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].ID != orig[0].ID || tasks[1].ID != orig[1].ID || tasks[1].Text != changed.Text || tasks[1].Explanation != "νέο" {
		t.Fatalf("tasks after save: %+v", tasks)
	}
	last, err := s.LastAttempts(ctx, device, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := last[orig[0].ID]; !ok || len(last) != 2 {
		t.Fatalf("attempts after save: %+v", last)
	}

	res, err = s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{}, []greek.TaskInput{ti("Α ___.", "α"), changed, ti("Ε ___.", "ε"), ti("Ζ ___.", "ζ")})
	if err != nil {
		t.Fatal(err)
	}
	if res != (store.DeckChanges{Kept: 2, Added: 2}) {
		t.Fatalf("changes = %+v", res)
	}
	tasks, _ = s.ListTasks(ctx, set.ID)
	var texts []string
	for _, task := range tasks {
		texts = append(texts, task.Text)
	}
	if !slices.Equal(texts, []string{"Α ___.", "Β ___ αλλαγή.", "Ε ___.", "Ζ ___."}) || tasks[0].ID != orig[0].ID {
		t.Fatalf("order after add: %v", texts)
	}

	bad := store.Visibility("everyone")
	if _, err := s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{Visibility: &bad}, []greek.TaskInput{ti("Ω ___.", "ω")}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("invalid patch: %v", err)
	}
	if after, _ := s.ListTasks(ctx, set.ID); len(after) != 4 {
		t.Fatalf("failed save changed tasks: %+v", after)
	}

	if _, err := s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{}, nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.ListTasks(ctx, set.ID); len(after) != 0 {
		t.Fatalf("empty save left tasks: %+v", after)
	}

	if _, err := s.SaveDeck(ctx, 999999, time.Time{}, store.SetPatch{}, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing set: %v", err)
	}
}

func newDeck(t *testing.T, s *store.Store, texts ...string) (store.Set, []store.Task) {
	t.Helper()
	ctx := context.Background()
	u, err := s.EnsureUser(ctx, "ann", false)
	if err != nil {
		t.Fatal(err)
	}
	set := mkSet(t, s, u.ID, store.SetInput{Title: "Deck " + texts[0]})
	var in []greek.TaskInput
	for _, x := range texts {
		in = append(in, ti(x+" ___.", x))
	}
	tasks := mkTasks(t, s, set.ID, in)
	return set, tasks
}

func deckOrder(t *testing.T, s *store.Store, setID int64) ([]int64, []string) {
	t.Helper()
	tasks, err := s.ListTasks(context.Background(), setID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	var texts []string
	for i, task := range tasks {
		if task.Position != i+1 {
			t.Errorf("task %d has position %d", i, task.Position)
		}
		ids = append(ids, task.ID)
		texts = append(texts, task.Text)
	}
	return ids, texts
}

func TestSaveDeckMatchesByContent(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	set, orig := newDeck(t, s, "α", "β", "γ", "δ")
	const device = "33333333-3333-4333-8333-333333333333"
	for _, task := range orig {
		if err := s.RecordAttempt(ctx, device, task.ID, task.Blanks[0].Answers, true); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{}, []greek.TaskInput{
		ti("α ___.", "α"), ti("νέο ___.", "νέο"), ti("β ___.", "β"), ti("γ ___.", "γ"), ti("δ ___.", "δ"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != (store.DeckChanges{Kept: 4, Added: 1}) {
		t.Fatalf("insert in the middle: %+v", res)
	}
	ids, texts := deckOrder(t, s, set.ID)
	if !slices.Equal(texts, []string{"α ___.", "νέο ___.", "β ___.", "γ ___.", "δ ___."}) ||
		ids[0] != orig[0].ID || ids[2] != orig[1].ID || ids[3] != orig[2].ID || ids[4] != orig[3].ID {
		t.Fatalf("after insert: %v %v", ids, texts)
	}
	last, err := s.LastAttempts(ctx, device, set.ID)
	if err != nil || len(last) != 4 {
		t.Fatalf("attempts after insert: %v %v", last, err)
	}

	res, err = s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{}, []greek.TaskInput{
		ti("δ ___.", "δ"), ti("γ ___.", "γ"), ti("β ___.", "β"), ti("α ___.", "α"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != (store.DeckChanges{Kept: 4, Deleted: 1}) {
		t.Fatalf("reorder: %+v", res)
	}
	ids, _ = deckOrder(t, s, set.ID)
	if !slices.Equal(ids, []int64{orig[3].ID, orig[2].ID, orig[1].ID, orig[0].ID}) {
		t.Fatalf("reorder ids: %v", ids)
	}

	res, err = s.SaveDeck(ctx, set.ID, time.Time{}, store.SetPatch{}, []greek.TaskInput{
		ti("δ ___.", "δ"), ti("γ2 ___.", "γ"), ti("α ___.", "α"), ti("α ___.", "α"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != (store.DeckChanges{Kept: 2, Updated: 2}) {
		t.Fatalf("edit and duplicate: %+v", res)
	}
	ids, texts = deckOrder(t, s, set.ID)
	if ids[0] != orig[3].ID || ids[1] != orig[2].ID || ids[2] != orig[0].ID || ids[3] != orig[1].ID ||
		!slices.Equal(texts, []string{"δ ___.", "γ2 ___.", "α ___.", "α ___."}) {
		t.Fatalf("leftovers matched by position: %v %v", ids, texts)
	}
}

func TestSaveDeckVersion(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	set, _ := newDeck(t, s, "α", "β")
	set, err := s.GetSet(ctx, set.Slug)
	if err != nil {
		t.Fatal(err)
	}
	opened := set.UpdatedAt

	bump := func(name string, fn func() error) {
		t.Helper()
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetSet(ctx, set.Slug)
		if err != nil {
			t.Fatal(err)
		}
		if !got.UpdatedAt.After(opened) {
			t.Errorf("%s does not bump updated_at", name)
		}
		opened = got.UpdatedAt
	}
	tasks, _ := s.ListTasks(ctx, set.ID)
	bump("AddTasks", func() error { _, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{ti("γ ___.", "γ")}); return err })
	text := "β2 ___."
	bump("UpdateTask", func() error { _, err := s.UpdateTask(ctx, tasks[1].ID, store.TaskPatch{Text: &text}); return err })
	bump("DeleteTask", func() error { return s.DeleteTask(ctx, tasks[0].ID) })

	stale := set.UpdatedAt
	title := "changed"
	if _, err := s.SaveDeck(ctx, set.ID, stale, store.SetPatch{Title: &title}, nil); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale save: %v, want conflict", err)
	}
	if got, _ := s.GetSet(ctx, set.Slug); got.Title == title || got.TaskCount != 2 {
		t.Fatalf("stale save wrote: %+v", got)
	}
	if _, err := s.SaveDeck(ctx, set.ID, opened, store.SetPatch{Title: &title}, nil); err != nil {
		t.Fatalf("fresh save: %v", err)
	}
	got, _ := s.GetSet(ctx, set.Slug)
	if got.Title != title || got.TaskCount != 0 || !got.UpdatedAt.After(opened) {
		t.Fatalf("after fresh save: %+v", got)
	}
}
