package queue

import (
	"context"
	"sync/atomic"
	"time"
)

// A hosted Redis bills per command, and a blocking read that times out still
// counts as one. The helpers here keep the idle polling loops cheap: look for
// abandoned entries only every ReclaimEvery, wait as long as that for new
// ones, and let a stop signal end a long wait at once.

// reclaimDue reports whether consumer should look for entries abandoned by
// another consumer on stream now, and records that it did. The first call for a
// stream and consumer is always due.
func (q *Queue) reclaimDue(stream, consumer string) bool {
	v, _ := q.lastReclaim.LoadOrStore(stream+"|"+consumer, new(atomic.Int64))
	last := v.(*atomic.Int64)
	now := time.Now().UnixNano()
	prev := last.Load()
	if prev != 0 && now-prev < int64(q.cfg.ReclaimEvery) {
		return false
	}
	last.Store(now)
	return true
}

// capBlock limits a blocking wait to ReclaimEvery, so a consumer comes back to
// look for abandoned entries at least that often.
func (q *Queue) capBlock(block time.Duration) time.Duration {
	return min(block, q.cfg.ReclaimEvery)
}

// untilDone runs a blocking Redis read and returns when it finishes or when ctx
// ends, whichever is first. go-redis does not interrupt a read that is waiting
// on the server when ctx is cancelled, so without this a stop would wait out
// the whole block. The abandoned read finishes in the background; an entry it
// still receives stays pending for this consumer and is reclaimed after MinIdle.
func untilDone[T any](ctx context.Context, read func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := read()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
