package agent

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"leetforce/runner/internal/metrics"
)

func TestProcessCountsVerdictAndJudgeTime(t *testing.T) {
	jobs := metrics.Jobs.WithLabelValues("submission", "AC")
	before := testutil.ToFloat64(jobs)
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("m1"))
	newAgent(q, &fakeJudger{rep: acReport()}).Process(context.Background(), d)

	if got := testutil.ToFloat64(jobs) - before; got != 1 {
		t.Fatalf("AC counter moved by %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.InFlight); got != 0 {
		t.Fatalf("in-flight gauge = %v after the job, want 0", got)
	}
	if n := testutil.CollectAndCount(metrics.JudgeSeconds); n == 0 {
		t.Fatal("judge duration histogram has no series")
	}
}

func TestReclaimedDeliveryIsCounted(t *testing.T) {
	before := testutil.ToFloat64(metrics.Reclaimed)
	q := newQueue(t, 50*time.Millisecond)
	if _, err := q.Enqueue(context.Background(), sampleJob("m2")); err != nil {
		t.Fatal(err)
	}
	if d, err := q.Receive(context.Background(), "dead-runner", time.Second); err != nil || d == nil {
		t.Fatalf("first receive = %v, %v", d, err)
	}
	time.Sleep(120 * time.Millisecond) // the first runner never heartbeats
	a := newAgent(q, &fakeJudger{rep: acReport()})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { _ = a.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for testutil.ToFloat64(metrics.Reclaimed)-before < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := testutil.ToFloat64(metrics.Reclaimed) - before; got != 1 {
		t.Fatalf("reclaimed counter moved by %v, want 1", got)
	}
}
