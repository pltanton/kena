package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func TestPublicRules(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	admin := mkUser(t, s, "root", true)
	anna := mkUser(t, s, "anna", false)
	boris := mkUser(t, s, "boris", false)

	if _, err := s.CreateRuleCategory(ctx, anna, "ellenika"); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("non-admin category: %v", err)
	}
	if _, err := s.CreateRuleCategory(ctx, admin, "Ellenika"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("bad category name: %v", err)
	}
	if _, err := s.CreateRuleCategory(ctx, admin, "ellenika"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRuleCategory(ctx, admin, "ellenika"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate category: %v", err)
	}

	in := store.PublicRuleInput{Title: "Πόσος", Body: "Το πόσος συμφωνεί."}
	if _, _, err := s.SavePublicRule(ctx, anna, "nope/posos", time.Time{}, in); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("missing category: %v", err)
	}
	p, created, err := s.SavePublicRule(ctx, anna, "ellenika/posos", time.Time{}, in)
	if err != nil || !created || p.OwnerID != anna.ID || p.Key() != "ellenika/posos" {
		t.Fatalf("create: %+v %v %v", p, created, err)
	}
	if _, _, err := s.SavePublicRule(ctx, boris, "ellenika/posos", time.Time{}, in); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("other user edit: %v", err)
	}
	if _, _, err := s.SavePublicRule(ctx, anna, "ellenika/posos", p.UpdatedAt.Add(-time.Second), in); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	in.Body = "Νέο σώμα."
	if p2, created, err := s.SavePublicRule(ctx, admin, "ellenika/posos", p.UpdatedAt, in); err != nil || created || p2.Body != "Νέο σώμα." {
		t.Fatalf("admin edit: %+v %v %v", p2, created, err)
	}

	set := mkSet(t, s, boris.ID, store.SetInput{Title: "Deck", Rules: []greek.Rule{{Key: "local", Body: "local body"}}})
	if _, err := s.AddTasks(ctx, set.ID, []greek.TaskInput{{Text: "a ___", Blanks: []greek.Blank{{Answers: []string{"b"}}}, Rule: "ellenika/missing"}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("unknown public rule: %v", err)
	}
	tasks := mkTasks(t, s, set.ID, []greek.TaskInput{
		{Text: "a ___", Blanks: []greek.Blank{{Answers: []string{"b"}}}, Rule: "ellenika/posos"},
		{Text: "c ___", Blanks: []greek.Blank{{Answers: []string{"d"}}}, Rule: "local"},
	})
	if tasks[0].Rule != "ellenika/posos" || tasks[1].Rule != "local" {
		t.Fatalf("task rules = %q, %q", tasks[0].Rule, tasks[1].Rule)
	}
	got, _ := s.GetSet(ctx, set.Slug)
	if len(got.Public) != 1 || got.Public[0].Key != "ellenika/posos" || got.Public[0].General || got.Public[0].Body != "Νέο σώμα." {
		t.Fatalf("Public = %+v", got.Public)
	}
	if all := got.AllRules(); len(all) != 2 || all[0].Key != "local" {
		t.Fatalf("AllRules = %+v", all)
	}

	stamp := got.PublicStamp
	in.Body = "Τρίτο σώμα."
	if _, _, err := s.SavePublicRule(ctx, anna, "ellenika/posos", time.Time{}, in); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetSet(ctx, set.Slug)
	if !after.PublicStamp.After(stamp) || !after.UpdatedAt.Equal(got.UpdatedAt) {
		t.Fatalf("stamp %v -> %v, updated %v -> %v", stamp, after.PublicStamp, got.UpdatedAt, after.UpdatedAt)
	}

	if err := s.DeletePublicRule(ctx, anna, "ellenika/posos"); !errors.As(err, new(*store.PublicRuleInUseError)) {
		t.Fatalf("delete in use: %v", err)
	}
	if err := s.DeleteUser(ctx, admin.ID, anna.ID); !errors.Is(err, store.ErrOwnsPublicRules) {
		t.Fatalf("delete owner: %v", err)
	}

	local := "local"
	if tk, err := s.UpdateTask(ctx, tasks[0].ID, store.TaskPatch{Rule: &local}); err != nil || tk.Rule != "local" {
		t.Fatalf("switch to local: %+v %v", tk, err)
	}
	pub := "ellenika/posos"
	if tk, err := s.UpdateTask(ctx, tasks[1].ID, store.TaskPatch{Rule: &pub}); err != nil || tk.Rule != pub {
		t.Fatalf("switch to public: %+v %v", tk, err)
	}
	cur, _ := s.GetSet(ctx, set.Slug)
	if _, err := s.SaveDeck(ctx, set.ID, cur.UpdatedAt, store.SetPatch{}, []greek.TaskInput{
		{Text: "a ___", Blanks: []greek.Blank{{Answers: []string{"b"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	if cur, _ = s.GetSet(ctx, set.Slug); len(cur.Public) != 0 {
		t.Fatalf("Public after dropping task = %+v", cur.Public)
	}

	attached, err := s.AttachPublicRule(ctx, set.ID, "ellenika/posos")
	if err != nil || len(attached.Public) != 1 || !attached.Public[0].General || !attached.UpdatedAt.After(cur.UpdatedAt) {
		t.Fatalf("attach: %+v %v", attached.Public, err)
	}
	if again, err := s.AttachPublicRule(ctx, set.ID, "ellenika/posos"); err != nil || len(again.Public) != 1 {
		t.Fatalf("attach twice: %+v %v", again.Public, err)
	}
	if _, err := s.AttachPublicRule(ctx, set.ID, "ellenika/missing"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("attach missing: %v", err)
	}
	if use, err := s.PublicRuleUse(ctx, "ellenika/posos"); err != nil || use.Decks != 1 || use.Tasks != 0 {
		t.Fatalf("use = %+v %v", use, err)
	}
	if err := s.DeletePublicRule(ctx, anna, "ellenika/posos"); !errors.As(err, new(*store.PublicRuleInUseError)) {
		t.Fatalf("delete attached: %v", err)
	}
	if err := s.DeleteRuleCategory(ctx, admin, "ellenika"); !errors.Is(err, store.ErrCategoryNotEmpty) {
		t.Fatalf("delete non-empty category: %v", err)
	}
	if d, err := s.DetachPublicRule(ctx, set.ID, "ellenika/posos"); err != nil || len(d.Public) != 0 {
		t.Fatalf("detach: %+v %v", d.Public, err)
	}
	if _, err := s.DetachPublicRule(ctx, set.ID, "ellenika/posos"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("detach twice: %v", err)
	}

	if n, err := s.TransferPublicRules(ctx, admin, anna.ID, "boris"); err != nil || n != 1 {
		t.Fatalf("transfer: %d %v", n, err)
	}
	if err := s.DeleteUser(ctx, admin.ID, anna.ID); err != nil {
		t.Fatalf("delete after transfer: %v", err)
	}
	if err := s.DeletePublicRule(ctx, anna, "ellenika/posos"); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("delete by former owner: %v", err)
	}
	if err := s.DeletePublicRule(ctx, boris, "ellenika/posos"); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
	if err := s.DeleteRuleCategory(ctx, admin, "ellenika"); err != nil {
		t.Fatalf("delete empty category: %v", err)
	}
}

func TestAttachLimit(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	admin := mkUser(t, s, "root", true)
	if _, err := s.CreateRuleCategory(ctx, admin, "c"); err != nil {
		t.Fatal(err)
	}
	set := mkSet(t, s, admin.ID, store.SetInput{Title: "Deck"})
	for i := range store.MaxAttachedPublicRules + 1 {
		key := "c/r" + string(rune('a'+i))
		if _, _, err := s.SavePublicRule(ctx, admin, key, time.Time{}, store.PublicRuleInput{Body: "b"}); err != nil {
			t.Fatal(err)
		}
		_, err := s.AttachPublicRule(ctx, set.ID, key)
		if i < store.MaxAttachedPublicRules && err != nil || i == store.MaxAttachedPublicRules && !errors.Is(err, store.ErrTooManyAttached) {
			t.Fatalf("attach %d: %v", i, err)
		}
	}
}

func TestMistakesPublicRule(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	admin := mkUser(t, s, "root", true)
	if _, err := s.CreateRuleCategory(ctx, admin, "c"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SavePublicRule(ctx, admin, "c/r", time.Time{}, store.PublicRuleInput{Title: "T", Body: "B"}); err != nil {
		t.Fatal(err)
	}
	set := mkSet(t, s, admin.ID, store.SetInput{Title: "Deck"})
	tasks := mkTasks(t, s, set.ID, []greek.TaskInput{{Text: "a ___", Blanks: []greek.Blank{{Answers: []string{"b"}}}, Rule: "c/r"}})
	if err := s.RecordAttempt(ctx, admin.ProgressID, tasks[0].ID, []string{"x"}, false); err != nil {
		t.Fatal(err)
	}
	rep, err := s.MistakesReport(ctx, admin.ID, admin.ProgressID, nil, time.Time{})
	if err != nil || len(rep.Sets) != 1 {
		t.Fatalf("report: %+v %v", rep, err)
	}
	tm := rep.Sets[0].Tasks[0]
	if tm.Task.Rule != "c/r" || tm.Rule.Key != "c/r" || tm.Rule.Title != "T" || tm.Rule.Body != "B" {
		t.Fatalf("mistake = %+v", tm)
	}
}
