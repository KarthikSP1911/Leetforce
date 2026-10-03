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

// ListSubmissions returns only the asking client's submissions to the problem,
// newest first, with verdicts, and never another client's.
func TestListSubmissions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, slug := range []string{"sum", "other"} {
		if err := s.UpsertProblem(ctx, Problem{Slug: slug, Title: slug, Difficulty: "easy"}, "v1"); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id, problem, client string) {
		t.Helper()
		if _, err := s.InsertSubmission(ctx, id, problem, "python", "x"); err != nil {
			t.Fatal(err)
		}
		if client != "" {
			if err := s.SetSubmissionClient(ctx, id, client); err != nil {
				t.Fatal(err)
			}
		}
	}
	const a, b, c, d = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"
	add(a, "sum", "client-one")
	add(b, "sum", "client-one")
	add(c, "sum", "client-two")
	add(d, "other", "client-one")
	if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: a, Verdict: "AC", RuntimeMS: 3, MemoryKB: 100, Passed: 2, Total: 2}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListSubmissions(ctx, "sum", "client-one", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != b || got[1].ID != a {
		t.Fatalf("list = %+v, want [b a] newest first", got)
	}
	if got[0].Verdict != nil || got[1].Verdict == nil || got[1].Verdict.Verdict != "AC" {
		t.Fatalf("verdicts = %+v / %+v", got[0].Verdict, got[1].Verdict)
	}
	if got, _ := s.ListSubmissions(ctx, "sum", "client-one", 1); len(got) != 1 {
		t.Fatalf("limit not applied: %d rows", len(got))
	}
	if got, err := s.ListSubmissions(ctx, "sum", "nobody", 10); err != nil || len(got) != 0 {
		t.Fatalf("unknown client = %+v, %v", got, err)
	}
}
