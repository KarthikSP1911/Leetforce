package reaper

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

type fakeStore struct {
	mu    sync.Mutex
	rows  []store.Unqueued
	sweep int
	got   struct {
		grace time.Duration
		limit int
	}
}

func (f *fakeStore) ReapUnqueued(ctx context.Context, grace time.Duration, limit int, enqueue func(context.Context, store.Unqueued) error) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sweep++
	f.got.grace, f.got.limit = grace, limit
	var errs []error
	n := 0
	for _, u := range f.rows {
		if err := enqueue(ctx, u); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

type fakeQ struct {
	mu   sync.Mutex
	jobs []queue.Job
	err  error
}

func (f *fakeQ) Enqueue(_ context.Context, j queue.Job) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.jobs = append(f.jobs, j)
	return "1-0", nil
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSweepRequeuesWithTheStoredVersion(t *testing.T) {
	st := &fakeStore{rows: []store.Unqueued{{ID: "a", Problem: "sum", Language: "go", Source: "src", TestSetVersion: "ts-0123456789abcdef"}}}
	q := &fakeQ{}
	r := New(st, q, quiet(), Config{Grace: 3 * time.Minute, Batch: 7})
	if n := r.Sweep(context.Background()); n != 1 {
		t.Fatalf("Sweep = %d, want 1", n)
	}
	want := queue.Job{SubmissionID: "a", Problem: "sum", Language: "go", Source: "src", TestSetVersion: "ts-0123456789abcdef"}
	if len(q.jobs) != 1 || q.jobs[0] != want {
		t.Fatalf("jobs = %+v, want [%+v]", q.jobs, want)
	}
	if st.got.grace != 3*time.Minute || st.got.limit != 7 {
		t.Errorf("store called with grace %v limit %d", st.got.grace, st.got.limit)
	}
}

func TestSweepSurvivesQueueFailure(t *testing.T) {
	st := &fakeStore{rows: []store.Unqueued{{ID: "a"}, {ID: "b"}}}
	q := &fakeQ{err: errors.New("redis down")}
	if n := New(st, q, quiet(), Config{}).Sweep(context.Background()); n != 0 {
		t.Fatalf("Sweep = %d, want 0 when the queue is down", n)
	}
}

func TestRunSweepsAtStartAndThenOnTheInterval(t *testing.T) {
	st := &fakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { New(st, &fakeQ{}, quiet(), Config{Every: 20 * time.Millisecond}).Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st.mu.Lock()
		n := st.sweep
		st.mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.sweep < 3 {
		t.Fatalf("only %d sweeps, want the startup sweep plus interval sweeps", st.sweep)
	}
}

func TestDefaults(t *testing.T) {
	r := New(&fakeStore{}, &fakeQ{}, quiet(), Config{})
	if r.cfg.Every != 15*time.Minute || r.cfg.Grace != 2*time.Minute || r.cfg.Batch != 50 {
		t.Fatalf("defaults = %+v", r.cfg)
	}
}
