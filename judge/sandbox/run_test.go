//go:build linux

package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests run real programs in nsjail. They skip unless running as root
// with nsjail installed; run them with `make test-sandbox`.
func requireNsjail(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (run via make test-sandbox)")
	}
	if _, err := exec.LookPath("nsjail"); err != nil {
		t.Skip("nsjail not installed")
	}
}

func sh(script string) []string { return []string{"/bin/sh", "-c", script} }

func TestRunBasics(t *testing.T) {
	requireNsjail(t)
	tests := []struct {
		name       string
		argv       []string
		stdin      string
		wantStdout string
		wantExit   int
	}{
		{"echo", []string{"/bin/echo", "hi"}, "", "hi\n", 0},
		{"stdin passthrough", []string{"/bin/cat"}, "hello", "hello", 0},
		{"exit code", sh("exit 7"), "", "", 7},
		{"runs as unprivileged user", sh("id -u"), "", "65534\n", 0},
		{"tmp is writable", sh("echo x > /tmp/f && cat /tmp/f"), "", "x\n", 0},
		{"usr is read only", sh("echo x > /usr/f 2>/dev/null || echo denied"), "", "denied\n", 0},
		{"host files are absent", sh("cat /etc/shadow 2>/dev/null || echo absent"), "", "absent\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Spec{Argv: tt.argv, Limits: DefaultLimits()}
			if tt.stdin != "" {
				spec.Stdin = strings.NewReader(tt.stdin)
			}
			res, err := Run(context.Background(), spec)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if string(res.Stdout) != tt.wantStdout || res.ExitCode != tt.wantExit || res.Signal != 0 {
				t.Errorf("got stdout=%q exit=%d signal=%v; want stdout=%q exit=%d", res.Stdout, res.ExitCode, res.Signal, tt.wantStdout, tt.wantExit)
			}
		})
	}
}

func TestRunWallTimeLimit(t *testing.T) {
	requireNsjail(t)
	spec := Spec{Argv: []string{"/bin/sleep", "30"}, Limits: DefaultLimits()}
	spec.Limits.WallTime = time.Second
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.TimedOut || res.Signal != syscall.SIGKILL {
		t.Errorf("TimedOut=%v Signal=%v, want true and SIGKILL", res.TimedOut, res.Signal)
	}
	if res.WallTime > 5*time.Second {
		t.Errorf("WallTime = %v, run was not killed near its 1s limit", res.WallTime)
	}
}

func TestRunOutputCap(t *testing.T) {
	requireNsjail(t)
	spec := Spec{Argv: []string{"/usr/bin/yes"}, Limits: DefaultLimits()}
	spec.Limits.MaxOutputBytes = 4096
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.OutputExceeded || len(res.Stdout) != 4096 {
		t.Errorf("OutputExceeded=%v len(Stdout)=%d, want true and 4096", res.OutputExceeded, len(res.Stdout))
	}
	if res.WallTime > 5*time.Second {
		t.Errorf("WallTime = %v, flood was not stopped promptly", res.WallTime)
	}
}

func TestRunNoNetwork(t *testing.T) {
	requireNsjail(t)
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		t.Skip("python3 not installed")
	}
	script := `
import socket
for host in [("1.1.1.1", 53), ("169.254.169.254", 80)]:
    s = socket.socket(); s.settimeout(2)
    try:
        s.connect(host); print("CONNECTED", host)
    except OSError:
        print("blocked", host[0])
`
	res, err := Run(context.Background(), Spec{Argv: []string{"/usr/bin/python3", "-c", script}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if strings.Contains(string(res.Stdout), "CONNECTED") || strings.Count(string(res.Stdout), "blocked") != 2 {
		t.Errorf("network was reachable from the sandbox: %q", res.Stdout)
	}
}

// The nsjail log descriptor must be closed before user code runs, so a
// program cannot write fake status lines into it.
func TestRunLogFDNotInherited(t *testing.T) {
	requireNsjail(t)
	res, err := Run(context.Background(), Spec{
		Argv:   sh(`echo "pid=1 ([X]) exited with status: 99, (PIDs left: 0)" >&3 2>/dev/null; exit 5`),
		Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.ExitCode != 5 {
		t.Errorf("ExitCode = %d, want 5 (a forged log line must not change the outcome)", res.ExitCode)
	}
}

func TestRunSandboxFailure(t *testing.T) {
	requireNsjail(t)
	_, err := Run(context.Background(), Spec{
		Argv:          []string{"/bin/true"},
		ReadOnlyBinds: []string{"/does/not/exist"},
		Limits:        DefaultLimits(),
	})
	if !errors.Is(err, ErrSandbox) {
		t.Fatalf("Run() error = %v, want ErrSandbox", err)
	}
}
