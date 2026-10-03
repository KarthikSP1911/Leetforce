package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestProblems(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, err := s.GetProblem(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProblem(missing) err = %v, want ErrNotFound", err)
	}
	b := Problem{Slug: "b-two", Title: "B", Difficulty: "hard"}
	a := Problem{Slug: "a-one", Title: "A", Difficulty: "easy", Tags: []string{"math"}}
	for _, p := range []Problem{b, a} {
		if err := s.UpsertProblem(ctx, p, "v1"); err != nil {
			t.Fatal(err)
		}
	}
	// Upserting again changes the title, not the number of rows.
	a.Title = "A renamed"
	if err := s.UpsertProblem(ctx, a, "v2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListProblems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Problem{a, {Slug: "b-two", Title: "B", Difficulty: "hard", Tags: []string{}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListProblems = %+v, want %+v", got, want)
	}
	one, err := s.GetProblem(ctx, "a-one")
	if err != nil || one.Title != "A renamed" {
		t.Fatalf("GetProblem = %+v, %v", one, err)
	}
	if err := s.UpsertProblem(ctx, Problem{Slug: "x", Title: "X", Difficulty: "bogus"}, "v1"); err == nil {
		t.Fatal("difficulty outside easy/medium/hard must be rejected by the schema")
	}
}

func TestProblemAcceptance(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.UpsertProblem(ctx, Problem{Slug: "acc", Title: "Acc", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetProblem(ctx, "acc")
	if err != nil || p.Acceptance != nil {
		t.Fatalf("no submissions: acceptance = %v, err %v (want nil)", p.Acceptance, err)
	}
	ids := []string{
		"22222222-2222-4222-8222-222222222221", "22222222-2222-4222-8222-222222222222",
		"22222222-2222-4222-8222-222222222223", "22222222-2222-4222-8222-222222222224",
	}
	for i, verdict := range []string{"AC", "WA", "AC", "IE"} {
		if _, err := s.InsertSubmission(ctx, ids[i], "acc", "python", "x"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: ids[i], Verdict: verdict, TestSetVersion: "v1"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListProblems(ctx)
	if err != nil || len(got) != 1 || got[0].Acceptance == nil {
		t.Fatalf("ListProblems = %+v, %v", got, err)
	}
	// 2 AC of 3 counted verdicts (the IE is excluded).
	if a := *got[0].Acceptance; a < 66.6 || a > 66.7 {
		t.Fatalf("acceptance = %v, want about 66.67", a)
	}
}

// TestSetVersion returns the version a submission would be stamped with, and
// ErrNotFound for an unknown problem; Run jobs use it to fetch the same bundle.
func TestTestSetVersion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.TestSetVersion(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown problem err = %v, want ErrNotFound", err)
	}
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.TestSetVersion(ctx, "sum"); err != nil || v != "v1" {
		t.Fatalf("version = %q, %v, want v1", v, err)
	}
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.TestSetVersion(ctx, "sum"); v != "v2" {
		t.Fatalf("version after a test fix = %q, want v2", v)
	}
}
