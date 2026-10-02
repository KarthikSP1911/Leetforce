package store

import (
	"context"
	"testing"
)

// Exit criterion: duplicate verdict posts do not change state.
func TestRecordVerdictIsIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const id = "11111111-1111-4111-8111-111111111111"
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSubmission(ctx, id, "sum", "python", "print(1)"); err != nil {
		t.Fatal(err)
	}

	first := VerdictRecord{SubmissionID: id, Verdict: "AC", RuntimeMS: 12, MemoryKB: 900, Passed: 5, Total: 5, TestSetVersion: "v1", RunnerID: "runner-a"}
	ok, err := s.RecordVerdict(ctx, first)
	if err != nil || !ok {
		t.Fatalf("first record = %v, %v", ok, err)
	}
	want, err := s.GetSubmission(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if want.Status != StatusJudged || want.Verdict == nil || want.Verdict.Verdict != "AC" || want.Verdict.RuntimeMS != 12 {
		t.Fatalf("after first record: %+v", want)
	}

	// The same verdict again, then a conflicting one: nothing changes.
	for _, dup := range []VerdictRecord{
		first,
		{SubmissionID: id, Verdict: "WA", RuntimeMS: 99, MemoryKB: 1, Passed: 1, Total: 5, TestSetVersion: "v2", RunnerID: "runner-b"},
		{SubmissionID: id, Verdict: "IE"},
	} {
		ok, err := s.RecordVerdict(ctx, dup)
		if err != nil || ok {
			t.Fatalf("duplicate %s record = %v, %v (want false, nil)", dup.Verdict, ok, err)
		}
	}
	got, err := s.GetSubmission(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict == nil || *got.Verdict != *want.Verdict || got.Status != want.Status || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("duplicates changed state:\n before %+v %+v\n after  %+v %+v", want, *want.Verdict, got, got.Verdict)
	}
	var n int
	var runner, version string
	if err := s.pool.QueryRow(ctx, `SELECT count(*), max(runner_id), max(test_set_version) FROM verdicts WHERE submission_id = $1::uuid`, id).Scan(&n, &runner, &version); err != nil {
		t.Fatal(err)
	}
	if n != 1 || runner != "runner-a" || version != "v1" {
		t.Fatalf("verdict row = count %d runner %q version %q, want 1, runner-a, v1", n, runner, version)
	}
}

func TestRecordVerdictEdgeCases(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	const id = "11111111-1111-4111-8111-111111111111"
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v7"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSubmission(ctx, id, "sum", "go", "x"); err != nil {
		t.Fatal(err)
	}

	// Unknown submission: stores nothing, not an error.
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: "99999999-9999-4999-8999-999999999999", Verdict: "AC"}); err != nil || ok {
		t.Fatalf("unknown submission = %v, %v", ok, err)
	}
	// Invalid data is a permanent error; a retry could never work.
	if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: "not-a-uuid", Verdict: "AC"}); !IsPermanent(err) {
		t.Fatalf("bad uuid err = %v, want a permanent error", err)
	}
	if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: id, Verdict: "ZZ"}); !IsPermanent(err) {
		t.Fatalf("bad verdict err = %v, want a permanent error", err)
	}
	if got, _ := s.GetSubmission(ctx, id); got.Status != StatusQueued || got.Verdict != nil {
		t.Fatalf("rejected writes changed the submission: %+v", got)
	}
	// An internal error has no test-set version of its own: the submission's version is used.
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: id, Verdict: "IE", RunnerID: "dead-letter"}); err != nil || !ok {
		t.Fatalf("IE record = %v, %v", ok, err)
	}
	var version string
	if err := s.pool.QueryRow(ctx, `SELECT test_set_version FROM verdicts WHERE submission_id = $1::uuid`, id).Scan(&version); err != nil || version != "v7" {
		t.Fatalf("IE test-set version = %q, %v, want v7", version, err)
	}
	if IsPermanent(context.DeadlineExceeded) {
		t.Fatal("a timeout is not a permanent error")
	}
}
