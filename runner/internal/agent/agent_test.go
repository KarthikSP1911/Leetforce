package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/problem"
	"leetforce/judge/verdict"
	"leetforce/queue"
)

const problemsDir = "../../../problems" // sample-sum has 5 tests

// fakeJudger returns a canned report (or error) after an optional delay.
type fakeJudger struct {
	calls atomic.Int32
	delay time.Duration
	rep   *engine.Report
	err   error

	custom      *engine.CustomReport // canned RunCustom result
	gotTests    []problem.Test       // tests of the problem the last Judge call saw
	gotOptions  engine.Options
	customInput string
}

func (f *fakeJudger) RunCustom(ctx context.Context, _ *problem.Problem, _ string, _, input []byte) (*engine.CustomReport, error) {
	f.calls.Add(1)
	f.customInput = string(input)
	return f.custom, f.err
}

func (f *fakeJudger) Judge(ctx context.Context, p *problem.Problem, _ string, _ []byte, opts engine.Options) (*engine.Report, error) {
	f.calls.Add(1)
	f.gotTests, f.gotOptions = p.Tests, opts
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return f.rep, f.err
}

func acReport() *engine.Report {
	var cases []engine.CaseResult
	for i := 1; i <= 5; i++ {
		cases = append(cases, engine.CaseResult{Case: verdict.Case{Name: fmt.Sprintf("%02d", i), Verdict: verdict.AC}})
	}
	return &engine.Report{
		TestSetVersion: "v-test",
		Cases:          cases,
		Overall:        verdict.Overall{Verdict: verdict.AC, Time: 12 * time.Millisecond, Memory: 4096},
	}
}

func newQueue(t *testing.T, minIdle time.Duration) *queue.Queue {
	t.Helper()
	url := os.Getenv("LEETFORCE_TEST_REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379/0"
	}
	q, err := queue.Open(url, queue.Config{
		Prefix:  fmt.Sprintf("lftest-agent-%s-%d", t.Name(), time.Now().UnixNano()),
		MinIdle: minIdle, MaxDeliveries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := q.Ping(ctx); err != nil {
		_ = q.Close()
		t.Skipf("redis not reachable: %v", err)
	}
	if err := q.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = q.Destroy(context.Background())
		_ = q.Close()
	})
	return q
}

func newAgent(q JobQueue, j Judger) *Agent {
	return New(q, j, Config{
		ID: "r1", ProblemsDir: problemsDir, PollBlock: 100 * time.Millisecond,
		HeartbeatEvery: 40 * time.Millisecond, MaxAttempts: 3,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func enqueueAndReceive(t *testing.T, q *queue.Queue, j queue.Job) *queue.Delivery {
	t.Helper()
	ctx := context.Background()
	if _, err := q.Enqueue(ctx, j); err != nil {
		t.Fatal(err)
	}
	d, err := q.Receive(ctx, "r1", time.Second)
	if err != nil || d == nil {
		t.Fatalf("receive: %+v %v", d, err)
	}
	return d
}

func sampleJob(id string) queue.Job {
	return queue.Job{SubmissionID: id, Problem: "sample-sum", Language: "python", Source: "print(1)"}
}

func results(t *testing.T, q *queue.Queue) []queue.Result {
	t.Helper()
	r, err := q.Results(context.Background(), "-")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// stillPending reports whether the job is still waiting for a consumer: a
// second consumer with a tiny MinIdle would reclaim it only if it is pending.
func stillPending(t *testing.T, q *queue.Queue) bool {
	t.Helper()
	time.Sleep(2 * 300 * time.Millisecond)
	d, err := q.Receive(context.Background(), "probe", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return d != nil
}

func TestProcessReportsVerdictAndAcks(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	newAgent(q, &fakeJudger{rep: acReport()}).Process(context.Background(), d)

	got := results(t, q)
	want := queue.Result{SubmissionID: "s1", Verdict: "AC", RuntimeMS: 12, MemoryKB: 4, TestSetVersion: "v-test", Passed: 5, Total: 5, RunnerID: "r1"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("results = %+v, want [%+v]", got, want)
	}
	if stillPending(t, q) {
		t.Fatal("job was not acknowledged")
	}
}

func TestProcessWrongAnswerHidesFailingTest(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	rep := &engine.Report{
		TestSetVersion: "v-test",
		Cases: []engine.CaseResult{
			{Case: verdict.Case{Name: "01", Verdict: verdict.AC}},
			{Case: verdict.Case{Name: "03", Verdict: verdict.WA}, Detail: &engine.Detail{Input: "secret", Expected: "secret"}},
		},
		Overall: verdict.Overall{Verdict: verdict.WA, Failed: "03"},
	}
	newAgent(q, &fakeJudger{rep: rep}).Process(context.Background(), d)
	got := results(t, q)
	if len(got) != 1 || got[0].Verdict != "WA" || got[0].Passed != 1 || got[0].Total != 5 {
		t.Fatalf("results = %+v", got)
	}
	// queue.Result has no field for the failing test name, input or output.
}

func TestProcessCompileErrorCarriesCompilerMessage(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	rep := &engine.Report{
		CompileOutput: "main.cpp:3: error",
		Cases:         []engine.CaseResult{{Case: verdict.Case{Name: "compile", Verdict: verdict.CE}}},
		Overall:       verdict.Overall{Verdict: verdict.CE},
	}
	newAgent(q, &fakeJudger{rep: rep}).Process(context.Background(), d)
	got := results(t, q)
	if len(got) != 1 || got[0].Verdict != "CE" || got[0].CompileOutput != "main.cpp:3: error" || got[0].Passed != 0 {
		t.Fatalf("results = %+v", got)
	}
}

func TestProcessBadJobsGetIEAndAreAcked(t *testing.T) {
	cases := map[string]struct {
		job queue.Job
		err error
	}{
		"path traversal slug": {queue.Job{SubmissionID: "a", Problem: "../x", Language: "python"}, nil},
		"unknown problem":     {queue.Job{SubmissionID: "b", Problem: "no-such-problem", Language: "python"}, nil},
		"unknown language":    {sampleJob("c"), fmt.Errorf("x: %w", engine.ErrUnknownLanguage)},
		"source too large":    {sampleJob("d"), fmt.Errorf("x: %w", engine.ErrSourceTooLarge)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			q := newQueue(t, 300*time.Millisecond)
			d := enqueueAndReceive(t, q, c.job)
			newAgent(q, &fakeJudger{err: c.err}).Process(context.Background(), d)
			got := results(t, q)
			if len(got) != 1 || got[0].Verdict != VerdictInternalError || got[0].CompileOutput != "" {
				t.Fatalf("results = %+v", got)
			}
			if stillPending(t, q) {
				t.Fatal("bad job was not acknowledged")
			}
		})
	}
}

func TestHostFailureLeavesJobPendingUntilLastAttempt(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	a := newAgent(q, &fakeJudger{err: errors.New("sandbox setup failed")})

	a.Process(context.Background(), d) // delivery 1: retry later
	if got := results(t, q); len(got) != 0 {
		t.Fatalf("verdict recorded after a host failure: %+v", got)
	}
	time.Sleep(400 * time.Millisecond)
	d2, err := q.Receive(context.Background(), "r1", time.Second)
	if err != nil || d2 == nil || d2.Deliveries != 2 {
		t.Fatalf("redelivery: %+v %v", d2, err)
	}

	d2.Deliveries = 3 // last attempt: report IE instead of retrying again
	a.Process(context.Background(), d2)
	if got := results(t, q); len(got) != 1 || got[0].Verdict != VerdictInternalError {
		t.Fatalf("results = %+v", got)
	}
}

func TestRedeliveredJobWithRecordedVerdictIsNotJudgedAgain(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	if _, err := q.Publish(context.Background(), queue.Result{SubmissionID: "s1", Verdict: "AC", RunnerID: "dead"}); err != nil {
		t.Fatal(err)
	}
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	fj := &fakeJudger{rep: acReport()}
	newAgent(q, fj).Process(context.Background(), d)
	if fj.calls.Load() != 0 {
		t.Fatal("judged a submission that already has a verdict")
	}
	if got := results(t, q); len(got) != 1 || got[0].RunnerID != "dead" {
		t.Fatalf("results = %+v", got)
	}
	if stillPending(t, q) {
		t.Fatal("job was not acknowledged")
	}
}

// touchCounter wraps the real queue to count heartbeats or simulate a lost claim.
type touchCounter struct {
	*queue.Queue
	touches atomic.Int32
	lose    bool
}

func (c *touchCounter) Touch(ctx context.Context, consumer, id string) error {
	c.touches.Add(1)
	if c.lose {
		return queue.ErrLost
	}
	return c.Queue.Touch(ctx, consumer, id)
}

func TestHeartbeatRunsWhileJudging(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	tc := &touchCounter{Queue: q}
	newAgent(tc, &fakeJudger{rep: acReport(), delay: 400 * time.Millisecond}).Process(context.Background(), d)
	if n := tc.touches.Load(); n < 3 {
		t.Fatalf("only %d heartbeats during a 400 ms job with a 40 ms interval", n)
	}
	if len(results(t, q)) != 1 {
		t.Fatal("verdict missing")
	}
}

func TestLostClaimCancelsJudgingAndDiscardsResult(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	fj := &fakeJudger{rep: acReport(), delay: 5 * time.Second}
	start := time.Now()
	newAgent(&touchCounter{Queue: q, lose: true}, fj).Process(context.Background(), d)
	if time.Since(start) > 2*time.Second {
		t.Fatal("judging was not cancelled when the claim was lost")
	}
	if got := results(t, q); len(got) != 0 {
		t.Fatalf("a runner that lost the job published %+v", got)
	}
}

func TestRunProcessesJobsUntilCancelled(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 1; i <= 2; i++ {
		if _, err := q.Enqueue(ctx, sampleJob(fmt.Sprintf("s%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = newAgent(q, &fakeJudger{rep: acReport()}).Run(ctx)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for len(results(t, q)) < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := len(results(t, q)); n != 2 {
		t.Fatalf("got %d verdicts, want 2", n)
	}
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

// statusFail wraps the real queue but fails every status publish.
type statusFail struct{ *queue.Queue }

func (statusFail) PublishStatus(context.Context, queue.StatusEvent) error {
	return errors.New("status stream down")
}

func TestProcessReportsJudgingBeforeVerdict(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	ctx := context.Background()
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	newAgent(q, &fakeJudger{rep: acReport()}).Process(ctx, d)

	evs, _, err := q.ReadStatus(ctx, "0-0", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := queue.StatusEvent{SubmissionID: "s1", State: queue.StateJudging, RunnerID: "r1"}
	if len(evs) != 1 || evs[0] != want {
		t.Fatalf("status events = %+v, want [%+v]", evs, want)
	}
	if len(results(t, q)) != 1 {
		t.Fatal("verdict missing")
	}
}

func TestStatusFailureDoesNotStopTheJob(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, sampleJob("s1"))
	newAgent(statusFail{q}, &fakeJudger{rep: acReport()}).Process(context.Background(), d)
	if got := results(t, q); len(got) != 1 || got[0].Verdict != "AC" {
		t.Fatalf("results = %+v, want one AC even though the status publish failed", got)
	}
	if stillPending(t, q) {
		t.Fatal("job was not acknowledged")
	}
}
