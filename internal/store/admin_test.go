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

func TestAdminGuardsAndActions(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	s := store.New(pool)
	root, _ := s.CreateUser(ctx, "root", true)
	anna, _ := s.CreateUser(ctx, "anna", false)

	if err := s.SetAdmin(ctx, root.ID, root.ID, false); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("demoting yourself: %v", err)
	}
	if err := s.DeleteUser(ctx, root.ID, root.ID); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("deleting yourself: %v", err)
	}
	if err := s.SetAdmin(ctx, anna.ID, root.ID, false); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("demoting the last admin: %v", err)
	}
	if err := s.DeleteUser(ctx, anna.ID, root.ID); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("deleting the last admin: %v", err)
	}
	if err := s.SetAdmin(ctx, root.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UserByID(ctx, anna.ID); !u.IsAdmin {
		t.Fatal("anna not admin")
	}
	if err := s.SetAdmin(ctx, anna.ID, root.ID, false); err != nil {
		t.Fatalf("demoting one of two admins: %v", err)
	}
	if err := s.SetAdmin(ctx, root.ID, anna.ID, false); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("root (no longer admin is irrelevant here) demoting the last admin: %v", err)
	}
	if err := s.SetAdmin(ctx, root.ID, 9999, true); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}

	bob, _ := s.CreateUser(ctx, "bob", false)
	set, _ := s.CreateSet(ctx, bob.ID, store.SetInput{Title: "Bob deck"})
	tasks, _ := s.AddTasks(ctx, set.ID, []greek.TaskInput{{Text: "Α ___.", Blanks: []greek.Blank{{Answers: []string{"β"}}}}})
	if err := s.RecordAttempt(ctx, bob.ProgressID, tasks[0].ID, []string{"β"}, true); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateSet(ctx, anna.ID, store.SetInput{Title: "Anna deck"})
	otherTasks, _ := s.AddTasks(ctx, other.ID, []greek.TaskInput{{Text: "Γ ___.", Blanks: []greek.Blank{{Answers: []string{"δ"}}}}})
	if err := s.RecordAttempt(ctx, bob.ProgressID, otherTasks[0].ID, []string{"x"}, false); err != nil {
		t.Fatal(err)
	}
	sess, _ := s.CreateSession(ctx, bob.ID, time.Hour)
	tok, _, _ := s.IssueToken(ctx, bob.ID, "laptop")
	code, _ := s.CreateLoginCode(ctx, bob.ID, time.Minute)

	sums, err := s.ListUserSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bs store.UserSummary
	for _, x := range sums {
		if x.User.ID == bob.ID {
			bs = x
		}
	}
	if bs.Decks != 1 || bs.Sessions != 1 || bs.Tokens != 1 || bs.LastActive == nil {
		t.Fatalf("bob summary = %+v", bs)
	}

	if err := s.LogoutEverywhere(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, sess); !errors.Is(err, store.ErrNotFound) {
		t.Error("session survived logout everywhere")
	}
	if _, err := s.UseLoginCode(ctx, code); !errors.Is(err, store.ErrNotFound) {
		t.Error("login code survived logout everywhere")
	}
	if _, err := s.UserByToken(ctx, tok); err != nil {
		t.Errorf("personal token should survive logout everywhere: %v", err)
	}

	if err := s.DeleteUser(ctx, anna.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByName(ctx, "bob"); !errors.Is(err, store.ErrNotFound) {
		t.Error("bob still exists")
	}
	if _, err := s.GetSet(ctx, set.Slug); !errors.Is(err, store.ErrNotFound) {
		t.Error("bob's deck survived")
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM attempts WHERE device_id = $1::uuid`, bob.ProgressID).Scan(&left); err != nil || left != 0 {
		t.Errorf("bob's progress left: %d, %v", left, err)
	}
	if err := s.DeleteUser(ctx, anna.ID, bob.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}
