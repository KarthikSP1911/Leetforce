package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReclaimDue(t *testing.T) {
	q := New(nil, Config{MinIdle: 10 * time.Millisecond, ReclaimEvery: 40 * time.Millisecond})
	if !q.reclaimDue("s", "a") {
		t.Fatal("first call must be due")
	}
	if q.reclaimDue("s", "a") {
		t.Error("second call right after must not be due")
	}
	if !q.reclaimDue("s", "b") || !q.reclaimDue("t", "a") {
		t.Error("another consumer or stream has its own clock")
	}
	time.Sleep(50 * time.Millisecond)
	if !q.reclaimDue("s", "a") {
		t.Error("due again after ReclaimEvery")
	}
}

func TestDefaultsAndBlockCap(t *testing.T) {
	q := New(nil, Config{MinIdle: 30 * time.Second})
	if q.cfg.ReclaimEvery != time.Minute {
		t.Errorf("ReclaimEvery default = %v, want twice MinIdle", q.cfg.ReclaimEvery)
	}
	for _, tc := range []struct{ in, want time.Duration }{
		{5 * time.Second, 5 * time.Second},
		{time.Hour, time.Minute},
	} {
		if got := q.capBlock(tc.in); got != tc.want {
			t.Errorf("capBlock(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestUntilDoneReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	defer close(release)
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := untilDone(ctx, func() (int, error) { <-release; return 1, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("did not return promptly after cancel")
	}
	v, err := untilDone(context.Background(), func() (int, error) { return 7, nil })
	if v != 7 || err != nil {
		t.Errorf("normal read = %d, %v", v, err)
	}
}
