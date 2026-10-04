package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func (q *Queue) kvKey(k string) string { return q.cfg.Prefix + ":kv:" + k }

// KVGet returns a cached value, and false if it is missing or expired.
func (q *Queue) KVGet(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := q.rdb.Get(ctx, q.kvKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("kv get %s: %w", key, err)
	}
	return b, true, nil
}

// KVSet stores a value with a time to live.
func (q *Queue) KVSet(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	if err := q.rdb.Set(ctx, q.kvKey(key), val, ttl).Err(); err != nil {
		return fmt.Errorf("kv set %s: %w", key, err)
	}
	return nil
}

// KVIncr adds one to a counter and returns the new value. Counters are used as
// cache versions: any bump invalidates every snapshot tagged with an older one.
func (q *Queue) KVIncr(ctx context.Context, key string) (int64, error) {
	n, err := q.rdb.Incr(ctx, q.kvKey(key)).Result()
	if err != nil {
		return 0, fmt.Errorf("kv incr %s: %w", key, err)
	}
	return n, nil
}

// KVCounter reads a counter, 0 if it was never bumped.
func (q *Queue) KVCounter(ctx context.Context, key string) (int64, error) {
	n, err := q.rdb.Get(ctx, q.kvKey(key)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("kv counter %s: %w", key, err)
	}
	return n, nil
}
