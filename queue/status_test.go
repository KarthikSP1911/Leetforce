package queue

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestStatusPublishAndRead(t *testing.T) {
	q := newTestQueue(t, Config{})
	ctx := context.Background()

	tail, err := q.StatusTail(ctx)
	if err != nil || tail != "0-0" {
		t.Fatalf("tail of an empty stream = %q, %v; want 0-0", tail, err)
	}
	if evs, next, err := q.ReadStatus(ctx, tail, 20*time.Millisecond); err != nil || len(evs) != 0 || next != tail {
		t.Fatalf("read of an empty stream = %v, %q, %v", evs, next, err)
	}

	want := []StatusEvent{
		{SubmissionID: "a", State: StateJudging, RunnerID: "r1"},
		{SubmissionID: "b", State: StateJudging, RunnerID: "r2"},
	}
	for _, e := range want {
		if err := q.PublishStatus(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, next, err := q.ReadStatus(ctx, tail, time.Second)
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("read = %+v, %v; want %+v", got, err, want)
	}
	// Reading on from the returned ID sees nothing new; no acknowledgement exists.
	if evs, _, err := q.ReadStatus(ctx, next, 20*time.Millisecond); err != nil || len(evs) != 0 {
		t.Fatalf("second read = %v, %v; want nothing", evs, err)
	}
	// A reader that starts later begins at the tail and skips history.
	tail2, err := q.StatusTail(ctx)
	if err != nil || tail2 != next {
		t.Fatalf("tail = %q, %v; want %q", tail2, err, next)
	}
}

func TestStatusSkipsUndecodableEntries(t *testing.T) {
	q := newTestQueue(t, Config{})
	ctx := context.Background()
	if err := q.rdb.XAdd(ctx, xaddRaw(q.status(), "not json")).Err(); err != nil {
		t.Fatal(err)
	}
	if err := q.PublishStatus(ctx, StatusEvent{SubmissionID: "ok", State: StateJudging}); err != nil {
		t.Fatal(err)
	}
	got, _, err := q.ReadStatus(ctx, "0-0", time.Second)
	if err != nil || len(got) != 1 || got[0].SubmissionID != "ok" {
		t.Fatalf("read = %+v, %v; want only the valid event", got, err)
	}
}

func TestPublishStatusValidates(t *testing.T) {
	q := newTestQueue(t, Config{})
	if err := q.PublishStatus(context.Background(), StatusEvent{State: StateJudging}); err == nil {
		t.Error("missing submission_id accepted")
	}
	if err := q.PublishStatus(context.Background(), StatusEvent{SubmissionID: "a"}); err == nil {
		t.Error("missing state accepted")
	}
}

func xaddRaw(stream, event string) *redis.XAddArgs {
	return &redis.XAddArgs{Stream: stream, Values: map[string]any{"event": event}}
}
