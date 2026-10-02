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
	maxLogBytes  = 256 << 10
	logDrainWait = 2 * time.Second
)

// ErrSandbox means nsjail could not set up or finish the run. It is a host
// problem, not a verdict about the user's program.
var ErrSandbox = errors.New("sandbox failure")

// Run executes the spec inside nsjail and returns what the host observed. It
// must run as root (see ADR 0003). The caller's ctx can cancel the run; the
// spec's wall-time limit is enforced separately. Every run gets its own cgroup,
// which is emptied with cgroup.kill and removed before Run returns; if it
// cannot be emptied, Run fails with ErrSandbox instead of leaking processes.
func Run(ctx context.Context, spec Spec) (res *Result, err error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	root := spec.CgroupRoot
	if root == "" {
		root = DefaultCgroupRoot
	}
	job, err := newCgroupJob(root)
	if err != nil {
		return nil, fmt.Errorf("prepare run cgroup: %w", err)
	}
	defer func() {
		if rerr := job.remove(); rerr != nil {
			res = nil
			err = errors.Join(err, fmt.Errorf("%w: %w", ErrSandbox, rerr))
		}
	}()

	if b, _ := spec.backend(); b == BackendGVisor { // Validate rejected unknown values
		res, err = runGVisor(ctx, spec, job)
	} else {
		res, err = runJob(ctx, spec, job)
	}
	if res != nil {
		s := job.stats()
		res.PeakMemoryBytes = s.PeakMemoryBytes
		res.CPUTime = s.CPUTime
		res.OOMKilled = s.OOMKills > 0
		res.PeakPIDs = s.PeakPIDs
		res.PIDLimitHit = s.PIDLimitHits > 0
		res.RawPeakMemoryBytes, res.RawCPUTime, res.RawPeakPIDs = res.PeakMemoryBytes, res.CPUTime, res.PeakPIDs
		if b, _ := spec.backend(); b == BackendGVisor {
			discountGVisorOverhead(res)
		}
	}
	return res, err
}

func runJob(ctx context.Context, spec Spec, job *cgroupJob) (*Result, error) {
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

	resR, resW, err := os.Pipe()
	if err != nil {
		_ = logW.Close()
		return nil, fmt.Errorf("create result pipe: %w", err)
	}
	defer func() { _ = resR.Close() }()
	resultBuf := newCappedBuffer(spec.Limits.MaxResultBytes, cancel)

	// The nsjail path is configuration and the arguments are built by
	// nsjailArgs from a validated Spec, never from shell text.
	cmd := exec.CommandContext(runCtx, bin, spec.nsjailArgs(job.dir)...) //nolint:gosec // see comment above
	cmd.Stdin = spec.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// ExtraFiles[0] becomes fd 3 (nsjailLogFD) and [1] fd 4 (ResultFD) in nsjail.
	cmd.ExtraFiles = []*os.File{logW, resW}
	cmd.Cancel = func() error {
		_ = job.kill() // the whole run, not just nsjail
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = killDelay

	logBuf := newCappedBuffer(maxLogBytes, nil)
	logDone := copyAsync(logBuf, logR)
	resultDone := copyAsync(resultBuf, resR)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		_ = logW.Close()
		_ = resW.Close()
		return nil, fmt.Errorf("start nsjail: %w", err)
	}
	// The child holds the only write ends now, so the readers see EOF when
	// nsjail and everything inside it are gone.
	_ = logW.Close()
	_ = resW.Close()
	waitErr := cmd.Wait()
	wall := time.Since(start)

	drain(logDone, logR)
	drain(resultDone, resR)

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
		ResultData:     resultBuf.Bytes(),
		ExitCode:       term.exitCode,
		Signal:         term.signal,
		TimedOut:       term.timedOut || errors.Is(runCtx.Err(), context.DeadlineExceeded),
		OutputExceeded: stdout.Exceeded() || stderr.Exceeded() || resultBuf.Exceeded(),
		WallTime:       wall,
	}, nil
}

// copyAsync copies r into dst until EOF in the background.
func copyAsync(dst io.Writer, r io.Reader) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(dst, r)
	}()
	return done
}

// drain waits for a copyAsync to finish; if a stray holder of the write end
// keeps the pipe open, it closes the read end after logDrainWait to unblock it.
func drain(done <-chan struct{}, r *os.File) {
	select {
	case <-done:
	case <-time.After(logDrainWait):
		_ = r.Close()
		<-done
	}
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
