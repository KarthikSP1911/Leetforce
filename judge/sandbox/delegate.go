package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	cgroupMount = "/sys/fs/cgroup"
	// leafName is the cgroup the runner process itself lives in. cgroup v2 does
	// not let a cgroup with processes delegate controllers to children, so the
	// per-run cgroups are siblings of this leaf under the delegated parent.
	leafName = "runner"
)

// Rootless reports whether the process lacks root. Then nsjail maps the
// program to the caller's own uid and gid (an unprivileged process may map
// only itself) and the cgroup root must be a delegated cgroup.
func Rootless() bool { return os.Geteuid() != 0 }

// PrepareDelegatedRoot returns a cgroup v2 directory an unprivileged runner
// can create per-run cgroups in. It requires the runner to start in a cgroup
// delegated to its user (systemd Delegate=yes). The runner moves itself into
// a leaf child (unless systemd already did, with DelegateSubgroup=runner),
// enables the memory, pids and cpu controllers on the delegated parent and
// returns the jobs directory below it.
func PrepareDelegatedRoot() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", fmt.Errorf("read own cgroup: %w", err)
	}
	rel, ok := bytes.CutPrefix(bytes.TrimSpace(data), []byte("0::"))
	if !ok {
		return "", fmt.Errorf("own cgroup %q is not cgroup v2", bytes.TrimSpace(data))
	}
	cur := filepath.Join(cgroupMount, string(rel))
	parent := cur
	if filepath.Base(cur) == leafName {
		parent = filepath.Dir(cur)
	} else {
		leaf := filepath.Join(cur, leafName)
		if err := os.MkdirAll(leaf, 0o750); err != nil { //nolint:gosec // leaf is derived from our own delegated cgroup path, not user input
			return "", fmt.Errorf("create runner leaf cgroup (is the cgroup delegated?): %w", err)
		}
		if err := writeFile(filepath.Join(leaf, "cgroup.procs"), strconv.Itoa(os.Getpid())); err != nil {
			return "", fmt.Errorf("move runner into %s: %w", leaf, err)
		}
	}
	if err := writeFile(filepath.Join(parent, "cgroup.subtree_control"), cgroupControllers); err != nil {
		return "", fmt.Errorf("enable controllers on %s (need Delegate=memory pids cpu): %w", parent, err)
	}
	return filepath.Join(parent, "jobs"), nil
}
