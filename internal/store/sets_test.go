package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"greek transliteration", "Αόριστος: ρήματα σε -ω", "aoristos-rimata-se-o"},
		{"empty", "", "set"},
		{"digits only", "2024", "set-2024"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := store.Slugify(c.input)
			if got != c.want {
				t.Errorf("Slugify(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestSlugifyLongTitleTruncated(t *testing.T) {
	long := strings.Repeat("alpha ", 20)
	got := store.Slugify(long)
	if len(got) > 60 {
		t.Fatalf("Slugify result too long: %q (%d)", got, len(got))
	}
	if strings.HasSuffix(got, "-") {
		t.Fatalf("Slugify result has trailing dash: %q", got)
	}
}

func TestCreateSetSlugCollision(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner1", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	set1, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "My Title"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	if set1.Slug != "my-title" {
		t.Fatalf("expected slug my-title, got %q", set1.Slug)
	}

	set2, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "My Title"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	if set2.Slug != "my-title-2" {
		t.Fatalf("expected slug my-title-2, got %q", set2.Slug)
	}

	set3, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "My Title"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	if set3.Slug != "my-title-3" {
		t.Fatalf("expected slug my-title-3, got %q", set3.Slug)
	}
}

func TestCreateSetBlankTitle(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner2", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	_, err = s.CreateSet(ctx, owner.ID, store.SetInput{Title: "   "})
	if !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestCreateSetExplicitSlug(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner3", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Whatever", Slug: "Custom Slug!"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	if set.Slug != "custom-slug" {
		t.Fatalf("expected slug custom-slug, got %q", set.Slug)
	}
}

func TestGetSetByIDAndSlug(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner4", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	created, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Aorist Verbs"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}

	byID, err := s.GetSet(ctx, fmt.Sprintf("%d", created.ID))
	if err != nil {
		t.Fatalf("GetSet by id: %v", err)
	}
	if byID.ID != created.ID || byID.Slug != created.Slug {
		t.Fatalf("unexpected set by id: %+v", byID)
	}
	if byID.OwnerName != "owner4" {
		t.Fatalf("expected OwnerName owner4, got %q", byID.OwnerName)
	}

	bySlug, err := s.GetSet(ctx, created.Slug)
	if err != nil {
		t.Fatalf("GetSet by slug: %v", err)
	}
	if bySlug.ID != created.ID {
		t.Fatalf("unexpected set by slug: %+v", bySlug)
	}

	if _, err := s.GetSet(ctx, "999999"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown id, got %v", err)
	}
	if _, err := s.GetSet(ctx, "no-such-slug"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown slug, got %v", err)
	}
}

func TestAddTasksPositionsContiguous(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner5", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Set A"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}

	first, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "___ ένα", Blanks: []greek.Blank{{Answers: []string{"ένα"}}}},
		{Text: "___ δύο", Blanks: []greek.Blank{{Answers: []string{"δύο"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}
	if len(first) != 2 || first[0].Position != 1 || first[1].Position != 2 {
		t.Fatalf("unexpected positions: %+v", first)
	}

	second, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "___ τρία", Blanks: []greek.Blank{{Answers: []string{"τρία"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}
	if len(second) != 1 || second[0].Position != 3 {
		t.Fatalf("unexpected position for third task: %+v", second)
	}

	all, err := s.ListTasks(ctx, set.ID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(all))
	}
	for i, task := range all {
		if task.Position != i+1 {
			t.Fatalf("expected contiguous positions, got %+v", all)
		}
	}

	refreshed, err := s.GetSet(ctx, fmt.Sprintf("%d", set.ID))
	if err != nil {
		t.Fatalf("GetSet: %v", err)
	}
	if refreshed.TaskCount != 3 {
		t.Fatalf("expected TaskCount 3, got %d", refreshed.TaskCount)
	}
}

func TestAddTasksSetNotFound(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	_, err := s.AddTasks(ctx, 999999, []greek.TaskInput{{Text: "x"}})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTaskBlanksJSONRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner6", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Set B"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}

	blanks := []greek.Blank{
		{Answers: []string{"γράφω", "γράφεις"}, Hint: "ενεστώτας"},
	}
	created, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "εγώ ___", Blanks: blanks, Explanation: "επεξήγηση"},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	got, err := s.GetTask(ctx, created[0].ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if len(got.Blanks) != 1 || got.Blanks[0].Hint != "ενεστώτας" ||
		len(got.Blanks[0].Answers) != 2 || got.Blanks[0].Answers[0] != "γράφω" || got.Blanks[0].Answers[1] != "γράφεις" {
		t.Fatalf("unexpected blanks round trip: %+v", got.Blanks)
	}
	if got.Explanation != "επεξήγηση" {
		t.Fatalf("unexpected explanation: %q", got.Explanation)
	}
}

func TestUpdateTaskPartial(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner7", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Set C"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	created, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "εγώ ___", Blanks: []greek.Blank{{Answers: []string{"τρέχω"}}}, Explanation: "old"},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	newExplanation := "new explanation"
	updated, err := s.UpdateTask(ctx, created[0].ID, store.TaskPatch{Explanation: &newExplanation})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Explanation != "new explanation" {
		t.Fatalf("expected updated explanation, got %q", updated.Explanation)
	}
	if updated.Text != "εγώ ___" {
		t.Fatalf("expected text unchanged, got %q", updated.Text)
	}
	if len(updated.Blanks) != 1 || updated.Blanks[0].Answers[0] != "τρέχω" {
		t.Fatalf("expected blanks unchanged, got %+v", updated.Blanks)
	}

	refreshedSet, err := s.GetSet(ctx, fmt.Sprintf("%d", set.ID))
	if err != nil {
		t.Fatalf("GetSet: %v", err)
	}
	if !refreshedSet.UpdatedAt.After(set.UpdatedAt) && !refreshedSet.UpdatedAt.Equal(set.UpdatedAt) {
		t.Fatalf("expected set updated_at to be bumped")
	}
}

func TestUpdateSetPartial(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner8", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Original", Topic: "topic1", Level: "A1"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}

	newTopic := "topic2"
	updated, err := s.UpdateSet(ctx, set.ID, store.SetPatch{Topic: &newTopic})
	if err != nil {
		t.Fatalf("UpdateSet: %v", err)
	}
	if updated.Topic != "topic2" {
		t.Fatalf("expected topic2, got %q", updated.Topic)
	}
	if updated.Title != "Original" {
		t.Fatalf("expected title unchanged, got %q", updated.Title)
	}
	if updated.Level != "A1" {
		t.Fatalf("expected level unchanged, got %q", updated.Level)
	}
}

func TestUpdateSetNotFound(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	newTitle := "x"
	_, err := s.UpdateSet(ctx, 999999, store.SetPatch{Title: &newTitle})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteSetCascadesTasks(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner9", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Doomed"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	created, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "___", Blanks: []greek.Blank{{Answers: []string{"x"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	if err := s.DeleteSet(ctx, set.ID); err != nil {
		t.Fatalf("DeleteSet: %v", err)
	}

	if _, err := s.GetTask(ctx, created[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cascaded task, got %v", err)
	}
	if _, err := s.GetSet(ctx, fmt.Sprintf("%d", set.ID)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for deleted set, got %v", err)
	}

	if err := s.DeleteSet(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown set, got %v", err)
	}
}

func TestDeleteTaskLeavesOtherPositions(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	owner, err := s.CreateUser(ctx, "owner10", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	set, err := s.CreateSet(ctx, owner.ID, store.SetInput{Title: "Set D"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	created, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "a", Blanks: []greek.Blank{{Answers: []string{"a"}}}},
		{Text: "b", Blanks: []greek.Blank{{Answers: []string{"b"}}}},
		{Text: "c", Blanks: []greek.Blank{{Answers: []string{"c"}}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	if err := s.DeleteTask(ctx, created[1].ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	remaining, err := s.ListTasks(ctx, set.ID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(remaining) != 2 || remaining[0].Position != 1 || remaining[1].Position != 3 {
		t.Fatalf("expected positions 1 and 3, got %+v", remaining)
	}

	if err := s.DeleteTask(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCanEdit(t *testing.T) {
	owner := store.User{ID: 1}
	admin := store.User{ID: 2, IsAdmin: true}
	other := store.User{ID: 3}
	set := store.Set{OwnerID: 1}

	if !store.CanEdit(owner, set) {
		t.Fatal("expected owner to be able to edit")
	}
	if !store.CanEdit(admin, set) {
		t.Fatal("expected admin to be able to edit")
	}
	if store.CanEdit(other, set) {
		t.Fatal("expected other user to not be able to edit")
	}
}

func TestListSetsOwnerNameAndTaskCount(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	ann, err := s.CreateUser(ctx, "ann", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bob, err := s.CreateUser(ctx, "bob2", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	annSet, err := s.CreateSet(ctx, ann.ID, store.SetInput{Title: "Ann Set"})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	if _, err := s.AddTasks(ctx, annSet.ID, []greek.TaskInput{
		{Text: "x", Blanks: []greek.Blank{{Answers: []string{"x"}}}},
	}); err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	if _, err := s.CreateSet(ctx, bob.ID, store.SetInput{Title: "Bob Set"}); err != nil {
		t.Fatalf("CreateSet: %v", err)
	}

	all, err := s.ListSets(ctx, "")
	if err != nil {
		t.Fatalf("ListSets: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 sets, got %d", len(all))
	}

	annOnly, err := s.ListSets(ctx, "ann")
	if err != nil {
		t.Fatalf("ListSets filtered: %v", err)
	}
	if len(annOnly) != 1 || annOnly[0].OwnerName != "ann" || annOnly[0].TaskCount != 1 {
		t.Fatalf("unexpected filtered sets: %+v", annOnly)
	}
}
