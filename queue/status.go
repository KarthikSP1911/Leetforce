package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// StateJudging is the one state a runner reports: it took the job and started
// judging. Queued and judged come from the API's own database writes.
const StateJudging = "judging"

// statusMaxLen bounds the status stream. Entries are only needed for as long as
// the API takes to read them, so an approximate cap of a few thousand is far
// more than enough and keeps Redis memory flat.
const statusMaxLen = 5000

// StatusEvent says a submission changed state. It is best effort: a lost event
// only means a submission shows "queued" until its verdict arrives.
type StatusEvent struct {
	SubmissionID string `json:"submission_id"`
	State        string `json:"state"`
	RunnerID     string `json:"runner_id"`
}

func (q *Queue) status() string { return q.cfg.Prefix + ":status" }

// PublishStatus appends a status event. The stream is trimmed to roughly
// statusMaxLen entries.
func (q *Queue) PublishStatus(ctx context.Context, e StatusEvent) error {
	if e.SubmissionID == "" || e.State == "" {
		return errors.New("publish status: submission_id and state are required")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("publish status: %w", err)
	}
	err = q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: q.status(), MaxLen: statusMaxLen, Approx: true,
		Values: map[string]any{"event": string(b)},
	}).Err()
	if err != nil {
		return fmt.Errorf("publish status: %w", err)
	}
	return nil
}

// StatusTail returns the ID of the newest status entry ("0-0" if the stream is
// empty). Start reading from it so only events after startup are processed.
func (q *Queue) StatusTail(ctx context.Context) (string, error) {
	msgs, err := q.rdb.XRevRangeN(ctx, q.status(), "+", "-", 1).Result()
	if err != nil {
		return "", fmt.Errorf("status tail: %w", err)
	}
	if len(msgs) == 0 {
		return "0-0", nil
	}
	return msgs[0].ID, nil
}

// ReadStatus returns status events after the given entry ID, waiting up to
// block when there are none, together with the ID to pass next time. There is
// no consumer group and nothing to acknowledge: every API instance reads the
// whole stream and applies the same idempotent update. Entries that cannot be
// decoded are skipped.
func (q *Queue) ReadStatus(ctx context.Context, after string, block time.Duration) ([]StatusEvent, string, error) {
	res, err := q.rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{q.status(), after}, Count: 100, Block: block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, after, nil
	}
	if err != nil {
		return nil, after, fmt.Errorf("read status: %w", err)
	}
	var out []StatusEvent
	next := after
	for _, s := range res {
		for _, m := range s.Messages {
			next = m.ID
			var e StatusEvent
			raw, _ := m.Values["event"].(string)
			if json.Unmarshal([]byte(raw), &e) == nil && e.SubmissionID != "" {
				out = append(out, e)
			}
		}
	}
	return out, next, nil
}
