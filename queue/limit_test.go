package queue

import (
	"context"
	"testing"
	"time"
)

func TestAllowCountsPerKeyAndWindow(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	window := 2 * time.Second

	for i := 1; i <= 3; i++ {
		ok, _, err := q.Allow(ctx, "submit-user:u1", 3, window)
		if err != nil || !ok {
			t.Fatalf("hit %d = %v, %v; want allowed", i, ok, err)
		}
	}
	ok, retry, err := q.Allow(ctx, "submit-user:u1", 3, window)
	if err != nil || ok || retry <= 0 || retry > window {
		t.Fatalf("hit 4 = %v, retry %v, %v; want denied with a retry time inside the window", ok, retry, err)
	}
	// Another key has its own counter.
	if ok, _, err := q.Allow(ctx, "submit-user:u2", 3, window); err != nil || !ok {
		t.Fatalf("other key = %v, %v; want allowed", ok, err)
	}
	// The window ends by itself.
	time.Sleep(window + 300*time.Millisecond)
	if ok, _, err := q.Allow(ctx, "submit-user:u1", 3, window); err != nil || !ok {
		t.Fatalf("after the window = %v, %v; want allowed again", ok, err)
	}
}

// A counter whose expiry was lost (for example a crash between INCR and
// PEXPIRE) must not stay blocked forever.
func TestAllowRepairsCounterWithoutExpiry(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	if err := q.rdb.Set(ctx, q.limitKey("stuck"), 99, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := q.Allow(ctx, "stuck", 3, time.Minute); err != nil {
		t.Fatal(err)
	}
	if ttl := q.rdb.PTTL(ctx, q.limitKey("stuck")).Val(); ttl <= 0 {
		t.Fatalf("ttl = %v, want an expiry to have been set", ttl)
	}
}
