package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"leetforce/queue"
)

type fakeMarker struct {
	mu     sync.Mutex
	marked []string
	err    error
}

func (f *fakeMarker) MarkJudging(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, id)
	return f.err == nil, f.err
}

func newWatcher(src StatusSource, mk JudgingMarker) *StatusWatcher {
	return NewStatusWatcher(src, mk, slog.New(slog.NewTextHandler(io.Discard, nil)), StatusConfig{Block: 10 * time.Millisecond, Backoff: time.Millisecond})
}

func TestHandleStatusMarksOnlyValidJudgingEvents(t *testing.T) {
	mk := &fakeMarker{}
	newWatcher(nil, mk).Handle(context.Background(), []queue.StatusEvent{
		{SubmissionID: sid, State: queue.StateJudging},
		{SubmissionID: sid, State: "something-else"},
		{SubmissionID: "not-a-uuid", State: queue.StateJudging},
	})
	if len(mk.marked) != 1 || mk.marked[0] != sid {
		t.Fatalf("marked = %v, want only [%s]", mk.marked, sid)
	}
}

func TestHandleStatusSurvivesDatabaseErrors(t *testing.T) {
	mk := &fakeMarker{err: errors.New("database asleep")}
	// A failing write must not panic or stop the rest of the batch.
	newWatcher(nil, mk).Handle(context.Background(), []queue.StatusEvent{
		{SubmissionID: sid, State: queue.StateJudging},
		{SubmissionID: "22222222-2222-4222-8222-222222222222", State: queue.StateJudging},
	})
	if len(mk.marked) != 2 {
		t.Fatalf("marked = %v, want both attempted", mk.marked)
	}
}

// fakeStatusSrc serves one batch after the tail, then nothing.
type fakeStatusSrc struct {
	mu       sync.Mutex
	tailErrs int
	batch    []queue.StatusEvent
	afters   []string
}

func (f *fakeStatusSrc) StatusTail(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tailErrs > 0 {
		f.tailErrs--
		return "", errors.New("redis down")
	}
	return "5-0", nil
}

func (f *fakeStatusSrc) ReadStatus(ctx context.Context, after string, block time.Duration) ([]queue.StatusEvent, string, error) {
	f.mu.Lock()
	f.afters = append(f.afters, after)
	b := f.batch
	f.batch = nil
	f.mu.Unlock()
	if b != nil {
		return b, "6-0", nil
	}
	select {
	case <-ctx.Done():
	case <-time.After(block):
	}
	return nil, after, nil
}

func TestStatusWatcherRunStartsAtTailAndAdvances(t *testing.T) {
	src := &fakeStatusSrc{tailErrs: 2, batch: []queue.StatusEvent{{SubmissionID: sid, State: queue.StateJudging}}}
	mk := &fakeMarker{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { newWatcher(src, mk).Run(ctx); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mk.mu.Lock()
		n := len(mk.marked)
		mk.mu.Unlock()
		if n == 1 {
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
	mk.mu.Lock()
	defer mk.mu.Unlock()
	if len(mk.marked) != 1 {
		t.Fatalf("marked = %v, want the one event", mk.marked)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.afters) < 2 || src.afters[0] != "5-0" || src.afters[1] != "6-0" {
		t.Fatalf("read positions = %v, want to start at the tail (5-0) and advance (6-0)", src.afters)
	}
}
