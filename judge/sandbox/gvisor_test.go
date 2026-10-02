//go:build linux

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseBackend(t *testing.T) {
	tests := []struct {
		in      string
		want    Backend
		wantErr bool
	}{
		{"", BackendNsjail, false},
		{"nsjail", BackendNsjail, false},
		{" GVisor ", BackendGVisor, false},
		{"runsc", BackendGVisor, false},
		{"docker", "", true},
	}
	for _, tt := range tests {
		got, err := ParseBackend(tt.in)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("ParseBackend(%q) = %q, %v; want %q, error=%v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestSpecBackendFromEnv(t *testing.T) {
	spec := Spec{Argv: []string{"/bin/true"}, Limits: DefaultLimits()}
	t.Setenv(BackendEnv, "gvisor")
	if b, err := spec.backend(); err != nil || b != BackendGVisor {
		t.Errorf("env gvisor: got %q, %v", b, err)
	}
	spec.Backend = BackendNsjail
	if b, err := spec.backend(); err != nil || b != BackendNsjail {
		t.Errorf("explicit field must beat the environment: got %q, %v", b, err)
	}
	spec.Backend = "bogus"
	if err := spec.Validate(); err == nil {
		t.Error("Validate accepted an unknown backend")
	}
}

func TestGVisorSpec(t *testing.T) {
	l := DefaultLimits()
	l.MemoryBytes = 128 << 20
	l.MaxPIDs = 20
	spec := Spec{Argv: []string{"/usr/bin/prog", "x"}, Env: []string{"A=b"}, ReadOnlyBinds: []string{"/var/tmp/job"}, Limits: l}
	oci := spec.gvisorSpec("rootfs")

	if !oci.Root.Readonly || oci.Process.NoNewPrivileges != true {
		t.Errorf("root must be read-only with no new privileges: %+v", oci)
	}
	if oci.Process.User.UID != jailUID || oci.Process.User.GID != jailUID {
		t.Errorf("must run as the unprivileged jail user, got %+v", oci.Process.User)
	}
	dests := map[string]ociMount{}
	for _, m := range oci.Mounts {
		dests[m.Destination] = m
	}
	for _, p := range append([]string{"/var/tmp/job"}, systemBinds...) {
		m, ok := dests[p]
		if !ok || m.Type != "bind" || !contains(m.Options, "ro") {
			t.Errorf("%s must be a read-only bind mount, got %+v", p, m)
		}
	}
	if m := dests[jailTmp]; m.Type != "tmpfs" || !contains(m.Options, "size=67108864") {
		t.Errorf("/tmp must be a size-limited tmpfs, got %+v", m)
	}
	for _, ns := range []string{"pid", "network", "ipc", "uts", "mount"} {
		found := false
		for _, n := range oci.Linux.Namespaces {
			found = found || n.Type == ns
		}
		if !found {
			t.Errorf("missing %s namespace", ns)
		}
	}
	rl := map[string]uint64{}
	for _, r := range oci.Process.Rlimits {
		rl[r.Type] = r.Hard
		if r.Hard != r.Soft {
			t.Errorf("%s soft %d != hard %d: the program could raise it", r.Type, r.Soft, r.Hard)
		}
	}
	if rl["RLIMIT_CPU"] != 10 || rl["RLIMIT_NOFILE"] != 64 || rl["RLIMIT_CORE"] != 0 || rl["RLIMIT_FSIZE"] != l.MaxFileBytes {
		t.Errorf("rlimits = %v", rl)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestDiscountGVisorOverhead(t *testing.T) {
	r := &Result{PeakMemoryBytes: gvisorMemBaseline + 5, CPUTime: gvisorCPUBaseline + time.Millisecond, PeakPIDs: gvisorPIDBaseline + 3}
	discountGVisorOverhead(r)
	if r.PeakMemoryBytes != 5 || r.CPUTime != time.Millisecond || r.PeakPIDs != 3 {
		t.Errorf("got %+v", r)
	}
	low := &Result{PeakMemoryBytes: 1, CPUTime: 1, PeakPIDs: 1}
	discountGVisorOverhead(low)
	if low.PeakMemoryBytes != 0 || low.CPUTime != 0 || low.PeakPIDs != 0 {
		t.Errorf("a peak below the baseline must clamp to zero, got %+v", low)
	}
}

// requireGVisor skips unless the gVisor backend can run here.
func requireGVisor(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (run via make test-sandbox)")
	}
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("runsc not installed (scripts/setup-gvisor.sh)")
	}
	capHostCgroup(t)
}

func TestGVisorBackendRuns(t *testing.T) {
	requireGVisor(t)
	tests := []struct {
		name       string
		script     string
		stdin      string
		wantStdout string
		wantExit   int
		wantSignal syscall.Signal
		wantResult string
	}{
		{"echo", "echo hi", "", "hi\n", 0, 0, ""},
		{"stdin passthrough", "cat", "hello", "hello", 0, 0, ""},
		{"exit code", "exit 7", "", "", 7, 0, ""},
		{"result fd is separate from stdout", "echo data >&4; echo out", "", "out\n", 0, 0, "data\n"},
		{"killed by signal", "kill -9 $$", "", "", -1, syscall.SIGKILL, ""},
		{"runs as unprivileged user", "id -u", "", "65534\n", 0, 0, ""},
		{"usr is read only", "echo x > /usr/f 2>/dev/null || echo denied", "", "denied\n", 0, 0, ""},
		{"host files are absent", "cat /etc/shadow 2>/dev/null || echo absent", "", "absent\n", 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Spec{Argv: sh(tt.script), Limits: DefaultLimits(), Backend: BackendGVisor}
			if tt.stdin != "" {
				spec.Stdin = strings.NewReader(tt.stdin)
			}
			res, err := Run(context.Background(), spec)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if string(res.Stdout) != tt.wantStdout || res.ExitCode != tt.wantExit || res.Signal != tt.wantSignal || string(res.ResultData) != tt.wantResult {
				t.Errorf("got stdout=%q exit=%d signal=%v result=%q; want stdout=%q exit=%d signal=%v result=%q",
					res.Stdout, res.ExitCode, res.Signal, res.ResultData, tt.wantStdout, tt.wantExit, tt.wantSignal, tt.wantResult)
			}
		})
	}
	requireNoRunCgroups(t)
	requireNoGVisorLeftovers(t)
}

func TestGVisorBackendWallLimitKillsRun(t *testing.T) {
	requireGVisor(t)
	spec := Spec{Argv: sh("while :; do :; done"), Limits: DefaultLimits(), Backend: BackendGVisor}
	spec.Limits.WallTime = time.Second
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.TimedOut || res.WallTime > 5*time.Second {
		t.Errorf("TimedOut=%v WallTime=%v, want a timeout within the grace period", res.TimedOut, res.WallTime)
	}
	requireNoRunCgroups(t)
	requireNoGVisorLeftovers(t)
}

func TestGVisorBackendProgramThatCannotStartIsAHostError(t *testing.T) {
	requireGVisor(t)
	_, err := Run(context.Background(), Spec{Argv: []string{"/bin/does-not-exist"}, Limits: DefaultLimits(), Backend: BackendGVisor})
	if err == nil || !strings.Contains(err.Error(), ErrSandbox.Error()) {
		t.Errorf("err = %v, want ErrSandbox", err)
	}
}

// requireNoGVisorLeftovers fails if a finished run left runsc state behind: the
// null network namespace bind mount (see deleteRunsc).
func requireNoGVisorLeftovers(t *testing.T) {
	t.Helper()
	mi, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(mi), "\n") {
		if strings.Contains(line, "/lf-gvisor-") && strings.Contains(line, "null-netns") {
			t.Errorf("runsc mount leaked: %s", line)
		}
	}
}
