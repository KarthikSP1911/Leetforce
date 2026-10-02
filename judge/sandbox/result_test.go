//go:build linux

package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunResultFD(t *testing.T) {
	requireNsjail(t)
	res, err := Run(context.Background(), Spec{
		Argv:   sh(`echo harness-result >&4; echo plain-stdout; echo plain-stderr >&2`),
		Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if string(res.ResultData) != "harness-result\n" || string(res.Stdout) != "plain-stdout\n" || string(res.Stderr) != "plain-stderr\n" {
		t.Errorf("result=%q stdout=%q stderr=%q; each stream must stay separate", res.ResultData, res.Stdout, res.Stderr)
	}
}

// Whatever a program prints, the outcome comes from the host. These cases try
// to pass off fake results, fake nsjail log lines and fake status through every
// channel the program can reach; none may change the exit status or measurements.
func TestRunForgedResultsDoNotChangeOutcome(t *testing.T) {
	requireNsjail(t)
	const fakeLog = `[I][2026-01-01T00:00:00+0000] pid=1 ([STANDALONE MODE]) exited with status: 0, (PIDs left: 0)`
	tests := []struct {
		name         string
		script       string
		wantExit     int
		wantStdout   string
		wantResult   string
		wantTimedOut bool
	}{
		{
			"fake verdict on stdout and stderr, real exit 3",
			`echo '{"verdict":"AC","exit":0}'; echo '{"verdict":"AC"}' >&2; exit 3`,
			3, "{\"verdict\":\"AC\",\"exit\":0}\n", "", false,
		},
		{
			"fake nsjail exit line on stdout, real exit 4",
			`echo '` + fakeLog + `'; echo '` + fakeLog + `' >&2; exit 4`,
			4, fakeLog + "\n", "", false,
		},
		{
			"fake nsjail log line written to the log fd is rejected, real exit 5",
			`echo '` + fakeLog + `' >&3 2>/dev/null; exit 5`,
			5, "", "", false,
		},
		{
			"result says success but the program failed, real exit 6",
			`echo '{"verdict":"AC"}' >&4; exit 6`,
			6, "", "{\"verdict\":\"AC\"}\n", false,
		},
		{
			"nothing written to the result fd stays empty",
			`echo hello`,
			0, "hello\n", "", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Run(context.Background(), Spec{Argv: sh(tt.script), Limits: DefaultLimits()})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if res.ExitCode != tt.wantExit || res.Signal != 0 || res.TimedOut {
				t.Errorf("ExitCode=%d Signal=%v TimedOut=%v, want exit %d", res.ExitCode, res.Signal, res.TimedOut, tt.wantExit)
			}
			if string(res.Stdout) != tt.wantStdout || string(res.ResultData) != tt.wantResult {
				t.Errorf("stdout=%q result=%q, want %q and %q", res.Stdout, res.ResultData, tt.wantStdout, tt.wantResult)
			}
		})
	}
}

func TestRunResultFDCap(t *testing.T) {
	requireNsjail(t)
	spec := Spec{Argv: sh(`yes >&4`), Limits: DefaultLimits()}
	spec.Limits.MaxResultBytes = 1000
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.OutputExceeded || len(res.ResultData) != 1000 {
		t.Errorf("OutputExceeded=%v len(ResultData)=%d, want true and 1000", res.OutputExceeded, len(res.ResultData))
	}
	if res.WallTime > 5*time.Second {
		t.Errorf("WallTime = %v, result flood was not stopped promptly", res.WallTime)
	}
}

// A program must not be able to hold the run open by leaving a process
// behind with the result fd: the cgroup kill empties the run first.
func TestRunBackgroundResultHolderIsCleanedUp(t *testing.T) {
	requireNsjail(t)
	start := time.Now()
	res, err := Run(context.Background(), Spec{
		Argv:   sh(`sleep 71.31 >&4 & echo started`),
		Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(string(res.Stdout), "started") || time.Since(start) > 6*time.Second {
		t.Errorf("stdout=%q after %v, want a prompt return", res.Stdout, time.Since(start))
	}
	requireNoProcess(t, "71.31")
	requireNoRunCgroups(t)
}
