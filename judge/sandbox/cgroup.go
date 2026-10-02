package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultCgroupRoot is the cgroup v2 directory under which per-run cgroups are
// created. It must be creatable by root and have the memory, pids and cpu
// controllers available from its parent.
var DefaultCgroupRoot = cgroupRootFromEnv()

// CgroupRootEnv overrides DefaultCgroupRoot, so two suites on one host (for
// example a gVisor run beside an nsjail run) do not share a cgroup root.
const CgroupRootEnv = "LEETFORCE_CGROUP_ROOT"

func cgroupRootFromEnv() string {
	if v := os.Getenv(CgroupRootEnv); v != "" {
		return v
	}
	return "/sys/fs/cgroup/leetforce"
}

const (
	cgroupControllers = "+memory +pids +cpu"
	cgroupRemoveWait  = 3 * time.Second
	cgroupPoll        = 5 * time.Millisecond
)

var jobSeq atomic.Uint64

// cgroupJob is one run's cgroup directory. nsjail creates its own child cgroup
// inside it and puts the program there with the memory, pids and cpu limits;
// this directory outlives that child, so its accounting files keep the totals
// (memory.peak, cpu.stat, memory.events, pids.peak) and writing cgroup.kill
// here kills every process the run ever started.
type cgroupJob struct {
	dir string
}

// cgroupStats are host-side measurements read from the job directory.
type cgroupStats struct {
	PeakMemoryBytes uint64
	CPUTime         time.Duration
	OOMKills        uint64
	PeakPIDs        uint64
	PIDLimitHits    uint64
}

func newCgroupJob(root string) (*cgroupJob, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create cgroup root %s: %w", root, err)
	}
	if err := writeFile(filepath.Join(root, "cgroup.subtree_control"), cgroupControllers); err != nil {
		return nil, fmt.Errorf("enable controllers on %s: %w", root, err)
	}
	name := fmt.Sprintf("job-%d-%d", os.Getpid(), jobSeq.Add(1))
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create cgroup %s: %w", dir, err)
	}
	j := &cgroupJob{dir: dir}
	if err := writeFile(filepath.Join(dir, "cgroup.subtree_control"), cgroupControllers); err != nil {
		_ = os.Remove(dir)
		return nil, fmt.Errorf("enable controllers on %s: %w", dir, err)
	}
	return j, nil
}

// kill sends SIGKILL to every process in the job cgroup and its descendants.
func (j *cgroupJob) kill() error {
	err := writeFile(filepath.Join(j.dir, "cgroup.kill"), "1")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (j *cgroupJob) stats() cgroupStats {
	s := cgroupStats{
		PeakMemoryBytes: readUint(filepath.Join(j.dir, "memory.peak")),
		OOMKills:        readKeyed(filepath.Join(j.dir, "memory.events"), "oom_kill"),
		PeakPIDs:        readUint(filepath.Join(j.dir, "pids.peak")),
		PIDLimitHits:    readKeyed(filepath.Join(j.dir, "pids.events"), "max"),
	}
	usec := min(readKeyed(filepath.Join(j.dir, "cpu.stat"), "usage_usec"), uint64(math.MaxInt64/int64(time.Microsecond)))
	s.CPUTime = time.Duration(usec) * time.Microsecond //nolint:gosec // bounded by the min above
	return s
}

// remove kills anything left, waits for the cgroup to empty, and deletes it and
// nsjail's child directories. It returns an error if processes survive.
func (j *cgroupJob) remove() error {
	_ = j.kill()
	deadline := time.Now().Add(cgroupRemoveWait)
	for readKeyed(filepath.Join(j.dir, "cgroup.events"), "populated") != 0 {
		if time.Now().After(deadline) {
			return fmt.Errorf("cgroup %s still has processes after kill", j.dir)
		}
		_ = j.kill()
		time.Sleep(cgroupPoll)
	}
	// A just-emptied cgroup can report EBUSY for a few milliseconds.
	rmDeadline := time.Now().Add(cgroupRemoveWait)
	for {
		err := removeTree(j.dir)
		if err == nil {
			return nil
		}
		if time.Now().After(rmDeadline) {
			return fmt.Errorf("remove cgroup %s: %w", j.dir, err)
		}
		time.Sleep(cgroupPoll)
	}
}

// removeTree deletes nested cgroup directories bottom-up (cgroup directories
// are removed with rmdir, not recursive file deletion).
func removeTree(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := removeTree(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func writeFile(path, value string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0) //nolint:gosec // path is built from our own cgroup directory
	if err != nil {
		return err
	}
	_, werr := f.WriteString(value)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// readUint reads a single-number cgroup file; a missing or unreadable file is 0.
func readUint(path string) uint64 {
	b, err := os.ReadFile(path) //nolint:gosec // path is built from our own cgroup directory
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v
}

// readKeyed reads "key value" lines (memory.events, cpu.stat, ...) and returns
// the value for key, or 0 if the file or key is missing.
func readKeyed(path, key string) uint64 {
	b, err := os.ReadFile(path) //nolint:gosec // path is built from our own cgroup directory
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && k == key {
			n, _ := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
			return n
		}
	}
	return 0
}
