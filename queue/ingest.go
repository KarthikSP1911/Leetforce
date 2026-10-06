package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// APIGroup is the consumer group the API uses on the results and dead-letter
// streams. It is separate from Group, which only reads the jobs stream.
const APIGroup = "api"

// ResultDelivery is a verdict handed to the API. Malformed is true when the
// stream entry could not be decoded; the caller should log it and acknowledge.
type ResultDelivery struct {
	ID        string
	Result    Result
	Malformed bool
}

// DeadDelivery is a dead-lettered job handed to the API so it can report the
// submission as an internal error. Job is nil when the entry was undecodable.
type DeadDelivery struct {
	ID         string
	Job        *Job
	Reason     string
	Deliveries int64
}

// SetupAPI creates the API's consumer groups on the results and dead-letter
// streams (and the streams), reading from the start so nothing published
// before the API first runs is lost.
func (q *Queue) SetupAPI(ctx context.Context) error {
	for _, stream := range []string{q.results(), q.dead()} {
		err := q.rdb.XGroupCreateMkStream(ctx, stream, APIGroup, "0").Err()
		if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
			return fmt.Errorf("create api consumer group on %s: %w", stream, err)
		}
	}
	return nil
}

// nextEntry returns the next entry of stream for the API group: first one
// another consumer left unacknowledged for MinIdle (a crashed API instance),
// then a new one, waiting up to block (at most ReclaimEvery). The first look
// happens at most once per ReclaimEvery. It returns nil when there is none.
func (q *Queue) nextEntry(ctx context.Context, stream, consumer string, block time.Duration) (*redis.XMessage, error) {
	if q.reclaimDue(stream, consumer) {
		msgs, _, err := q.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: stream, Group: APIGroup, Consumer: consumer,
			MinIdle: q.cfg.MinIdle, Start: "0-0", Count: 1,
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("autoclaim %s: %w", stream, err)
		}
		if len(msgs) > 0 {
			return &msgs[0], nil
		}
	}
	block = q.capBlock(block)
	res, err := untilDone(ctx, func() ([]redis.XStream, error) {
		return q.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: APIGroup, Consumer: consumer, Streams: []string{stream, ">"}, Count: 1, Block: block,
		}).Result()
	})
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read group %s: %w", stream, err)
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, nil
	}
	return &res[0].Messages[0], nil
}

// ReceiveResult returns the next verdict from the results stream, or nil if
// none arrived within block. The entry stays pending until AckResult.
func (q *Queue) ReceiveResult(ctx context.Context, consumer string, block time.Duration) (*ResultDelivery, error) {
	m, err := q.nextEntry(ctx, q.results(), consumer, block)
	if err != nil || m == nil {
		return nil, err
	}
	d := &ResultDelivery{ID: m.ID}
	raw, _ := m.Values["result"].(string)
	if err := json.Unmarshal([]byte(raw), &d.Result); err != nil {
		d.Malformed = true
	}
	return d, nil
}

// AckResult marks a verdict as stored. The entry is kept in the stream (tools
// such as lfq read it); only the API group's pending record is cleared.
func (q *Queue) AckResult(ctx context.Context, id string) error {
	if err := q.rdb.XAck(ctx, q.results(), APIGroup, id).Err(); err != nil {
		return fmt.Errorf("ack result %s: %w", id, err)
	}
	return nil
}

// ReceiveDead returns the next dead-lettered job, or nil if none arrived
// within block. The entry stays pending until AckDead.
func (q *Queue) ReceiveDead(ctx context.Context, consumer string, block time.Duration) (*DeadDelivery, error) {
	m, err := q.nextEntry(ctx, q.dead(), consumer, block)
	if err != nil || m == nil {
		return nil, err
	}
	d := &DeadDelivery{ID: m.ID}
	d.Reason, _ = m.Values["reason"].(string)
	if s, ok := m.Values["deliveries"].(string); ok {
		_, _ = fmt.Sscan(s, &d.Deliveries)
	}
	var j Job
	if raw, _ := m.Values["job"].(string); json.Unmarshal([]byte(raw), &j) == nil && j.SubmissionID != "" {
		d.Job = &j
	}
	return d, nil
}

// AckDead marks a dead-lettered job as handled.
func (q *Queue) AckDead(ctx context.Context, id string) error {
	if err := q.rdb.XAck(ctx, q.dead(), APIGroup, id).Err(); err != nil {
		return fmt.Errorf("ack dead %s: %w", id, err)
	}
	return nil
}
