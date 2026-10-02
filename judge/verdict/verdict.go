// Package verdict turns what the host measured about a sandboxed run into a
// verdict. It looks only at the host-side facts in sandbox.Result (exit
// status, signal, CPU time, memory, output cap) and never at what the program
// printed or wrote to the result fd.
package verdict

import (
	"syscall"
	"time"

	"leetforce/judge/sandbox"
)

// Verdict is the outcome of judging one test or a whole submission.
type Verdict string

const (
	AC  Verdict = "AC"  // accepted
	WA  Verdict = "WA"  // wrong answer
	TLE Verdict = "TLE" // time limit exceeded
	MLE Verdict = "MLE" // memory limit exceeded
	RE  Verdict = "RE"  // runtime error (non-zero exit or killed by a signal)
	CE  Verdict = "CE"  // compile error
	OLE Verdict = "OLE" // output limit exceeded

	// Completed is not a verdict: the program ran to a clean exit within its
	// limits, so its output still has to be compared with the expected output.
	Completed Verdict = ""
)

// Limits are the problem's limits for one run.
type Limits struct {
	Time        time.Duration // CPU time limit
	MemoryBytes uint64
}

// Classify decides how a run ended, in this order:
//
//  1. OLE if an output cap was passed (the run was then killed by the host).
//  2. MLE if the kernel OOM-killed the run or peak memory is over the limit.
//  3. TLE if the wall-time limit fired, CPU time is over the limit, or the
//     program got SIGXCPU. CPU-time kills arrive as SIGKILL, so the signal
//     alone cannot tell them apart from a crash; the measured CPU time does.
//  4. RE for any other non-zero exit or signal, including a refused fork or
//     thread (pids limit).
//
// If none apply it returns Completed and the caller compares the output.
func Classify(r sandbox.Result, l Limits) Verdict {
	switch {
	case r.OutputExceeded:
		return OLE
	case r.OOMKilled || r.PeakMemoryBytes > l.MemoryBytes:
		return MLE
	case r.TimedOut || r.CPUTime > l.Time || r.Signal == syscall.SIGXCPU:
		return TLE
	case r.Signal != 0 || r.ExitCode != 0 || r.PIDLimitHit:
		return RE
	}
	return Completed
}

// Case is the outcome of one test.
type Case struct {
	Name    string
	Verdict Verdict
	Time    time.Duration // CPU time
	Memory  uint64        // peak bytes
}

// Overall is the outcome of a submission.
type Overall struct {
	Verdict Verdict
	// Failed is the name of the first failing test, empty for AC. Callers must
	// not show it for hidden tests on Submit.
	Failed string
	Time   time.Duration // largest CPU time over the cases that ran
	Memory uint64        // largest peak memory over the cases that ran
}

// Summarize combines per-test outcomes, given in test order. The overall
// verdict is the first non-AC case, because the judge stops at the first
// failure. A compile error is passed as a single case with verdict CE. No
// cases is not a valid submission, so it returns an empty Overall.
func Summarize(cases []Case) Overall {
	var o Overall
	if len(cases) == 0 {
		return o
	}
	o.Verdict = AC
	for _, c := range cases {
		o.Time = max(o.Time, c.Time)
		o.Memory = max(o.Memory, c.Memory)
		if o.Verdict == AC && c.Verdict != AC {
			o.Verdict = c.Verdict
			o.Failed = c.Name
		}
	}
	return o
}
