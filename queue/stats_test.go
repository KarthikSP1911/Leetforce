package queue

import (
	"context"
	"testing"
	"time"
)

func TestStatsCountsWaitingPendingAndDead(t *testing.T) {
	q := newTestQueue(t, Config{})
	ctx := context.Background()
	if err := q.Setup(ctx); err != nil {
		t.Fatal(err)
	}

	if st, err := q.Stats(ctx); err != nil || st != (Stats{}) {
		t.Fatalf("empty queue stats = %+v, %v; want zero", st, err)
	}

	for _, id := range []string{"a", "b", "c"} {
		if _, err := q.Enqueue(ctx, Job{SubmissionID: id, Problem: "p", Language: "python"}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	d, err := q.Receive(ctx, "r1", time.Second)
	if err != nil || d == nil {
		t.Fatalf("receive = %v, %v", d, err)
	}

	st, err := q.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Waiting != 2 || st.Pending != 1 || st.Dead != 0 || st.OldestAge < 20*time.Millisecond {
		t.Fatalf("stats = %+v; want 2 waiting, 1 pending, 0 dead, age >= 20ms", st)
	}

	if err := q.Ack(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	st, _ = q.Stats(ctx)
	if st.Waiting != 2 || st.Pending != 0 {
		t.Fatalf("after ack stats = %+v; want 2 waiting, 0 pending", st)
	}
}
