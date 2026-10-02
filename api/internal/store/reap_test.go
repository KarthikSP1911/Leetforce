package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReapUnqueued(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "ts-0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	const (
		orphan   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" // stored, never queued, old
		failing  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" // stored, never queued, old, enqueue will fail
		young    = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" // stored, never queued, but still within the grace period
		queued   = "dddddddd-dddd-4ddd-8ddd-dddddddddddd" // old, job already queued
		verdicts = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee" // old, never queued, but already judged
	)
	for _, id := range []string{orphan, failing, young, queued, verdicts} {
		if _, err := s.InsertSubmission(ctx, id, "sum", "python", "src-"+id[:1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{orphan, failing, queued, verdicts} {
		if _, err := s.pool.Exec(ctx, `UPDATE submissions SET created_at = now() - interval '10 minutes' WHERE id = $1::uuid`, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkEnqueued(ctx, queued); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVerdict(ctx, VerdictRecord{SubmissionID: verdicts, Verdict: "AC"}); err != nil {
		t.Fatal(err)
	}

	var seen []Unqueued
	n, err := s.ReapUnqueued(ctx, 2*time.Minute, 10, func(_ context.Context, u Unqueued) error {
		seen = append(seen, u)
		if u.ID == failing {
			return errors.New("redis down")
		}
		return nil
	})
	if err == nil {
		t.Fatal("a failed enqueue must be reported")
	}
	if n != 1 {
		t.Fatalf("queued %d, want 1 (only the orphan; the failing one stays for next time)", n)
	}
	if len(seen) != 2 {
		t.Fatalf("enqueue called for %+v, want exactly the orphan and the failing row", seen)
	}
	for _, u := range seen {
		if u.TestSetVersion != "ts-0123456789abcdef" || u.Problem != "sum" || u.Language != "python" || u.Source == "" {
			t.Errorf("job data incomplete: %+v", u)
		}
	}

	// The orphan is marked now; the failing row is still waiting; the rest were never candidates.
	seen = nil
	n, err = s.ReapUnqueued(ctx, 2*time.Minute, 10, func(_ context.Context, u Unqueued) error {
		seen = append(seen, u)
		return nil
	})
	if err != nil || n != 1 || len(seen) != 1 || seen[0].ID != failing {
		t.Fatalf("second sweep = %d, %v, %+v; want only the previously failing row", n, err, seen)
	}
	n, err = s.ReapUnqueued(ctx, 2*time.Minute, 10, func(context.Context, Unqueued) error { return errors.New("must not be called") })
	if err != nil || n != 0 {
		t.Fatalf("third sweep = %d, %v; want nothing to do", n, err)
	}

	// The young row is picked up once it is older than the grace period.
	n, err = s.ReapUnqueued(ctx, 0, 10, func(_ context.Context, u Unqueued) error {
		if u.ID != young {
			t.Errorf("unexpected row %s", u.ID)
		}
		return nil
	})
	if err != nil || n != 1 {
		t.Fatalf("sweep with no grace = %d, %v; want the young row", n, err)
	}
}

// One sweep takes at most `limit` rows; the rest wait for the next sweep.
func TestReapUnqueuedRespectsLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.UpsertProblem(ctx, Problem{Slug: "sum", Title: "Sum", Difficulty: "easy"}, "v1"); err != nil {
		t.Fatal(err)
	}
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	for _, id := range ids {
		if _, err := s.InsertSubmission(ctx, id, "sum", "go", "x"); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.ReapUnqueued(ctx, 0, 2, func(context.Context, Unqueued) error { return nil })
	if err != nil || n != 2 {
		t.Fatalf("first sweep = %d, %v; want 2 (the limit)", n, err)
	}
	n, err = s.ReapUnqueued(ctx, 0, 2, func(context.Context, Unqueued) error { return nil })
	if err != nil || n != 1 {
		t.Fatalf("second sweep = %d, %v; want the remaining 1", n, err)
	}
}
