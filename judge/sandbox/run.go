package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	// wallGrace is how long past the wall limit we wait for nsjail to enforce
	// its own time limit before the context kills it.
	wallGrace = 2 * time.Second
	// killDelay is how long after SIGTERM before nsjail is force-killed.
	killDelay = 2 * time.Second
	// maxLogBytes caps how much of nsjail's log is kept.
	maxLogBytes = 256 << 10
	logDrainWait = 2 * time.Second
)

// ErrSandbox means nsjail could not set up or finish the run. It is a host
// problem, not a verdict about the user's program.
var ErrSandbox = errors.New("sandbox failure")

// Run executes the spec inside nsjail and returns what the host observed. It
// must run as root (see ADR 0003). The caller's ctx can cancel the run; the
// spec's wall-time limit is enforced separately.
func Run(ctx context.Context, spec Spec) (*Result, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	bin := spec.NsjailPath
	if bin == "" {
		bin = "nsjail"
	}

	runCtx, cancel := context.WithTimeout(ctx, spec.Limits.WallTime+wallGrace)
	defer cancel()

	stdout := newCappedBuffer(spec.Limits.MaxOutputBytes, cancel)
	stderr := newCappedBuffer(spec.Limits.MaxOutputBytes, cancel)

	logR, logW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create nsjail log pipe: %w", err)
	}
	defer func() { _ = logR.Close() }()

	// The nsjail path is configuration and the arguments are built by
	// nsjailArgs from a validated Spec, never from shell text.
	cmd := exec.CommandContext(runCtx, bin, spec.nsjailArgs()...) //nolint:gosec // see comment above
	cmd.Stdin = spec.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// ExtraFiles[0] becomes fd 3 in nsjail, matching nsjailLogFD.
	cmd.ExtraFiles = []*os.File{logW}
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = killDelay

	logBuf := newCappedBuffer(maxLogBytes, nil)
	logDone := make(chan struct{})
	go func() {
		defer close(logDone)
		_, _ = io.Copy(logBuf, logR)
	}()

	start := time.Now()
	if err := cmd.Start(); err != nil {
		_ = logW.Close()
		return nil, fmt.Errorf("start nsjail: %w", err)
	}
	_ = logW.Close() // the child holds the only write end now
	waitErr := cmd.Wait()
	wall := time.Since(start)

	select {
	case <-logDone:
	case <-time.After(logDrainWait):
		_ = logR.Close()
		<-logDone
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("run sandbox: %w", err)
	}

	term := parseLog(string(logBuf.Bytes()))
	// If nsjail never reached "Executing", the program did not run, whatever
	// exit status or signal it logged while tearing the jail down.
	if !term.started {
		detail := strings.Join(tail(term.failure, 5), "; ")
		if detail == "" {
			detail = lastLines(logBuf.Bytes(), 3)
		}
		return nil, fmt.Errorf("%w: program did not start: %s", ErrSandbox, detail)
	}
	if !term.reported {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) || waitErr == nil {
			return nil, fmt.Errorf("%w: nsjail reported no exit status: %s", ErrSandbox, lastLines(logBuf.Bytes(), 3))
		}
		return nil, fmt.Errorf("wait for nsjail: %w", waitErr)
	}

	return &Result{
		Stdout:         stdout.Bytes(),
		Stderr:         stderr.Bytes(),
		ExitCode:       term.exitCode,
		Signal:         term.signal,
		TimedOut:       term.timedOut || errors.Is(runCtx.Err(), context.DeadlineExceeded),
		OutputExceeded: stdout.Exceeded() || stderr.Exceeded(),
		WallTime:       wall,
	}, nil
}

func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func lastLines(b []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(bytes.TrimSpace(b))), "\n")
	return strings.Join(tail(lines, n), " | ")
}
