package sandbox

import (
	"errors"
	"io"
	"syscall"
	"time"
)

// Limits are the per-run resource limits. Memory and process-count limits are
// enforced by cgroup v2 (added separately); everything here is enforced by
// nsjail rlimits, its time limit, and the output caps in this package.
type Limits struct {
	WallTime       time.Duration // real time before the run is killed
	CPUTime        time.Duration // CPU time before SIGXCPU (RLIMIT_CPU)
	MaxFileBytes   uint64        // largest file the program may write (RLIMIT_FSIZE)
	MaxOpenFiles   uint64        // RLIMIT_NOFILE
	MaxOutputBytes int64         // cap per stream (stdout, stderr); excess is discarded and the run is killed
	TmpfsBytes     uint64        // size of the writable /tmp
}

// DefaultLimits returns conservative limits suitable for tests and as a base
// for callers to override.
func DefaultLimits() Limits {
	return Limits{
		WallTime:       10 * time.Second,
		CPUTime:        10 * time.Second,
		MaxFileBytes:   8 << 20,
		MaxOpenFiles:   64,
		MaxOutputBytes: 1 << 20,
		TmpfsBytes:     64 << 20,
	}
}

// Spec describes one sandboxed run.
type Spec struct {
	// Argv is the program and its arguments, as seen inside the sandbox.
	Argv []string
	// Env is extra environment as "KEY=value". The sandbox starts with an
	// empty environment plus PATH and HOME defaults.
	Env []string
	// Stdin is fed to the program; nil means empty stdin.
	Stdin io.Reader
	// ReadOnlyBinds are host paths bind-mounted read-only at the same path,
	// in addition to the system directories (/usr, /lib, /lib64, /bin).
	ReadOnlyBinds []string
	Limits        Limits
	// NsjailPath overrides the nsjail binary; empty means "nsjail" from PATH.
	NsjailPath string
}

// Validate reports whether the spec can be run.
func (s Spec) Validate() error {
	if len(s.Argv) == 0 || s.Argv[0] == "" {
		return errors.New("spec: argv is empty")
	}
	l := s.Limits
	if l.WallTime <= 0 {
		return errors.New("spec: wall time limit must be positive")
	}
	if l.CPUTime <= 0 {
		return errors.New("spec: cpu time limit must be positive")
	}
	if l.MaxOutputBytes <= 0 {
		return errors.New("spec: output limit must be positive")
	}
	return nil
}

// Result is what the host observed about a run. Exit status and signal come
// from nsjail's own log (a separate file descriptor user code cannot write
// to), never from the program's stdout or stderr.
type Result struct {
	Stdout []byte
	Stderr []byte
	// ExitCode is the program's exit status, or -1 if it was killed by a signal.
	ExitCode int
	// Signal is the terminating signal, or 0 if the program exited normally.
	Signal syscall.Signal
	// TimedOut is true when the wall-time limit killed the run.
	TimedOut bool
	// OutputExceeded is true when stdout or stderr passed MaxOutputBytes.
	OutputExceeded bool
	WallTime       time.Duration
}
