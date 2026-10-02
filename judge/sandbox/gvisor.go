package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// runscLogFD is the descriptor runsc writes its own log to. It is a
	// separate pipe, so a failure to start is never confused with program
	// output; it is not passed into the sandbox.
	runscLogFD = 3
	// runscResultHostFD is the host-side fd of the result pipe (ExtraFiles[1]).
	runscResultHostFD = 4

	// runscStartFailExit is the exit status runsc uses when it could not start
	// the container (as opposed to the program's own status).
	runscStartFailExit = 128

	// gVisor's runsc, sentry and gofer run in the same cgroup as the program,
	// so the host sees their cost too. Measured on the dev host (ADR 0013): a
	// hello-world run peaks at about 18 MiB and 36-38 pids (host threads and
	// processes) before the program does anything, and touching 64 MiB costs
	// about 16 MiB more. The baselines are subtracted from the reported peaks
	// so limits and verdicts mean "the program"; the slack on top is how far the
	// host limit sits above the program limit before the kernel kills the run.
	gvisorMemBaseline = 16 << 20
	gvisorMemSlack    = 6 << 20
	gvisorPIDBaseline = 38
	gvisorPIDSlack    = 0
	// A hello-world run spends about 110 ms of CPU in runsc and the sentry.
	gvisorCPUBaseline = 100 * time.Millisecond
)

// discountGVisorOverhead removes the fixed sentry cost from the host-side
// peaks and CPU time, so a program near its limit is not charged for the sandbox itself.
func discountGVisorOverhead(r *Result) {
	r.PeakMemoryBytes -= min(r.PeakMemoryBytes, gvisorMemBaseline)
	r.PeakPIDs -= min(r.PeakPIDs, gvisorPIDBaseline)
	r.CPUTime -= min(r.CPUTime, gvisorCPUBaseline)
}

var gvisorSeq atomic.Uint64

// ociSpec is the part of the OCI runtime spec the gVisor backend writes. Only
// fields that matter for isolation are present; everything else keeps runsc's
// defaults.
type ociSpec struct {
	Version  string     `json:"ociVersion"`
	Process  ociProcess `json:"process"`
	Root     ociRoot    `json:"root"`
	Hostname string     `json:"hostname"`
	Mounts   []ociMount `json:"mounts"`
	Linux    ociLinux   `json:"linux"`
}

type ociProcess struct {
	Terminal        bool        `json:"terminal"`
	User            ociUser     `json:"user"`
	Args            []string    `json:"args"`
	Env             []string    `json:"env"`
	Cwd             string      `json:"cwd"`
	Rlimits         []ociRlimit `json:"rlimits"`
	NoNewPrivileges bool        `json:"noNewPrivileges"`
	Capabilities    ociCaps     `json:"capabilities"`
}

type ociUser struct {
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

type ociCaps struct {
	Bounding    []string `json:"bounding"`
	Effective   []string `json:"effective"`
	Inheritable []string `json:"inheritable"`
	Permitted   []string `json:"permitted"`
	Ambient     []string `json:"ambient"`
}

type ociRlimit struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

type ociRoot struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly"`
}

type ociMount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Source      string   `json:"source"`
	Options     []string `json:"options,omitempty"`
}

type ociLinux struct {
	Namespaces []ociNamespace `json:"namespaces"`
	Resources  *ociResources  `json:"resources,omitempty"`
}

type ociNamespace struct {
	Type string `json:"type"`
}

type ociResources struct {
	Pids   *ociPids   `json:"pids,omitempty"`
	Memory *ociMemory `json:"memory,omitempty"`
}

type ociPids struct {
	Limit int64 `json:"limit"`
}

type ociMemory struct {
	Limit int64 `json:"limit"`
	Swap  int64 `json:"swap"`
}

// gvisorSpec builds the OCI spec for a run. The root file system is an empty
// read-only directory; the same read-only system directories as the nsjail
// backend and a size-limited tmpfs /tmp are mounted over it.
func (s Spec) gvisorSpec(rootfs string) ociSpec {
	l := s.Limits
	mounts := make([]ociMount, 0, len(systemBinds)+len(s.ReadOnlyBinds)+1)
	for _, p := range append(append([]string(nil), systemBinds...), s.ReadOnlyBinds...) {
		mounts = append(mounts, ociMount{Destination: p, Type: "bind", Source: p, Options: []string{"rbind", "ro"}})
	}
	if l.TmpfsBytes > 0 {
		mounts = append(mounts, ociMount{
			Destination: jailTmp, Type: "tmpfs", Source: "tmpfs",
			Options: []string{"size=" + strconv.FormatUint(l.TmpfsBytes, 10), "mode=1777"},
		})
	}
	openFiles := orDefault(l.MaxOpenFiles, 64)
	cpuSecs := uint64(ceilSeconds(l.CPUTime)) //nolint:gosec // ceilSeconds returns at least 1
	return ociSpec{
		Version: "1.0.0",
		Process: ociProcess{
			User:            ociUser{UID: jailUID, GID: jailUID},
			Args:            s.Argv,
			Env:             s.environment(),
			Cwd:             jailTmp,
			NoNewPrivileges: true,
			Capabilities:    ociCaps{},
			Rlimits: []ociRlimit{
				{"RLIMIT_CPU", cpuSecs, cpuSecs},
				{"RLIMIT_FSIZE", l.MaxFileBytes, l.MaxFileBytes},
				{"RLIMIT_NOFILE", openFiles, openFiles},
				{"RLIMIT_CORE", 0, 0},
			},
		},
		Root:     ociRoot{Path: rootfs, Readonly: true},
		Hostname: "sandbox",
		Mounts:   mounts,
		Linux: ociLinux{
			Namespaces: []ociNamespace{{"pid"}, {"network"}, {"ipc"}, {"uts"}, {"mount"}},
			Resources: &ociResources{
				Pids:   &ociPids{Limit: int64(min(l.MaxPIDs, 1<<31))},                                               //nolint:gosec // bounded
				Memory: &ociMemory{Limit: int64(min(l.MemoryBytes, 1<<62)), Swap: int64(min(l.MemoryBytes, 1<<62))}, //nolint:gosec // bounded
			},
		},
	}
}

// runscArgs are the global runsc flags: no network, no overlay (the root is
// read-only and everything writable is the tmpfs), no host cgroup handling (the
// job cgroup is set up by this package, as for nsjail).
func runscArgs(stateDir string) []string {
	return []string{
		"--root", stateDir,
		"--network=none",
		"--overlay2=none",
		"--ignore-cgroups",
		"--platform=systrap",
		"--log-fd", strconv.Itoa(runscLogFD),
	}
}

// prepareLimits applies the memory, pids and cpu limits to the job cgroup
// itself and creates the "run" child cgroup the runsc process tree starts in.
// The limit sits on the parent, so the child (where processes live) is covered
// and the job directory keeps the accounting files, as in the nsjail backend.
func (j *cgroupJob) prepareGVisor(l Limits) (string, error) {
	cpuMilli := orDefault(l.CPUMilliPerSec, 1000)
	files := []struct{ name, value string }{
		{"memory.max", strconv.FormatUint(l.MemoryBytes+gvisorMemBaseline+gvisorMemSlack, 10)},
		{"memory.swap.max", "0"},
		{"pids.max", strconv.FormatUint(l.MaxPIDs+gvisorPIDBaseline+gvisorPIDSlack, 10)},
		{"cpu.max", fmt.Sprintf("%d 100000", cpuMilli*100)},
	}
	for _, f := range files {
		if err := writeFile(filepath.Join(j.dir, f.name), f.value); err != nil {
			return "", fmt.Errorf("set %s: %w", f.name, err)
		}
	}
	run := filepath.Join(j.dir, "run")
	if err := os.Mkdir(run, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", run, err)
	}
	return run, nil
}

// runGVisor runs the spec under runsc inside the job cgroup. It mirrors runJob:
// stdout and stderr and the result fd (guest fd 4) are separate capped streams,
// a timeout or cap breach kills the whole job cgroup, and exit status comes from
// runsc, never from the program's output.
func runGVisor(ctx context.Context, spec Spec, job *cgroupJob) (*Result, error) {
	bin := spec.RunscPath
	if bin == "" {
		bin = "runsc"
	}

	work, err := os.MkdirTemp("", "lf-gvisor-")
	if err != nil {
		return nil, fmt.Errorf("create gvisor work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	rootfs := filepath.Join(work, "rootfs")
	stateDir := filepath.Join(work, "state")
	for _, d := range append([]string{rootfs, stateDir}, mountPoints(rootfs, spec)...) {
		if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // empty mount points under a private 0700 dir
			return nil, fmt.Errorf("create %s: %w", d, err)
		}
	}
	cfg, err := json.Marshal(spec.gvisorSpec("rootfs"))
	if err != nil {
		return nil, fmt.Errorf("encode oci spec: %w", err)
	}
	if err := os.WriteFile(filepath.Join(work, "config.json"), cfg, 0o600); err != nil {
		return nil, fmt.Errorf("write oci spec: %w", err)
	}

	runCgroup, err := job.prepareGVisor(spec.Limits)
	if err != nil {
		return nil, err
	}
	cgFile, err := os.Open(runCgroup)
	if err != nil {
		return nil, fmt.Errorf("open run cgroup: %w", err)
	}
	defer func() { _ = cgFile.Close() }()

	// The wall limit is whole seconds, as for nsjail's --time_limit.
	wall := time.Duration(ceilSeconds(spec.Limits.WallTime)) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, wall)
	defer cancel()

	stdout := newCappedBuffer(spec.Limits.MaxOutputBytes, cancel)
	stderr := newCappedBuffer(spec.Limits.MaxOutputBytes, cancel)

	logR, logW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create runsc log pipe: %w", err)
	}
	defer func() { _ = logR.Close() }()
	resR, resW, err := os.Pipe()
	if err != nil {
		_ = logW.Close()
		return nil, fmt.Errorf("create result pipe: %w", err)
	}
	defer func() { _ = resR.Close() }()
	resultBuf := newCappedBuffer(spec.Limits.MaxResultBytes, cancel)

	id := fmt.Sprintf("lf-%d-%d", os.Getpid(), gvisorSeq.Add(1))
	args := append(runscArgs(stateDir), "run", "--bundle", work,
		"--pass-fd", fmt.Sprintf("%d:%d", runscResultHostFD, ResultFD), id)

	// bin is configuration and args are built from a validated Spec.
	cmd := exec.CommandContext(runCtx, bin, args...) //nolint:gosec // see comment above
	cmd.Dir = work
	cmd.Stdin = spec.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// ExtraFiles[0] is fd 3 (runscLogFD), [1] is fd 4 (runscResultHostFD).
	cmd.ExtraFiles = []*os.File{logW, resW}
	// Starting runsc inside the run cgroup puts its sentry, gofer and every
	// guest process on the host side in the same cgroup from the first instant.
	cmd.SysProcAttr = &syscall.SysProcAttr{UseCgroupFD: true, CgroupFD: int(cgFile.Fd())}
	cmd.Cancel = func() error {
		_ = job.kill() // the whole run, not just runsc
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
		return nil, fmt.Errorf("start runsc: %w", err)
	}
	_ = logW.Close()
	_ = resW.Close()
	waitErr := cmd.Wait()
	elapsed := time.Since(start)

	_ = job.kill()
	deleteRunsc(bin, stateDir, id)
	drain(logDone, logR)
	drain(resultDone, resR)

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("run sandbox: %w", err)
	}

	code, sig := 0, syscall.Signal(0)
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil:
	case errors.As(waitErr, &exitErr):
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			code, sig = -1, ws.Signal() // runsc itself was killed: the cgroup kill or the OOM killer
		} else {
			code = exitErr.ExitCode()
		}
	default:
		return nil, fmt.Errorf("wait for runsc: %w", waitErr)
	}
	// runsc reports a program killed by signal N as exit status 128+N.
	if code > 128 && code <= 128+64 {
		code, sig = -1, syscall.Signal(code-128)
	}
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	if code == runscStartFailExit && len(logBuf.Bytes()) > 0 && !timedOut {
		return nil, fmt.Errorf("%w: program did not start: %s", ErrSandbox, lastLines(logBuf.Bytes(), 3))
	}

	return &Result{
		Stdout:         stdout.Bytes(),
		Stderr:         stderr.Bytes(),
		ResultData:     resultBuf.Bytes(),
		ExitCode:       code,
		Signal:         sig,
		TimedOut:       timedOut,
		OutputExceeded: stdout.Exceeded() || stderr.Exceeded() || resultBuf.Exceeded(),
		WallTime:       elapsed,
	}, nil
}

// mountPoints lists the empty directories that must exist under the read-only
// root for every mount destination.
func mountPoints(rootfs string, spec Spec) []string {
	var dirs []string
	for _, p := range append(append([]string(nil), systemBinds...), spec.ReadOnlyBinds...) {
		dirs = append(dirs, filepath.Join(rootfs, filepath.Clean("/"+strings.TrimPrefix(p, "/"))))
	}
	return append(dirs, filepath.Join(rootfs, jailTmp), filepath.Join(rootfs, "dev"))
}

// deleteRunsc removes runsc's state after the run. With --network=none runsc
// bind-mounts an empty network namespace file at <state>/null-netns on the host
// and never unmounts it; left alone, every run would leak a mount and keep a
// network namespace alive, so it is detached here before the caller removes the
// work directory.
func deleteRunsc(bin, stateDir, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, bin, "--root", stateDir, "delete", "--force", id).Run() //nolint:gosec // fixed arguments
	_ = syscall.Unmount(filepath.Join(stateDir, "null-netns"), syscall.MNT_DETACH)       // EINVAL if never mounted
}
