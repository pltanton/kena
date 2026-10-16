package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"

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
	ann, err := s.CreateUser(ctx, "ann", false)
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.CreateSet(ctx, ann.ID, store.SetInput{Title: "Deck"})
	if err != nil {
		t.Fatal(err)
	}
	orig, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{ti("Α ___.", "α"), ti("Β ___.", "β"), ti("Γ ___.", "γ"), ti("Δ ___.", "δ")})
	if err != nil {
		t.Fatal(err)
	}
	const device = "11111111-1111-4111-8111-111111111111"
	for _, task := range orig {
		if err := s.RecordAttempt(ctx, device, task.ID, task.Blanks[0].Answers, true); err != nil {
			t.Fatal(err)
		}
	}

	title, topic := "Deck v2", "aorist"
	changed := ti("Β ___ αλλαγή.", "β")
	changed.Explanation = "νέο"
	res, err := s.SaveDeck(ctx, set.ID, store.SetPatch{Title: &title, Topic: &topic}, []greek.TaskInput{ti("Α ___.", "α"), changed})
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

	res, err = s.SaveDeck(ctx, set.ID, store.SetPatch{}, []greek.TaskInput{ti("Α ___.", "α"), changed, ti("Ε ___.", "ε"), ti("Ζ ___.", "ζ")})
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
	if _, err := s.SaveDeck(ctx, set.ID, store.SetPatch{Visibility: &bad}, []greek.TaskInput{ti("Ω ___.", "ω")}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("invalid patch: %v", err)
	}
	if after, _ := s.ListTasks(ctx, set.ID); len(after) != 4 {
		t.Fatalf("failed save changed tasks: %+v", after)
	}

	if _, err := s.SaveDeck(ctx, set.ID, store.SetPatch{}, nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.ListTasks(ctx, set.ID); len(after) != 0 {
		t.Fatalf("empty save left tasks: %+v", after)
	}

	if _, err := s.SaveDeck(ctx, 999999, store.SetPatch{}, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing set: %v", err)
	}
}
