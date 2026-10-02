// Package queue is the Redis Streams job queue shared by the API (producer)
// and the runners (consumers). Jobs go to one stream read through a consumer
// group; a job a dead runner left unacknowledged is reclaimed with XAUTOCLAIM.
// Verdicts go to a results stream, written at most once per submission.
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

// Group is the consumer group every runner joins.
const Group = "runners"

// ErrLost means this consumer no longer owns the job (another runner
// reclaimed it, or it was acknowledged).
var ErrLost = errors.New("queue: job no longer owned by this consumer")

// Config tunes a Queue. Zero values take the defaults noted per field.
type Config struct {
	Prefix        string        // key prefix (default "leetforce"); tests use a unique one
	MinIdle       time.Duration // a pending job idle this long is reclaimable (default 30s)
	MaxDeliveries int64         // deliveries before a job is dead-lettered (default 3)
	VerdictTTL    time.Duration // how long a verdict marker blocks duplicates (default 7 days)
}

// Job is one submission to judge. Source travels in the job so a runner needs
// nothing but Redis; hidden tests are loaded from the problem directory.
type Job struct {
	SubmissionID string `json:"submission_id"`
	Problem      string `json:"problem"`
	Language     string `json:"language"`
	Source       string `json:"source"`
}

// Result is the verdict a runner reports. It deliberately carries no test
// input, expected output or stderr (Submit never returns them).
type Result struct {
	SubmissionID   string `json:"submission_id"`
	Verdict        string `json:"verdict"`
	RuntimeMS      int64  `json:"runtime_ms"`
	MemoryKB       uint64 `json:"memory_kb"`
	TestSetVersion string `json:"test_set_version"`
	Passed         int    `json:"passed"`
	Total          int    `json:"total"`
	CompileOutput  string `json:"compile_output,omitempty"`
	RunnerID       string `json:"runner_id"`
}

// Delivery is a job handed to a consumer.
type Delivery struct {
	ID         string // stream entry ID, used to Ack and Touch
	Job        Job
	Reclaimed  bool  // true when taken over from another consumer
	Deliveries int64 // how many times this entry has been delivered, including this one
}

// Queue is a handle on the job and result streams.
type Queue struct {
	rdb redis.UniversalClient
	cfg Config
}

// Open connects using a redis:// or rediss:// URL.
func Open(url string, cfg Config) (*Queue, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return New(redis.NewClient(opt), cfg), nil
}

// New wraps an existing client.
func New(rdb redis.UniversalClient, cfg Config) *Queue {
	if cfg.Prefix == "" {
		cfg.Prefix = "leetforce"
	}
	if cfg.MinIdle <= 0 {
		cfg.MinIdle = 30 * time.Second
	}
	if cfg.MaxDeliveries <= 0 {
		cfg.MaxDeliveries = 3
	}
	if cfg.VerdictTTL <= 0 {
		cfg.VerdictTTL = 7 * 24 * time.Hour
	}
	return &Queue{rdb: rdb, cfg: cfg}
}

// Close releases the connection.
func (q *Queue) Close() error { return q.rdb.Close() }

// Ping checks the connection.
func (q *Queue) Ping(ctx context.Context) error { return q.rdb.Ping(ctx).Err() }

func (q *Queue) jobs() string    { return q.cfg.Prefix + ":jobs" }
func (q *Queue) dead() string    { return q.cfg.Prefix + ":jobs:dead" }
func (q *Queue) results() string { return q.cfg.Prefix + ":results" }
func (q *Queue) marker(id string) string {
	return q.cfg.Prefix + ":verdict:" + id
}

// Setup creates the consumer group (and the stream) if missing. It reads from
// the start, so jobs enqueued before the first runner starts are not lost.
func (q *Queue) Setup(ctx context.Context) error {
	err := q.rdb.XGroupCreateMkStream(ctx, q.jobs(), Group, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

// Enqueue adds a job and returns its stream entry ID.
func (q *Queue) Enqueue(ctx context.Context, j Job) (string, error) {
	if j.SubmissionID == "" || j.Problem == "" || j.Language == "" {
		return "", errors.New("enqueue: submission_id, problem and language are required")
	}
	b, err := json.Marshal(j)
	if err != nil {
		return "", fmt.Errorf("enqueue: %w", err)
	}
	id, err := q.rdb.XAdd(ctx, &redis.XAddArgs{Stream: q.jobs(), Values: map[string]any{"job": string(b)}}).Result()
	if err != nil {
		return "", fmt.Errorf("enqueue: %w", err)
	}
	return id, nil
}

// Receive returns the next job for consumer, or nil if none arrived within
// block. It first reclaims a job abandoned by another consumer, then reads a
// new one. A job delivered more than MaxDeliveries times is moved to the
// dead-letter stream instead of being returned, so a job that keeps killing
// runners cannot loop forever.
func (q *Queue) Receive(ctx context.Context, consumer string, block time.Duration) (*Delivery, error) {
	for {
		msgs, _, err := q.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: q.jobs(), Group: Group, Consumer: consumer,
			MinIdle: q.cfg.MinIdle, Start: "0-0", Count: 1,
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("autoclaim: %w", err)
		}
		if len(msgs) == 0 {
			break
		}
		d, err := q.deliver(ctx, msgs[0], true)
		if err != nil {
			return nil, err
		}
		if d != nil {
			return d, nil
		}
	}
	for {
		res, err := q.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: Group, Consumer: consumer, Streams: []string{q.jobs(), ">"},
			Count: 1, Block: block,
		}).Result()
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read group: %w", err)
		}
		if len(res) == 0 || len(res[0].Messages) == 0 {
			return nil, nil
		}
		d, err := q.deliver(ctx, res[0].Messages[0], false)
		if err != nil {
			return nil, err
		}
		if d != nil {
			return d, nil
		}
		// dead-lettered or undecodable: look again without waiting a full block
		block = time.Millisecond
	}
}

// deliver turns a stream message into a Delivery, or dead-letters it and
// returns nil when it is malformed or has used up its deliveries.
func (q *Queue) deliver(ctx context.Context, m redis.XMessage, reclaimed bool) (*Delivery, error) {
	pend, err := q.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: q.jobs(), Group: Group, Start: m.ID, End: m.ID, Count: 1,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("pending info: %w", err)
	}
	var tries int64 = 1
	if len(pend) == 1 {
		tries = pend[0].RetryCount
	}
	var j Job
	raw, _ := m.Values["job"].(string)
	if err := json.Unmarshal([]byte(raw), &j); err != nil || tries > q.cfg.MaxDeliveries {
		reason := "max deliveries exceeded"
		if err != nil {
			reason = "undecodable job"
		}
		if derr := q.deadLetter(ctx, m, reason, tries); derr != nil {
			return nil, derr
		}
		return nil, nil
	}
	return &Delivery{ID: m.ID, Job: j, Reclaimed: reclaimed, Deliveries: tries}, nil
}

func (q *Queue) deadLetter(ctx context.Context, m redis.XMessage, reason string, tries int64) error {
	pipe := q.rdb.TxPipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: q.dead(), Values: map[string]any{
		"job": m.Values["job"], "entry": m.ID, "reason": reason, "deliveries": tries,
	}})
	pipe.XAck(ctx, q.jobs(), Group, m.ID)
	pipe.XDel(ctx, q.jobs(), m.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("dead-letter %s: %w", m.ID, err)
	}
	return nil
}

// Ack marks a job finished and removes it from the stream.
func (q *Queue) Ack(ctx context.Context, id string) error {
	pipe := q.rdb.TxPipeline()
	pipe.XAck(ctx, q.jobs(), Group, id)
	pipe.XDel(ctx, q.jobs(), id)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("ack %s: %w", id, err)
	}
	return nil
}

// touchScript resets a pending entry's idle time only if consumer still owns it.
var touchScript = redis.NewScript(`
local p = redis.call('XPENDING', KEYS[1], ARGV[1], ARGV[2], ARGV[2], 1)
if #p == 0 or p[1][2] ~= ARGV[3] then return 0 end
redis.call('XCLAIM', KEYS[1], ARGV[1], ARGV[3], 0, ARGV[2], 'JUSTID')
return 1
`)

// Touch is the heartbeat: it tells the queue this consumer is still working on
// the job so nobody reclaims it. It returns ErrLost if the job was taken away.
func (q *Queue) Touch(ctx context.Context, consumer, id string) error {
	n, err := touchScript.Run(ctx, q.rdb, []string{q.jobs()}, Group, id, consumer).Int()
	if err != nil {
		return fmt.Errorf("touch %s: %w", id, err)
	}
	if n == 0 {
		return ErrLost
	}
	return nil
}

// publishScript records the verdict marker and appends to the results stream
// in one step; a second publish for the same submission does nothing.
var publishScript = redis.NewScript(`
if redis.call('SET', KEYS[1], ARGV[2], 'NX', 'EX', ARGV[1]) then
  redis.call('XADD', KEYS[2], '*', 'result', ARGV[3])
  return 1
end
return 0
`)

// Publish reports a verdict. It returns true if this call recorded it and
// false if a verdict for the submission already existed (idempotent).
func (q *Queue) Publish(ctx context.Context, r Result) (bool, error) {
	if r.SubmissionID == "" || r.Verdict == "" {
		return false, errors.New("publish: submission_id and verdict are required")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return false, fmt.Errorf("publish: %w", err)
	}
	n, err := publishScript.Run(ctx, q.rdb, []string{q.marker(r.SubmissionID), q.results()},
		int64(q.cfg.VerdictTTL.Seconds()), r.RunnerID, string(b)).Int()
	if err != nil {
		return false, fmt.Errorf("publish: %w", err)
	}
	return n == 1, nil
}

// Published reports whether a verdict for the submission was already recorded,
// so a runner that gets a redelivered job can acknowledge it without judging
// it again.
func (q *Queue) Published(ctx context.Context, submissionID string) (bool, error) {
	n, err := q.rdb.Exists(ctx, q.marker(submissionID)).Result()
	if err != nil {
		return false, fmt.Errorf("check verdict: %w", err)
	}
	return n == 1, nil
}

// Results lists published verdicts after the given stream ID ("-" for all).
// The API consumes the stream with its own group in Phase 4; this is for
// tests and tools.
func (q *Queue) Results(ctx context.Context, from string) ([]Result, error) {
	if from == "" {
		from = "-"
	}
	msgs, err := q.rdb.XRange(ctx, q.results(), from, "+").Result()
	if err != nil {
		return nil, fmt.Errorf("read results: %w", err)
	}
	out := make([]Result, 0, len(msgs))
	for _, m := range msgs {
		var r Result
		raw, _ := m.Values["result"].(string)
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, fmt.Errorf("decode result %s: %w", m.ID, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// DeadLetters returns how many jobs were dead-lettered.
func (q *Queue) DeadLetters(ctx context.Context) (int64, error) {
	n, err := q.rdb.XLen(ctx, q.dead()).Result()
	if err != nil {
		return 0, fmt.Errorf("dead letter length: %w", err)
	}
	return n, nil
}

// Destroy deletes every key under this queue's prefix. It is for tests and
// tools that use a throwaway prefix; never call it on the production prefix.
func (q *Queue) Destroy(ctx context.Context) error {
	var cursor uint64
	for {
		keys, next, err := q.rdb.Scan(ctx, cursor, q.cfg.Prefix+":*", 100).Result()
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if len(keys) > 0 {
			if err := q.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
		}
		if cursor = next; cursor == 0 {
			return nil
		}
	}
}
