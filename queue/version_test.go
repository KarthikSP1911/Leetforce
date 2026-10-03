package queue

import (
	"context"
	"testing"
)

// A rejudge of a submission against a fixed test set has a new version and
// must not be mistaken for a duplicate of the first verdict.
func TestPublishIsScopedToTestSetVersion(t *testing.T) {
	ctx := context.Background()
	q := newTestQueue(t, Config{})
	r := Result{SubmissionID: "s1", Verdict: "AC", TestSetVersion: "v1", RunnerID: "r1"}
	if ok, err := q.Publish(ctx, r); err != nil || !ok {
		t.Fatalf("publish v1 = %v, %v", ok, err)
	}
	if ok, err := q.Published(ctx, "s1", "v1"); err != nil || !ok {
		t.Fatalf("Published(s1, v1) = %v, %v", ok, err)
	}
	if ok, err := q.Published(ctx, "s1", "v2"); err != nil || ok {
		t.Fatalf("Published(s1, v2) = %v, %v (want false before the rejudge)", ok, err)
	}
	r.TestSetVersion, r.Verdict = "v2", "WA"
	if ok, err := q.Publish(ctx, r); err != nil || !ok {
		t.Fatalf("publish v2 = %v, %v (a rejudge must be recorded)", ok, err)
	}
	if ok, err := q.Publish(ctx, r); err != nil || ok {
		t.Fatalf("second publish v2 = %v, %v (want false)", ok, err)
	}
}
