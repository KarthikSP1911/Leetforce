package store

import (
	"context"
	"testing"
)

const (
	rjA = "aaaaaaaa-0000-4000-8000-000000000001"
	rjB = "aaaaaaaa-0000-4000-8000-000000000002"
	rjC = "aaaaaaaa-0000-4000-8000-000000000003"
)

func rejudgeFixture(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.SyncProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	return s, ctx
}

func (s *Store) submissionState(t *testing.T, id string) (status, version string, enqueued bool, verdict, verdictVersion string) {
	t.Helper()
	err := s.pool.QueryRow(context.Background(), `
		SELECT s.status, s.test_set_version, s.enqueued_at IS NOT NULL, COALESCE(v.verdict, ''), COALESCE(v.test_set_version, '')
		FROM submissions s LEFT JOIN verdicts v ON v.submission_id = s.id WHERE s.id = $1::uuid`, id).
		Scan(&status, &version, &enqueued, &verdict, &verdictVersion)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestSyncProblemReportsVersionChange(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}
	for _, tc := range []struct {
		version string
		want    ProblemChange
		changed bool
	}{
		{"v1", ProblemChange{Slug: "sum", Created: true, New: "v1"}, false},
		{"v1", ProblemChange{Slug: "sum", Old: "v1", New: "v1"}, false},
		{"v2", ProblemChange{Slug: "sum", Old: "v1", New: "v2"}, true},
	} {
		got, err := s.SyncProblem(ctx, p, tc.version)
		if err != nil || got != tc.want || got.VersionChanged() != tc.changed {
			t.Fatalf("SyncProblem(%s) = %+v, %v; want %+v changed=%v", tc.version, got, err, tc.want, tc.changed)
		}
	}
	if v, err := s.TestSetVersion(ctx, "sum"); err != nil || v != "v2" {
		t.Fatalf("stored version = %q, %v, want v2", v, err)
	}
}

// Exit criterion: fixing a test set (a new version) puts the affected
// submissions back in the queue against the new version, and their new
// verdicts replace the old ones exactly once.
func TestRejudgeAfterVersionChange(t *testing.T) {
	s, ctx := rejudgeFixture(t)
	for _, id := range []string{rjA, rjB} {
		if _, err := s.InsertSubmission(ctx, id, "sum", "python", "print(1)"); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkEnqueued(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	// A is judged at v1, B is still queued at v1.
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: rjA, Verdict: "AC", TestSetVersion: "v1", Passed: 3, Total: 3, RunnerID: "r1"}); err != nil || !ok {
		t.Fatalf("record A = %v, %v", ok, err)
	}
	if n, err := s.PendingRejudge(ctx, "sum"); err != nil || n != 0 {
		t.Fatalf("pending before the change = %d, %v, want 0", n, err)
	}

	if _, err := s.SyncProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v2"); err != nil {
		t.Fatal(err)
	}
	// C is accepted after the fix, so it is already on v2 and untouched.
	if _, err := s.InsertSubmission(ctx, rjC, "sum", "python", "print(1)"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: rjC, Verdict: "WA", TestSetVersion: "v2", RunnerID: "r1"}); err != nil || !ok {
		t.Fatalf("record C = %v, %v", ok, err)
	}
	if n, err := s.PendingRejudge(ctx, "sum"); err != nil || n != 2 {
		t.Fatalf("pending after the change = %d, %v, want 2 (A judged at v1, B queued at v1)", n, err)
	}

	// Batches of one: each call takes a different submission, the third none.
	first, err := s.BeginRejudge(ctx, "sum", 1)
	if err != nil || len(first) != 1 || first[0].ID != rjA || first[0].TestSetVersion != "v2" || first[0].Source != "print(1)" || first[0].Language != "python" {
		t.Fatalf("first batch = %+v, %v", first, err)
	}
	second, err := s.BeginRejudge(ctx, "sum", 10)
	if err != nil || len(second) != 1 || second[0].ID != rjB {
		t.Fatalf("second batch = %+v, %v", second, err)
	}
	if third, err := s.BeginRejudge(ctx, "sum", 10); err != nil || len(third) != 0 {
		t.Fatalf("third batch = %+v, %v, want none (idempotent)", third, err)
	}
	if st, ver, enq, v, vv := s.submissionState(t, rjA); st != StatusQueued || ver != "v2" || enq || v != "AC" || vv != "v1" {
		t.Fatalf("A after BeginRejudge: status %s version %s enqueued %v verdict %s@%s", st, ver, enq, v, vv)
	}
	if st, ver, _, _, _ := s.submissionState(t, rjC); st != StatusJudged || ver != "v2" {
		t.Fatalf("C was touched: status %s version %s", st, ver)
	}

	// The reaper's mark: after the enqueue succeeds the row is marked.
	if err := s.MarkEnqueued(ctx, rjA); err != nil {
		t.Fatal(err)
	}

	// The late verdict of the old job (v1) is dropped; B had no verdict yet.
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: rjB, Verdict: "AC", TestSetVersion: "v1"}); err != nil || ok {
		t.Fatalf("stale v1 verdict for B = %v, %v, want dropped", ok, err)
	}
	if st, _, _, v, _ := s.submissionState(t, rjB); st != StatusQueued || v != "" {
		t.Fatalf("B after a stale verdict: status %s verdict %q, want queued and none", st, v)
	}

	// The rejudge verdict replaces the old one, once.
	newV := VerdictRecord{SubmissionID: rjA, Verdict: "WA", TestSetVersion: "v2", Passed: 1, Total: 3, RunnerID: "r2"}
	if ok, err := s.RecordVerdict(ctx, newV); err != nil || !ok {
		t.Fatalf("rejudge verdict for A = %v, %v, want stored", ok, err)
	}
	st, ver, _, v, vv := s.submissionState(t, rjA)
	if st != StatusJudged || ver != "v2" || v != "WA" || vv != "v2" {
		t.Fatalf("A after rejudge: status %s version %s verdict %s@%s", st, ver, v, vv)
	}
	for _, dup := range []VerdictRecord{
		newV, // same delivery again
		{SubmissionID: rjA, Verdict: "AC", TestSetVersion: "v2", RunnerID: "r3"}, // conflicting, same version
		{SubmissionID: rjA, Verdict: "AC", TestSetVersion: "v1", RunnerID: "r1"}, // the old verdict arrives late
	} {
		if ok, err := s.RecordVerdict(ctx, dup); err != nil || ok {
			t.Fatalf("duplicate %s@%s = %v, %v, want no change", dup.Verdict, dup.TestSetVersion, ok, err)
		}
	}
	if st2, ver2, _, v2, vv2 := s.submissionState(t, rjA); st2 != st || ver2 != ver || v2 != v || vv2 != vv {
		t.Fatalf("duplicates changed A: %s %s %s@%s", st2, ver2, v2, vv2)
	}
	if n, err := s.PendingRejudge(ctx, "sum"); err != nil || n != 0 {
		t.Fatalf("pending at the end = %d, %v, want 0 (B waits for its v2 verdict but is already on v2)", n, err)
	}
}

// An internal error from a failed rejudge must not wipe out a real verdict,
// and must not leave the submission waiting for a job that will never finish.
func TestRejudgeIEKeepsPreviousVerdict(t *testing.T) {
	s, ctx := rejudgeFixture(t)
	if _, err := s.InsertSubmission(ctx, rjA, "sum", "python", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: rjA, Verdict: "AC", TestSetVersion: "v1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v2"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.BeginRejudge(ctx, "sum", 10); err != nil || len(got) != 1 {
		t.Fatalf("BeginRejudge = %+v, %v", got, err)
	}
	for _, ie := range []VerdictRecord{
		{SubmissionID: rjA, Verdict: "IE", RunnerID: "dead-letter"},
		{SubmissionID: rjA, Verdict: "IE", TestSetVersion: "v2", RunnerID: "r1"},
	} {
		if ok, err := s.RecordVerdict(ctx, ie); err != nil || ok {
			t.Fatalf("IE %+v = %v, %v, want not stored", ie, ok, err)
		}
	}
	st, _, _, v, vv := s.submissionState(t, rjA)
	if st != StatusJudged || v != "AC" || vv != "v1" {
		t.Fatalf("after a failed rejudge: status %s verdict %s@%s, want judged AC@v1", st, v, vv)
	}
}

// A submission accepted against a version that is replaced while its job is in
// flight is rejudged, and the in-flight verdict does not stick.
func TestRejudgeInFlightSubmission(t *testing.T) {
	s, ctx := rejudgeFixture(t)
	if _, err := s.InsertSubmission(ctx, rjA, "sum", "python", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkJudging(ctx, rjA); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.BeginRejudge(ctx, "sum", 10)
	if err != nil || len(got) != 1 || got[0].ID != rjA {
		t.Fatalf("BeginRejudge = %+v, %v", got, err)
	}
	if st, ver, _, _, _ := s.submissionState(t, rjA); st != StatusQueued || ver != "v2" {
		t.Fatalf("status %s version %s, want queued v2", st, ver)
	}
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: rjA, Verdict: "AC", TestSetVersion: "v1"}); err != nil || ok {
		t.Fatalf("in-flight v1 verdict = %v, %v, want dropped", ok, err)
	}
	if ok, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: rjA, Verdict: "WA", TestSetVersion: "v2"}); err != nil || !ok {
		t.Fatalf("v2 verdict = %v, %v, want stored", ok, err)
	}
}
