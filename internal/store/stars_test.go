package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/store"
)

func deckTitles(decks []store.Deck) []string {
	var out []string
	for _, d := range decks {
		s := d.Title
		if d.Starred {
			s += "*"
		}
		out = append(out, s)
	}
	return out
}

func TestStars(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	ann, err := s.CreateUser(ctx, "ann", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.CreateUser(ctx, "bob", false)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(owner int64, title string, v store.Visibility) store.Set {
		t.Helper()
		set, err := s.CreateSet(ctx, owner, store.SetInput{Title: title, Visibility: v})
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	annPrivate := mk(ann.ID, "Ann private", store.Private)
	annShared := mk(ann.ID, "Ann shared", store.Public)
	annOther := mk(ann.ID, "Ann other", store.Public)
	annFriends := mk(ann.ID, "Ann friends", store.Friends)
	bobOwn := mk(bob.ID, "Bob own", store.Private)

	if err := s.Star(ctx, bob.ID, annPrivate.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("star private: %v, want not found", err)
	}
	if err := s.Star(ctx, bob.ID, bobOwn.ID); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("star own: %v, want invalid", err)
	}
	if err := s.Star(ctx, bob.ID, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("star missing: %v, want not found", err)
	}
	for range 2 {
		if err := s.Star(ctx, bob.ID, annShared.ID); err != nil {
			t.Fatalf("star: %v", err)
		}
	}
	if ok, err := s.IsStarred(ctx, bob.ID, annShared.ID); err != nil || !ok {
		t.Fatalf("IsStarred: %v, %v", ok, err)
	}
	if ok, _ := s.IsStarred(ctx, ann.ID, annShared.ID); ok {
		t.Fatal("star leaked to another user")
	}

	my := func(u int64) string {
		t.Helper()
		d, err := s.MyDecks(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(deckTitles(d))
	}
	browse := func(u int64) string {
		t.Helper()
		d, err := s.BrowseDecks(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(deckTitles(d))
	}
	for _, c := range []struct{ name, got, want string }{
		{"bob my", my(bob.ID), "[Ann shared* Bob own]"},
		{"ann my", my(ann.ID), "[Ann friends Ann other Ann private Ann shared]"},
		{"bob browse", browse(bob.ID), "[Ann friends Ann other Ann shared*]"},
		{"ann browse", browse(ann.ID), "[]"},
		{"anon browse", browse(0), "[Ann other Ann shared]"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %s, want %s", c.name, c.got, c.want)
		}
	}

	if err := s.Star(ctx, bob.ID, annFriends.ID); err != nil {
		t.Fatalf("star friends deck: %v", err)
	}
	if got := my(bob.ID); got != "[Ann friends* Ann shared* Bob own]" {
		t.Errorf("starred friends deck: %s", got)
	}
	if err := s.Unstar(ctx, bob.ID, annFriends.ID); err != nil {
		t.Fatal(err)
	}

	off := store.Private
	if _, err := s.UpdateSet(ctx, annShared.ID, store.SetPatch{Visibility: &off}); err != nil {
		t.Fatal(err)
	}
	if got := my(bob.ID); got != "[Bob own]" {
		t.Errorf("private-again set still listed: %s", got)
	}
	if err := s.Star(ctx, bob.ID, annShared.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("star private-again: %v", err)
	}
	on := store.Friends
	if _, err := s.UpdateSet(ctx, annShared.ID, store.SetPatch{Visibility: &on}); err != nil {
		t.Fatal(err)
	}
	if got := my(bob.ID); got != "[Ann shared* Bob own]" {
		t.Errorf("shared again: %s", got)
	}

	for range 2 {
		if err := s.Unstar(ctx, bob.ID, annShared.ID); err != nil {
			t.Fatalf("unstar: %v", err)
		}
	}
	if got := my(bob.ID); got != "[Bob own]" {
		t.Errorf("after unstar: %s", got)
	}

	if err := s.Star(ctx, bob.ID, annOther.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSet(ctx, annOther.ID); err != nil {
		t.Fatalf("delete starred set: %v", err)
	}
	if got := my(bob.ID); got != "[Bob own]" {
		t.Errorf("after delete: %s", got)
	}
}
