package sandbox

import (
	"fmt"
	"os"
	"strings"
)

// Backend selects the isolation technology a run uses. Both backends honour the
// same Spec and Limits and return the same Result; the host-side facts (exit
// status, signal, time, memory, pids) always come from the host, never from the
// program (ADR 0007).
type Backend string

const (
	// BackendNsjail runs the program in namespaces plus seccomp (ADR 0004). It
	// is the default.
	BackendNsjail Backend = "nsjail"
	// BackendGVisor runs the program on gVisor's user-space kernel (runsc), so
	// the program's syscalls never reach the host kernel directly.
	BackendGVisor Backend = "gvisor"
)

// BackendEnv names the environment variable that selects the backend when a
// Spec does not set one.
const BackendEnv = "LEETFORCE_SANDBOX"

// ParseBackend maps a configuration value to a Backend; empty means nsjail.
func ParseBackend(s string) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(BackendNsjail):
		return BackendNsjail, nil
	case string(BackendGVisor), "runsc":
		return BackendGVisor, nil
	}
	return "", fmt.Errorf("unknown sandbox backend %q (want nsjail or gvisor)", s)
}

// backend resolves the spec's backend: the explicit field, else the environment.
func (s Spec) backend() (Backend, error) {
	if s.Backend != "" {
		return ParseBackend(string(s.Backend))
	}
	return ParseBackend(os.Getenv(BackendEnv))
}
