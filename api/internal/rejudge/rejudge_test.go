package rejudge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeStore hands out `stale` submissions limit at a time, like BeginRejudge
// does once a row has moved onto the new version.
type fakeStore struct {
	stale     []store.Rejudgeable
	marked    []string
	beginErr  error
	markErr   error
	beginCall int
}

func (f *fakeStore) BeginRejudge(_ context.Context, _ string, limit int) ([]store.Rejudgeable, error) {
	f.beginCall++
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	n := min(limit, len(f.stale))
	out := f.stale[:n]
	f.stale = f.stale[n:]
	return out, nil
}

func (f *fakeStore) MarkEnqueued(_ context.Context, id string) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.marked = append(f.marked, id)
	return nil
}

type fakeQueue struct {
	jobs    []queue.Job
	failFor map[string]bool
}

func (f *fakeQueue) Enqueue(_ context.Context, j queue.Job) (string, error) {
	if f.failFor[j.SubmissionID] {
		return "", errors.New("redis down")
	}
	f.jobs = append(f.jobs, j)
	return "1-0", nil
}

func stale(n int) []store.Rejudgeable {
	var out []store.Rejudgeable
	for i := 0; i < n; i++ {
		out = append(out, store.Rejudgeable{ID: fmt.Sprintf("s%d", i), Problem: "sum", Language: "python", Source: "src", TestSetVersion: "v2"})
	}
	return out
}

func TestRunEnqueuesEveryBatchWithTheNewVersion(t *testing.T) {
	st := &fakeStore{stale: stale(5)}
	q := &fakeQueue{}
	res, err := New(st, q, quiet, 2).Run(context.Background(), "sum")
	if err != nil || res.Requeued != 5 || res.Deferred != 0 {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if len(q.jobs) != 5 || len(st.marked) != 5 {
		t.Fatalf("jobs %d, marked %d, want 5 each", len(q.jobs), len(st.marked))
	}
	for _, j := range q.jobs {
		if j.TestSetVersion != "v2" || j.Problem != "sum" || j.Source != "src" || j.Language != "python" || j.Kind != "" {
			t.Fatalf("job = %+v", j)
		}
	}
	// 5 rows in batches of 2: 2, 2, 1 (short batch ends the loop).
	if st.beginCall != 3 {
		t.Fatalf("BeginRejudge called %d times, want 3", st.beginCall)
	}
	// A second run finds nothing and does not enqueue again.
	res, err = New(st, q, quiet, 2).Run(context.Background(), "sum")
	if err != nil || res.Requeued != 0 || len(q.jobs) != 5 {
		t.Fatalf("second Run = %+v, %v, jobs %d", res, err, len(q.jobs))
	}
}

func TestRunDefersEnqueueFailuresWithoutMarking(t *testing.T) {
	st := &fakeStore{stale: stale(3)}
	q := &fakeQueue{failFor: map[string]bool{"s1": true}}
	res, err := New(st, q, quiet, 10).Run(context.Background(), "sum")
	if err == nil || res.Requeued != 2 || res.Deferred != 1 {
		t.Fatalf("Run = %+v, %v, want 2 requeued, 1 deferred and an error", res, err)
	}
	for _, id := range st.marked {
		if id == "s1" {
			t.Fatal("a row whose enqueue failed was marked enqueued; the reaper would never retry it")
		}
	}
}

func TestRunStopsOnDatabaseError(t *testing.T) {
	boom := errors.New("neon asleep")
	_, err := New(&fakeStore{beginErr: boom}, &fakeQueue{}, quiet, 10).Run(context.Background(), "sum")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the database error", err)
	}
}

func TestRunChangedOnlyTouchesChangedProblems(t *testing.T) {
	st := &fakeStore{stale: stale(2)}
	q := &fakeQueue{}
	err := New(st, q, quiet, 10).RunChanged(context.Background(), []store.ProblemChange{
		{Slug: "new", Created: true, New: "v1"},
		{Slug: "same", Old: "v1", New: "v1"},
		{Slug: "sum", Old: "v1", New: "v2"},
	})
	if err != nil || len(q.jobs) != 2 || st.beginCall != 1 {
		t.Fatalf("RunChanged: err %v, jobs %d, BeginRejudge calls %d, want nil, 2, 1", err, len(q.jobs), st.beginCall)
	}
}
