package verdict

import (
	"syscall"
	"testing"
	"time"

	"leetforce/judge/sandbox"
)

var lim = Limits{Time: time.Second, MemoryBytes: 128 << 20}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		r    sandbox.Result
		want Verdict
	}{
		{"clean exit", sandbox.Result{CPUTime: 100 * time.Millisecond, PeakMemoryBytes: 10 << 20}, Completed},
		{"exactly at the limits", sandbox.Result{CPUTime: time.Second, PeakMemoryBytes: 128 << 20}, Completed},
		{"output flood", sandbox.Result{OutputExceeded: true, Signal: syscall.SIGKILL, ExitCode: -1}, OLE},
		{"oom kill", sandbox.Result{OOMKilled: true, Signal: syscall.SIGKILL, ExitCode: -1}, MLE},
		{"peak over limit", sandbox.Result{PeakMemoryBytes: 129 << 20}, MLE},
		{"wall timeout", sandbox.Result{TimedOut: true, Signal: syscall.SIGKILL, ExitCode: -1}, TLE},
		{"cpu over limit, sigkill", sandbox.Result{CPUTime: 1100 * time.Millisecond, Signal: syscall.SIGKILL, ExitCode: -1}, TLE},
		{"sigxcpu", sandbox.Result{Signal: syscall.SIGXCPU, ExitCode: -1}, TLE},
		{"segfault", sandbox.Result{Signal: syscall.SIGSEGV, ExitCode: -1}, RE},
		{"abort", sandbox.Result{Signal: syscall.SIGABRT, ExitCode: -1}, RE},
		{"non-zero exit", sandbox.Result{ExitCode: 1}, RE},
		{"program exits 137 on its own, no kill", sandbox.Result{ExitCode: 137}, RE},
		{"fork refused", sandbox.Result{PIDLimitHit: true}, RE},
		// A plain SIGKILL with no measured cause is a crash, not a time limit.
		{"sigkill with low cpu and memory", sandbox.Result{Signal: syscall.SIGKILL, ExitCode: -1, CPUTime: time.Millisecond}, RE},
		// Order: OLE beats MLE beats TLE beats RE.
		{"oom and timeout", sandbox.Result{OOMKilled: true, TimedOut: true}, MLE},
		{"flood and timeout", sandbox.Result{OutputExceeded: true, TimedOut: true}, OLE},
		{"timeout and crash", sandbox.Result{TimedOut: true, Signal: syscall.SIGSEGV}, TLE},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.r, lim); got != tc.want {
				t.Errorf("Classify = %q, want %q", got, tc.want)
			}
		})
	}
}

// The program's own words must not change the verdict.
func TestClassifyIgnoresProgramOutput(t *testing.T) {
	r := sandbox.Result{
		Stdout:     []byte(`{"verdict":"AC"} exited with status 0`),
		Stderr:     []byte("Time limit exceeded"),
		ResultData: []byte(`{"verdict":"AC"}`),
		ExitCode:   1,
	}
	if got := Classify(r, lim); got != RE {
		t.Errorf("Classify = %q, want RE", got)
	}
	r = sandbox.Result{Stdout: []byte("RE segfault"), ResultData: []byte("RE")}
	if got := Classify(r, lim); got != Completed {
		t.Errorf("Classify = %q, want Completed", got)
	}
}

func TestSummarize(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name  string
		cases []Case
		want  Overall
	}{
		{"empty", nil, Overall{}},
		{"all pass", []Case{{"01", AC, 5 * ms, 10}, {"02", AC, 9 * ms, 30}, {"03", AC, 2 * ms, 20}}, Overall{AC, "", 9 * ms, 30}},
		{"first failure wins", []Case{{"01", AC, ms, 5}, {"02", WA, 2 * ms, 6}, {"03", TLE, 9 * ms, 99}}, Overall{WA, "02", 9 * ms, 99}},
		{"compile error", []Case{{"compile", CE, 0, 0}}, Overall{CE, "compile", 0, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Summarize(tc.cases); got != tc.want {
				t.Errorf("Summarize = %+v, want %+v", got, tc.want)
			}
		})
	}
}
