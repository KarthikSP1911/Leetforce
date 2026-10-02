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

	if _, err := s.InsertSubmission(ctx, id, "missing", "python", "print(1)"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("insert for an unknown problem err = %v, want ErrNotFound", err)
	}
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	ver, err := s.InsertSubmission(ctx, id, "sum", "python", "print(1)")
	if err != nil {
		t.Fatal(err)
	}
	if ver != "v1" {
		t.Fatalf("InsertSubmission returned version %q, want v1 (the job names the version it was accepted against)", ver)
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
	if _, err := s.InsertSubmission(ctx, id, "sum", "python", "dup"); err == nil {
		t.Fatal("inserting the same id twice must fail")
	}
	if _, err := s.InsertSubmission(ctx, "22222222-2222-4222-8222-222222222222", "sum", "rust", "x"); err == nil {
		t.Fatal("a language outside the judge's four must be rejected by the schema")
	}
	if err := s.DeleteSubmission(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSubmission(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}

// MarkJudging moves a queued submission forward once and never undoes a verdict.
func TestMarkJudging(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const id = "33333333-3333-4333-8333-333333333333"
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertSubmission(ctx, id, "sum", "python", "print(1)"); err != nil {
		t.Fatal(err)
	}

	if changed, err := s.MarkJudging(ctx, id); err != nil || !changed {
		t.Fatalf("first MarkJudging = %v, %v; want true", changed, err)
	}
	if got, _ := s.GetSubmission(ctx, id); got.Status != StatusJudging {
		t.Fatalf("status = %q, want judging", got.Status)
	}
	if changed, err := s.MarkJudging(ctx, id); err != nil || changed {
		t.Fatalf("repeated MarkJudging = %v, %v; want false (already judging)", changed, err)
	}

	if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: id, Verdict: "AC"}); err != nil {
		t.Fatal(err)
	}
	// A late "judging" event after the verdict must not move the submission back.
	if changed, err := s.MarkJudging(ctx, id); err != nil || changed {
		t.Fatalf("MarkJudging after the verdict = %v, %v; want false", changed, err)
	}
	if got, _ := s.GetSubmission(ctx, id); got.Status != StatusJudged || got.Verdict == nil {
		t.Fatalf("after a late judging event: %+v, want judged with its verdict", got)
	}
	if changed, err := s.MarkJudging(ctx, "44444444-4444-4444-8444-444444444444"); err != nil || changed {
		t.Fatalf("MarkJudging of an unknown submission = %v, %v; want false, nil", changed, err)
	}
	if _, err := s.MarkJudging(ctx, "not-a-uuid"); !IsPermanent(err) {
		t.Fatalf("MarkJudging with a malformed id err = %v, want a permanent error", err)
	}
}
