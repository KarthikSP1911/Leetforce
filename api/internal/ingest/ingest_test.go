package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"leetforce/api/internal/store"
	"leetforce/queue"
)

const sid = "11111111-1111-4111-8111-111111111111"

type fakeRec struct {
	mu       sync.Mutex
	recorded []store.VerdictRecord
	result   bool
	err      error
}

func (f *fakeRec) RecordVerdict(_ context.Context, v store.VerdictRecord) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, v)
	return f.result, f.err
}

type fakeSrc struct {
	mu      sync.Mutex
	results []*queue.ResultDelivery
	dead    []*queue.DeadDelivery
	acked   []string
}

func (f *fakeSrc) ReceiveResult(ctx context.Context, _ string, block time.Duration) (*queue.ResultDelivery, error) {
	f.mu.Lock()
	if len(f.results) > 0 {
		d := f.results[0]
		f.results = f.results[1:]
		f.mu.Unlock()
		return d, nil
	}
	f.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-time.After(block):
	}
	return nil, nil
}

func (f *fakeSrc) ReceiveDead(context.Context, string, time.Duration) (*queue.DeadDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.dead) == 0 {
		return nil, nil
	}
	d := f.dead[0]
	f.dead = f.dead[1:]
	return d, nil
}

func (f *fakeSrc) ack(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = append(f.acked, id)
	return nil
}
func (f *fakeSrc) AckResult(_ context.Context, id string) error { return f.ack(id) }
func (f *fakeSrc) AckDead(_ context.Context, id string) error   { return f.ack(id) }

func newIngester(src *fakeSrc, rec *fakeRec) *Ingester {
	return New(src, rec, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{Consumer: "api-test", Block: 10 * time.Millisecond, DeadEvery: time.Millisecond})
}

func TestHandleResult(t *testing.T) {
	good := queue.Result{SubmissionID: sid, Verdict: "AC", RuntimeMS: 7, MemoryKB: 512, Passed: 5, Total: 5, TestSetVersion: "v1", RunnerID: "r1"}
	tests := []struct {
		name       string
		d          *queue.ResultDelivery
		rec        *fakeRec
		wantAcked  bool
		wantRecord bool
	}{
		{"stored and acknowledged", &queue.ResultDelivery{ID: "1-0", Result: good}, &fakeRec{result: true}, true, true},
		{"duplicate verdict is acknowledged, not an error", &queue.ResultDelivery{ID: "1-0", Result: good}, &fakeRec{result: false}, true, true},
		{"transient database error stays pending", &queue.ResultDelivery{ID: "1-0", Result: good}, &fakeRec{err: errors.New("connection reset")}, false, true},
		{"permanent database error is dropped", &queue.ResultDelivery{ID: "1-0", Result: good}, &fakeRec{err: &pgconn.PgError{Code: "23514"}}, true, true},
		{"undecodable entry is dropped", &queue.ResultDelivery{ID: "1-0", Malformed: true}, &fakeRec{}, true, false},
		{"invalid submission id is dropped before the database", &queue.ResultDelivery{ID: "1-0", Result: queue.Result{SubmissionID: "../x", Verdict: "AC"}}, &fakeRec{}, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeSrc{}
			newIngester(src, tc.rec).HandleResult(context.Background(), tc.d)
			if got := len(src.acked) == 1; got != tc.wantAcked {
				t.Fatalf("acked = %v, want %v", src.acked, tc.wantAcked)
			}
			if got := len(tc.rec.recorded) == 1; got != tc.wantRecord {
				t.Fatalf("recorded = %+v, want a write: %v", tc.rec.recorded, tc.wantRecord)
			}
		})
	}
}

func TestHandleResultMapsFields(t *testing.T) {
	rec := &fakeRec{result: true}
	r := queue.Result{SubmissionID: sid, Verdict: "TLE", RuntimeMS: 1000, MemoryKB: 2048, Passed: 3, Total: 9, TestSetVersion: "v9", RunnerID: "r7"}
	newIngester(&fakeSrc{}, rec).HandleResult(context.Background(), &queue.ResultDelivery{ID: "1-0", Result: r})
	want := store.VerdictRecord{SubmissionID: sid, Verdict: "TLE", RuntimeMS: 1000, MemoryKB: 2048, Passed: 3, Total: 9, TestSetVersion: "v9", RunnerID: "r7"}
	if len(rec.recorded) != 1 || rec.recorded[0] != want {
		t.Fatalf("recorded = %+v, want %+v", rec.recorded, want)
	}
}

type fakeRuns struct {
	set map[string]queue.RunState
	err error
}

func (f *fakeRuns) SetRun(_ context.Context, id string, st queue.RunState) error {
	if f.err != nil {
		return f.err
	}
	if f.set == nil {
		f.set = map[string]queue.RunState{}
	}
	f.set[id] = st
	return nil
}

func TestHandleDeadRun(t *testing.T) {
	job := &queue.Job{SubmissionID: "run-1", Kind: queue.KindRun}
	t.Run("a dead run ends with IE and never touches the database", func(t *testing.T) {
		src, rec, runs := &fakeSrc{}, &fakeRec{result: true}, &fakeRuns{}
		g := newIngester(src, rec)
		g.SetRuns(runs)
		g.HandleDead(context.Background(), &queue.DeadDelivery{ID: "3-0", Job: job, Reason: "max deliveries exceeded"})
		st := runs.set["run-1"]
		if len(rec.recorded) != 0 || len(src.acked) != 1 || st.Status != queue.RunDone || st.Result == nil || st.Result.Verdict != "IE" {
			t.Fatalf("recorded = %+v, acked = %v, run = %+v", rec.recorded, src.acked, st)
		}
	})
	t.Run("a failed run write leaves it pending", func(t *testing.T) {
		src, rec := &fakeSrc{}, &fakeRec{}
		g := newIngester(src, rec)
		g.SetRuns(&fakeRuns{err: errors.New("timeout")})
		g.HandleDead(context.Background(), &queue.DeadDelivery{ID: "3-0", Job: job})
		if len(src.acked) != 0 {
			t.Fatalf("acked = %v, want none", src.acked)
		}
	})
}

func TestHandleDead(t *testing.T) {
	t.Run("a dead-lettered job becomes an IE verdict", func(t *testing.T) {
		src, rec := &fakeSrc{}, &fakeRec{result: true}
		newIngester(src, rec).HandleDead(context.Background(), &queue.DeadDelivery{ID: "2-0", Job: &queue.Job{SubmissionID: sid}, Reason: "max deliveries exceeded", Deliveries: 4})
		want := store.VerdictRecord{SubmissionID: sid, Verdict: "IE", RunnerID: DeadLetterRunnerID}
		if len(rec.recorded) != 1 || rec.recorded[0] != want || len(src.acked) != 1 {
			t.Fatalf("recorded = %+v, acked = %v", rec.recorded, src.acked)
		}
	})
	t.Run("an unreadable entry is acknowledged without a write", func(t *testing.T) {
		src, rec := &fakeSrc{}, &fakeRec{}
		newIngester(src, rec).HandleDead(context.Background(), &queue.DeadDelivery{ID: "2-0", Reason: "undecodable job"})
		if len(rec.recorded) != 0 || len(src.acked) != 1 {
			t.Fatalf("recorded = %+v, acked = %v", rec.recorded, src.acked)
		}
	})
	t.Run("a transient failure leaves it pending", func(t *testing.T) {
		src, rec := &fakeSrc{}, &fakeRec{err: errors.New("timeout")}
		newIngester(src, rec).HandleDead(context.Background(), &queue.DeadDelivery{ID: "2-0", Job: &queue.Job{SubmissionID: sid}})
		if len(src.acked) != 0 {
			t.Fatalf("acked = %v, want none", src.acked)
		}
	})
}

// Run drains results and dead letters from the queue and stops on cancel.
func TestRunProcessesBothStreams(t *testing.T) {
	src := &fakeSrc{
		results: []*queue.ResultDelivery{{ID: "1-0", Result: queue.Result{SubmissionID: sid, Verdict: "AC"}}},
		dead:    []*queue.DeadDelivery{{ID: "2-0", Job: &queue.Job{SubmissionID: "22222222-2222-4222-8222-222222222222"}}},
	}
	rec := &fakeRec{result: true}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { newIngester(src, rec).Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		n := len(rec.recorded)
		rec.mu.Unlock()
		if n == 2 {
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
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.recorded) != 2 {
		t.Fatalf("recorded = %+v, want the result and the IE", rec.recorded)
	}
}
