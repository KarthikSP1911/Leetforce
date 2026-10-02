package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newTestQueue returns a Queue on a unique key prefix against a real Redis
// (LEETFORCE_TEST_REDIS_URL, default local). XAUTOCLAIM and Lua behaviour are
// the point of these tests, so there is no fake. Skips if Redis is unreachable.
func newTestQueue(t *testing.T, cfg Config) *Queue {
	t.Helper()
	url := os.Getenv("LEETFORCE_TEST_REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379/0"
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	rdb := redis.NewClient(opt)
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("redis not reachable: %v", err)
	}
	// Timing tests set a small MinIdle; scale it to the measured round trip so a
	// slow remote Redis (Upstash) does not make a live job look idle.
	if cfg.MinIdle > 0 {
		start := time.Now()
		_ = rdb.Ping(ctx).Err()
		cfg.MinIdle = max(cfg.MinIdle, 10*time.Since(start))
	}
	cfg.Prefix = fmt.Sprintf("lftest-%s-%d", t.Name(), time.Now().UnixNano())
	q := New(rdb, cfg)
	if err := q.Setup(ctx); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() {
		keys, _ := rdb.Keys(context.Background(), cfg.Prefix+"*").Result()
		if len(keys) > 0 {
			rdb.Del(context.Background(), keys...)
		}
		_ = rdb.Close()
	})
	return q
}

func job(id string) Job {
	return Job{SubmissionID: id, Problem: "sample-sum", Language: "python", Source: "print(1)"}
}

func TestEnqueueReceiveAck(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	if _, err := q.Enqueue(ctx, job("s1")); err != nil {
		t.Fatal(err)
	}
	d, err := q.Receive(ctx, "r1", time.Second)
	if err != nil || d == nil {
		t.Fatalf("receive: %v %v", d, err)
	}
	if d.Job != job("s1") || d.Reclaimed || d.Deliveries != 1 {
		t.Fatalf("unexpected delivery %+v", d)
	}
	if err := q.Ack(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	again, err := q.Receive(ctx, "r1", 50*time.Millisecond)
	if err != nil || again != nil {
		t.Fatalf("queue should be empty, got %+v %v", again, err)
	}
}

func TestEnqueueValidates(t *testing.T) {
	q := newTestQueue(t, Config{})
	for _, j := range []Job{{}, {SubmissionID: "x"}, {SubmissionID: "x", Problem: "p"}} {
		if _, err := q.Enqueue(context.Background(), j); err == nil {
			t.Errorf("expected error for %+v", j)
		}
	}
}

func TestJobsEnqueuedBeforeGroupExistsAreDelivered(t *testing.T) {
	// Setup reads from "0": a job added before any runner joined is not lost.
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	if _, err := q.Enqueue(ctx, job("early")); err != nil {
		t.Fatal(err)
	}
	d, err := q.Receive(ctx, "late-runner", time.Second)
	if err != nil || d == nil || d.Job.SubmissionID != "early" {
		t.Fatalf("got %+v %v", d, err)
	}
}

func TestAbandonedJobIsReclaimedByAnotherConsumer(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{MinIdle: 150 * time.Millisecond})
	if _, err := q.Enqueue(ctx, job("crash")); err != nil {
		t.Fatal(err)
	}
	first, err := q.Receive(ctx, "dead-runner", time.Second)
	if err != nil || first == nil {
		t.Fatalf("first receive: %v %v", first, err)
	}
	// Too early: still owned.
	if d, _ := q.Receive(ctx, "r2", 20*time.Millisecond); d != nil {
		t.Fatalf("job reclaimed before MinIdle: %+v", d)
	}
	time.Sleep(q.cfg.MinIdle + 50*time.Millisecond)
	d, err := q.Receive(ctx, "r2", time.Second)
	if err != nil || d == nil {
		t.Fatalf("reclaim: %v %v", d, err)
	}
	if !d.Reclaimed || d.Job.SubmissionID != "crash" || d.Deliveries != 2 {
		t.Fatalf("unexpected reclaim %+v", d)
	}
	// The dead runner is no longer the owner.
	if err := q.Touch(ctx, "dead-runner", d.ID); !errors.Is(err, ErrLost) {
		t.Fatalf("touch by old owner = %v, want ErrLost", err)
	}
	if err := q.Ack(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTouchKeepsJobFromBeingReclaimed(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{MinIdle: 200 * time.Millisecond})
	if _, err := q.Enqueue(ctx, job("slow")); err != nil {
		t.Fatal(err)
	}
	d, err := q.Receive(ctx, "r1", time.Second)
	if err != nil || d == nil {
		t.Fatalf("receive: %v %v", d, err)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(q.cfg.MinIdle / 3) // five sleeps add up to well over MinIdle
		if err := q.Touch(ctx, "r1", d.ID); err != nil {
			t.Fatalf("touch %d: %v", i, err)
		}
		if other, _ := q.Receive(ctx, "r2", 5*time.Millisecond); other != nil {
			t.Fatalf("job stolen while heartbeating: %+v", other)
		}
	}
}

func TestTouchOfUnknownEntryIsLost(t *testing.T) {
	q := newTestQueue(t, Config{})
	if err := q.Touch(context.Background(), "r1", "1-1"); !errors.Is(err, ErrLost) {
		t.Fatalf("got %v, want ErrLost", err)
	}
}

func TestPoisonJobIsDeadLettered(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{MinIdle: 50 * time.Millisecond, MaxDeliveries: 2})
	if _, err := q.Enqueue(ctx, job("poison")); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ { // two deliveries, each abandoned
		d, err := q.Receive(ctx, fmt.Sprintf("r%d", i), time.Second)
		if err != nil || d == nil || d.Deliveries != int64(i) {
			t.Fatalf("delivery %d: %+v %v", i, d, err)
		}
		time.Sleep(q.cfg.MinIdle + 30*time.Millisecond)
	}
	d, err := q.Receive(ctx, "r3", 50*time.Millisecond)
	if err != nil || d != nil {
		t.Fatalf("third delivery should dead-letter, got %+v %v", d, err)
	}
	if n, err := q.DeadLetters(ctx); err != nil || n != 1 {
		t.Fatalf("dead letters = %d, %v", n, err)
	}
	if d, _ := q.Receive(ctx, "r4", 50*time.Millisecond); d != nil {
		t.Fatalf("dead-lettered job came back: %+v", d)
	}
}

func TestUndecodableJobIsDeadLettered(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	if err := q.rdb.XAdd(ctx, &redis.XAddArgs{Stream: q.jobs(), Values: map[string]any{"job": "not json"}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(ctx, job("good")); err != nil {
		t.Fatal(err)
	}
	d, err := q.Receive(ctx, "r1", time.Second)
	if err != nil || d == nil || d.Job.SubmissionID != "good" {
		t.Fatalf("got %+v %v", d, err)
	}
	if n, _ := q.DeadLetters(ctx); n != 1 {
		t.Fatalf("dead letters = %d, want 1", n)
	}
}

func TestPublishIsIdempotentPerSubmission(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	r := Result{SubmissionID: "s1", Verdict: "AC", RuntimeMS: 12, MemoryKB: 900, Passed: 5, Total: 5, RunnerID: "r1"}
	first, err := q.Publish(ctx, r)
	if err != nil || !first {
		t.Fatalf("first publish = %v, %v", first, err)
	}
	r2 := r
	r2.Verdict, r2.RunnerID = "WA", "r2"
	second, err := q.Publish(ctx, r2)
	if err != nil || second {
		t.Fatalf("second publish = %v, %v (want false)", second, err)
	}
	if ok, err := q.Published(ctx, "s1"); err != nil || !ok {
		t.Fatalf("Published(s1) = %v, %v", ok, err)
	}
	if ok, err := q.Published(ctx, "other"); err != nil || ok {
		t.Fatalf("Published(other) = %v, %v", ok, err)
	}
	got, err := q.Results(ctx, "-")
	if err != nil || len(got) != 1 || got[0] != r {
		t.Fatalf("results = %+v, %v", got, err)
	}
}

func TestPublishValidates(t *testing.T) {
	q := newTestQueue(t, Config{})
	if _, err := q.Publish(context.Background(), Result{}); err == nil {
		t.Fatal("expected error")
	}
}
