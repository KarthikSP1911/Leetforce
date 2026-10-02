package store

import (
	"context"
	"errors"
	"testing"
)

func TestSubmissions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const id = "11111111-1111-4111-8111-111111111111"

	if err := s.InsertSubmission(ctx, id, "missing", "python", "print(1)"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("insert for an unknown problem err = %v, want ErrNotFound", err)
	}
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSubmission(ctx, id, "sum", "python", "print(1)"); err != nil {
		t.Fatal(err)
	}
	// A later test fix changes the problem's version; the submission keeps the one it was accepted against.
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSubmission(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusQueued || got.Problem != "sum" || got.Language != "python" || got.Verdict != nil {
		t.Fatalf("submission = %+v", got)
	}
	if got.TestSetVersion != "v1" {
		t.Fatalf("test-set version = %q, want v1 (the version at accept time)", got.TestSetVersion)
	}

	if _, err := s.GetSubmission(ctx, "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get with a malformed id err = %v, want ErrNotFound", err)
	}
	if err := s.InsertSubmission(ctx, id, "sum", "python", "dup"); err == nil {
		t.Fatal("inserting the same id twice must fail")
	}
	if err := s.InsertSubmission(ctx, "22222222-2222-4222-8222-222222222222", "sum", "rust", "x"); err == nil {
		t.Fatal("a language outside the judge's four must be rejected by the schema")
	}
	if err := s.DeleteSubmission(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSubmission(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}
