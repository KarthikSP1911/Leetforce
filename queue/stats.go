package queue

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Stats is a point-in-time view of the job queue, for metrics.
type Stats struct {
	Waiting   int64         // entries no runner has taken yet
	Pending   int64         // entries delivered to a runner and not yet acknowledged
	OldestAge time.Duration // age of the oldest entry still in the stream (0 when empty)
	Dead      int64         // entries in the dead-letter stream
}

// Stats reads the queue depth in one round trip. Acknowledged jobs are deleted
// from the stream (Ack), so the stream length is waiting plus pending and the
// first entry is the oldest unfinished job. Age is measured against the Redis
// server clock, so a skewed API host cannot fake a stuck queue.
func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	pipe := q.rdb.Pipeline()
	length := pipe.XLen(ctx, q.jobs())
	pending := pipe.XPending(ctx, q.jobs(), Group)
	first := pipe.XRangeN(ctx, q.jobs(), "-", "+", 1)
	dead := pipe.XLen(ctx, q.dead())
	now := pipe.Time(ctx)
	if _, err := pipe.Exec(ctx); err != nil {
		return Stats{}, fmt.Errorf("queue stats: %w", err)
	}
	st := Stats{Pending: pending.Val().Count, Dead: dead.Val()}
	st.Waiting = max(0, length.Val()-st.Pending)
	if msgs := first.Val(); len(msgs) == 1 {
		if ms, ok := entryMillis(msgs[0]); ok {
			st.OldestAge = max(0, now.Val().Sub(time.UnixMilli(ms)))
		}
	}
	return st, nil
}

func entryMillis(m redis.XMessage) (int64, bool) {
	ms, _, ok := strings.Cut(m.ID, "-")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	return n, err == nil
}
