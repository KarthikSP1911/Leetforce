package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUsersAndSessions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u := User{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: "Ada@Example.com", Username: "Ada", PasswordHash: "hash"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	// Email and username are unique case-insensitively.
	dup := User{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Email: "ada@example.com", Username: "other", PasswordHash: "x"}
	if err := s.CreateUser(ctx, dup); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email err = %v, want ErrConflict", err)
	}
	dup.Email, dup.Username = "new@example.com", "ADA"
	if err := s.CreateUser(ctx, dup); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate username err = %v, want ErrConflict", err)
	}

	for _, login := range []string{"ada@example.com", "ADA"} {
		got, err := s.UserByLogin(ctx, login)
		if err != nil || got.ID != u.ID || got.PasswordHash != "hash" {
			t.Fatalf("UserByLogin(%q) = %+v, %v", login, got, err)
		}
	}
	if _, err := s.UserByLogin(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown login err = %v, want ErrNotFound", err)
	}

	live, dead := []byte("live-token-hash-0123456789abcdef"), []byte("dead-token-hash-0123456789abcdef")
	if err := s.CreateSession(ctx, live, u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, dead, u.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SessionUser(ctx, live); err != nil || got.ID != u.ID || got.PasswordHash != "" {
		t.Fatalf("live session = %+v, %v", got, err)
	}
	if _, err := s.SessionUser(ctx, dead); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session err = %v, want ErrNotFound", err)
	}
	if n, err := s.DeleteExpiredSessions(ctx); err != nil || n != 1 {
		t.Fatalf("DeleteExpiredSessions = %d, %v; want 1", n, err)
	}
	if err := s.DeleteSession(ctx, live); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, live); !errors.Is(err, ErrNotFound) {
		t.Fatalf("logged-out session err = %v, want ErrNotFound", err)
	}
}
