package sandbox

import (
	"errors"
	"fmt"
	"io"
	"syscall"
	"time"
)

// Limits are the per-run resource limits. Memory, process-count and CPU-quota
// limits are enforced by cgroup v2 (see cgroup.go); the rest by nsjail rlimits,
// its time limit, and the output caps in this package.
type Limits struct {
	WallTime       time.Duration // real time before the run is killed
	CPUTime        time.Duration // CPU time before SIGXCPU (RLIMIT_CPU)
	MaxFileBytes   uint64        // largest file the program may write (RLIMIT_FSIZE)
	MaxOpenFiles   uint64        // RLIMIT_NOFILE
	MaxOutputBytes int64         // cap per stream (stdout, stderr); excess is discarded and the run is killed
	MaxResultBytes int64         // cap on the harness result fd; excess is discarded and the run is killed
	TmpfsBytes     uint64        // size of the writable /tmp
	MemoryBytes    uint64        // cgroup memory.max for the program and its children; swap is disabled
	MaxPIDs        uint64        // cgroup pids.max: processes and threads
	CPUMilliPerSec uint64        // CPU quota in ms per second (1000 = one full CPU)
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
		MaxResultBytes: 1 << 20,
		TmpfsBytes:     64 << 20,
		MemoryBytes:    256 << 20,
		MaxPIDs:        64,
		CPUMilliPerSec: 1000,
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
	// CgroupRoot is the cgroup v2 directory per-run cgroups are created in;
	// empty means DefaultCgroupRoot.
	CgroupRoot string
	// Backend picks the isolation technology; empty means the LEETFORCE_SANDBOX
	// environment variable, which defaults to nsjail.
	Backend Backend
	// RunscPath overrides the runsc binary used by the gVisor backend; empty
	// means "runsc" from PATH.
	RunscPath string
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
	if l.MaxResultBytes <= 0 {
		return errors.New("spec: result limit must be positive")
	}
	if l.MemoryBytes == 0 {
		return errors.New("spec: memory limit must be positive")
	}
	if l.MaxPIDs == 0 {
		return errors.New("spec: process limit must be positive")
	}
	if _, err := s.backend(); err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	return nil
}

// Result is what the host observed about a run. Exit status and signal come
// from nsjail's own log (a separate file descriptor user code cannot write
// to), never from the program's stdout or stderr.
type Result struct {
	Stdout []byte
	Stderr []byte
	// ResultData is what the program wrote to ResultFD (fd 4). It is data from
	// inside the sandbox, never a verdict: exit status, signal, time and memory
	// below come from the host and are the only facts to trust about the run.
	ResultData []byte
	// ExitCode is the program's exit status, or -1 if it was killed by a signal.
	ExitCode int
	// Signal is the terminating signal, or 0 if the program exited normally.
	Signal syscall.Signal
	// TimedOut is true when the wall-time limit killed the run.
	TimedOut bool
	// OutputExceeded is true when stdout or stderr passed MaxOutputBytes.
	OutputExceeded bool // stdout, stderr or the result fd passed its cap
	WallTime       time.Duration

	// Host-side measurements, read from the run's cgroup.
	PeakMemoryBytes uint64        // highest memory use of the run (memory.peak)
	CPUTime         time.Duration // total CPU time used (cpu.stat usage_usec)
	OOMKilled       bool          // the kernel OOM-killed something in the run
	PeakPIDs        uint64        // most processes/threads alive at once
	PIDLimitHit     bool          // a fork or thread creation was refused by pids.max

	// The same three measurements before the gVisor backend discounts its own
	// fixed cost (see discountGVisorOverhead); equal to the fields above for nsjail.
	RawPeakMemoryBytes uint64
	RawCPUTime         time.Duration
	RawPeakPIDs        uint64
}
