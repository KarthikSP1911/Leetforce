package queue

import (
	"context"
	"fmt"
	"time"
)

func (q *Queue) limitKey(key string) string { return q.cfg.Prefix + ":rl:" + key }

// Allow counts one hit against key in a fixed window and reports whether it is
// within limit. When it is not, retryAfter says how long until the window
// resets. The counter lives in Redis so every API instance shares it.
func (q *Queue) Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, retryAfter time.Duration, err error) {
	k := q.limitKey(key)
	n, err := q.rdb.Incr(ctx, k).Result()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit %s: %w", key, err)
	}
	ttl, err := q.rdb.PTTL(ctx, k).Result()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit %s: %w", key, err)
	}
	// ttl < 0 covers a fresh key and one whose EXPIRE was lost, so a counter can
	// never get stuck without an end.
	if n == 1 || ttl < 0 {
		if err := q.rdb.PExpire(ctx, k, window).Err(); err != nil {
			return false, 0, fmt.Errorf("rate limit %s: %w", key, err)
		}
		ttl = window
	}
	if int(n) > limit {
		return false, ttl, nil
	}
	return true, 0, nil
}
