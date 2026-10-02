package sandbox

import (
	"strings"
	"testing"
)

func TestSeccompPolicyShape(t *testing.T) {
	policy := seccompPolicy()
	tests := []struct {
		name string
		want string
	}{
		{"default allow", "DEFAULT ALLOW"},
		{"ptrace denied", "ptrace"},
		{"new mount api denied", "fsmount"},
		{"io_uring denied", "io_uring_setup"},
		{"namespace clone flags denied", "clone(flags) { (flags & " + namespaceCloneFlags + ") != 0 }"},
		{"clone3 answered ENOSYS", "ERRNO(38) { clone3 }"},
		{"socket families restricted", "socket(domain) { domain != 1 && domain != 2 && domain != 10 }"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(policy, tt.want) {
				t.Errorf("policy missing %q:\n%s", tt.want, policy)
			}
		})
	}
	// Syscalls every runtime needs must never be denied (see seccomp.go).
	for _, needed := range []string{"fork", "vfork", "execve", "futex", "prctl", "flock", "pidfd_open", "wait4", "epoll_wait"} {
		for _, d := range denied {
			if d == needed {
				t.Errorf("%s is needed by a supported runtime but is on the denylist", needed)
			}
		}
	}
}
