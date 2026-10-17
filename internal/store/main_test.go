package store_test

import (
	"context"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func TestMain(m *testing.M) {
	dbtest.Main(m)
}

func mkUser(t *testing.T, s *store.Store, name string, admin bool) store.User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), name, admin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func mkSet(t *testing.T, s *store.Store, ownerID int64, in store.SetInput) store.Set {
	t.Helper()
	set, err := s.CreateSet(context.Background(), ownerID, in)
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	return set
}

func mkTasks(t *testing.T, s *store.Store, setID int64, in []greek.TaskInput) []store.Task {
	t.Helper()
	tasks, err := s.AddTasks(context.Background(), setID, in)
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}
	return tasks
}
