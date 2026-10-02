package queue

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestResultsReachTheAPIGroupOnce(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	// Published before the API group exists: SetupAPI reads from the start, so it is not lost.
	want := Result{SubmissionID: "s1", Verdict: "AC", RuntimeMS: 3, MemoryKB: 10, Passed: 2, Total: 2, RunnerID: "r1"}
	if _, err := q.Publish(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := q.SetupAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.SetupAPI(ctx); err != nil {
		t.Fatalf("SetupAPI must be repeatable: %v", err)
	}
	d, err := q.ReceiveResult(ctx, "api1", time.Second)
	if err != nil || d == nil || d.Malformed || d.Result != want {
		t.Fatalf("got %+v, %v", d, err)
	}
	if err := q.AckResult(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if d, err := q.ReceiveResult(ctx, "api1", 50*time.Millisecond); err != nil || d != nil {
		t.Fatalf("acknowledged result came back: %+v, %v", d, err)
	}
	// Acknowledging does not delete: tools that read the stream still see the verdict.
	if all, err := q.Results(ctx, "-"); err != nil || len(all) != 1 {
		t.Fatalf("results after ack = %+v, %v", all, err)
	}
}

func TestUnackedResultIsReclaimed(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{MinIdle: 50 * time.Millisecond})
	if err := q.SetupAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Publish(ctx, Result{SubmissionID: "s1", Verdict: "WA"}); err != nil {
		t.Fatal(err)
	}
	first, err := q.ReceiveResult(ctx, "api1", time.Second)
	if err != nil || first == nil {
		t.Fatalf("first receive = %+v, %v", first, err)
	}
	time.Sleep(q.cfg.MinIdle + 50*time.Millisecond) // api1 "crashed" without acking
	again, err := q.ReceiveResult(ctx, "api2", time.Second)
	if err != nil || again == nil || again.ID != first.ID {
		t.Fatalf("api2 did not reclaim the entry: %+v, %v", again, err)
	}
}

func TestMalformedResultIsFlagged(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	if err := q.SetupAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.rdb.XAdd(ctx, &redis.XAddArgs{Stream: q.results(), Values: map[string]any{"result": "not json"}}).Err(); err != nil {
		t.Fatal(err)
	}
	d, err := q.ReceiveResult(ctx, "api1", time.Second)
	if err != nil || d == nil || !d.Malformed {
		t.Fatalf("got %+v, %v", d, err)
	}
}

func TestDeadLetterReachesTheAPI(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{MinIdle: 50 * time.Millisecond, MaxDeliveries: 1})
	if err := q.SetupAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(ctx, job("poison")); err != nil {
		t.Fatal(err)
	}
	if d, err := q.Receive(ctx, "r1", time.Second); err != nil || d == nil {
		t.Fatalf("first delivery: %+v %v", d, err)
	}
	time.Sleep(q.cfg.MinIdle + 30*time.Millisecond)
	if d, err := q.Receive(ctx, "r2", 50*time.Millisecond); err != nil || d != nil {
		t.Fatalf("second delivery should dead-letter: %+v %v", d, err)
	}
	dead, err := q.ReceiveDead(ctx, "api1", time.Second)
	if err != nil || dead == nil || dead.Job == nil || dead.Job.SubmissionID != "poison" || dead.Reason == "" {
		t.Fatalf("dead = %+v, %v", dead, err)
	}
	if err := q.AckDead(ctx, dead.ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := q.ReceiveDead(ctx, "api1", 50*time.Millisecond); again != nil {
		t.Fatalf("acknowledged dead letter came back: %+v", again)
	}
}

func TestUndecodableDeadLetterHasNoJob(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	if err := q.SetupAPI(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.rdb.XAdd(ctx, &redis.XAddArgs{Stream: q.dead(), Values: map[string]any{"job": "garbage", "reason": "undecodable job"}}).Err(); err != nil {
		t.Fatal(err)
	}
	dead, err := q.ReceiveDead(ctx, "api1", time.Second)
	if err != nil || dead == nil || dead.Job != nil {
		t.Fatalf("dead = %+v, %v", dead, err)
	}
}
