package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/store"
)

func TestLoginCodes(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	s := store.New(pool)
	u, err := s.CreateUser(ctx, "anna", false)
	if err != nil {
		t.Fatal(err)
	}

	code, err := s.CreateLoginCode(ctx, u.ID, 5*time.Minute)
	if err != nil || !strings.HasPrefix(code, "kena_q_") {
		t.Fatalf("CreateLoginCode = %q, %v", code, err)
	}
	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM login_codes WHERE hash = $1`, []byte(code)).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("code stored in clear: %d, %v", stored, err)
	}
	if got, err := s.PeekLoginCode(ctx, code); err != nil || got.ID != u.ID {
		t.Fatalf("PeekLoginCode = %+v, %v", got, err)
	}
	if got, err := s.UseLoginCode(ctx, code); err != nil || got.ID != u.ID {
		t.Fatalf("UseLoginCode = %+v, %v", got, err)
	}
	for _, f := range []func(context.Context, string) (store.User, error){s.PeekLoginCode, s.UseLoginCode} {
		if _, err := f(ctx, code); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("used code still works: %v", err)
		}
	}
	if _, err := s.UseLoginCode(ctx, "kena_q_wrong"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("wrong code: %v", err)
	}
	if _, err := s.UseLoginCode(ctx, ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("empty code: %v", err)
	}

	expired, err := s.CreateLoginCode(ctx, u.ID, -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UseLoginCode(ctx, expired); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expired code: %v", err)
	}

	first, _ := s.CreateLoginCode(ctx, u.ID, 5*time.Minute)
	second, _ := s.CreateLoginCode(ctx, u.ID, 5*time.Minute)
	if _, err := s.UseLoginCode(ctx, first); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a new code should replace the unused old one: %v", err)
	}
	if _, err := s.UseLoginCode(ctx, second); err != nil {
		t.Errorf("newest code: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE login_codes SET expires_at = now() - interval '2 days', used_at = NULL`); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM login_codes`).Scan(&left); err != nil || left != 0 {
		t.Errorf("purge left %d codes, %v", left, err)
	}
}

func TestConcurrentLoginCodesLeaveOne(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	s := store.New(pool)
	u, err := s.CreateUser(ctx, "anna", false)
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 20; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.CreateLoginCode(ctx, u.ID, 5*time.Minute); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		var valid int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM login_codes WHERE user_id = $1 AND used_at IS NULL AND expires_at > now()`, u.ID).Scan(&valid); err != nil {
			t.Fatal(err)
		}
		if valid != 1 {
			t.Fatalf("round %d: %d valid codes after concurrent creates, want 1", round, valid)
		}
	}
}
