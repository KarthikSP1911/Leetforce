package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"leetforce/judge/engine"
	"leetforce/judge/verdict"
	"leetforce/queue"
)

func runJob(id string) queue.Job {
	j := sampleJob(id)
	j.Kind = queue.KindRun
	return j
}

func runState(t *testing.T, q *queue.Queue, id string) queue.RunState {
	t.Helper()
	st, ok, err := q.GetRun(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("GetRun(%s) = %+v, %v, %v", id, st, ok, err)
	}
	return st
}

func TestProcessRunSamplesOnly(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	d := enqueueAndReceive(t, q, runJob("run-1"))
	rep := &engine.Report{
		Overall: verdict.Overall{Verdict: verdict.WA, Time: 5 * time.Millisecond, Memory: 2048},
		Cases: []engine.CaseResult{
			{Case: verdict.Case{Name: "01", Verdict: verdict.AC}},
			{Case: verdict.Case{Name: "02", Verdict: verdict.WA}, Detail: &engine.Detail{Input: "1 2", Expected: "3", Actual: "4", Stderr: "boom"}},
		},
	}
	fj := &fakeJudger{rep: rep}
	newAgent(q, fj).Process(context.Background(), d)

	if len(fj.gotTests) == 0 {
		t.Fatal("no tests were judged")
	}
	for _, tc := range fj.gotTests {
		if !tc.Sample {
			t.Fatalf("a hidden test %q reached a Run", tc.Name)
		}
	}
	if !fj.gotOptions.Detail {
		t.Fatal("Run must ask for failing-sample detail")
	}
	st := runState(t, q, "run-1")
	if st.Status != queue.RunDone || st.Result == nil || st.Result.Verdict != "WA" || len(st.Result.Cases) != 2 {
		t.Fatalf("run state = %+v", st)
	}
	if c := st.Result.Cases[1]; c.Input != "1 2" || c.Expected != "3" || c.Actual != "4" || c.Stderr != "boom" {
		t.Fatalf("failing case = %+v", c)
	}
	if c := st.Result.Cases[0]; c.Input != "" || c.Expected != "" {
		t.Fatalf("passing case leaked detail: %+v", c)
	}
	if got := results(t, q); len(got) != 0 {
		t.Fatalf("a Run published to the results stream: %+v", got)
	}
	if stillPending(t, q) {
		t.Fatal("run job was not acknowledged")
	}
}

func TestProcessRunCustomInput(t *testing.T) {
	q := newQueue(t, 300*time.Millisecond)
	j := runJob("run-2")
	j.Custom, j.Input = true, "4 5\n"
	d := enqueueAndReceive(t, q, j)
	fj := &fakeJudger{custom: &engine.CustomReport{Verdict: verdict.Completed, Stdout: "9\n", Time: 3 * time.Millisecond, Memory: 8192}}
	newAgent(q, fj).Process(context.Background(), d)

	if fj.customInput != "4 5\n" {
		t.Fatalf("input = %q", fj.customInput)
	}
	st := runState(t, q, "run-2")
	if st.Status != queue.RunDone || st.Result.Verdict != "OK" || st.Result.Stdout != "9\n" || st.Result.MemoryKB != 8 {
		t.Fatalf("run state = %+v", st)
	}
	if got := results(t, q); len(got) != 0 {
		t.Fatalf("a Run published to the results stream: %+v", got)
	}
}

func TestProcessRunFailures(t *testing.T) {
	t.Run("a bad request ends with IE", func(t *testing.T) {
		q := newQueue(t, 300*time.Millisecond)
		d := enqueueAndReceive(t, q, runJob("run-3"))
		newAgent(q, &fakeJudger{err: engine.ErrUnknownLanguage}).Process(context.Background(), d)
		if st := runState(t, q, "run-3"); st.Status != queue.RunDone || st.Result.Verdict != VerdictInternalError {
			t.Fatalf("run state = %+v", st)
		}
	})
	t.Run("a host failure on the first delivery leaves it pending", func(t *testing.T) {
		q := newQueue(t, 300*time.Millisecond)
		d := enqueueAndReceive(t, q, runJob("run-4"))
		newAgent(q, &fakeJudger{err: errors.New("sandbox setup failed")}).Process(context.Background(), d)
		if st, ok, _ := q.GetRun(context.Background(), "run-4"); ok && st.Status == queue.RunDone {
			t.Fatalf("run finished despite a retryable failure: %+v", st)
		}
		if !stillPending(t, q) {
			t.Fatal("job was acknowledged; it should stay pending for redelivery")
		}
	})
}
