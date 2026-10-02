//go:build linux

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// compileC builds a C program into a world-readable directory that can be
// bind-mounted into the sandbox (the sandbox's own /tmp is a fresh tmpfs).
func compileC(t *testing.T, name, src string) (dir, bin string) {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not installed")
	}
	dir, err := os.MkdirTemp("/var/tmp", "lf-sandbox-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // must be traversable by the jailed uid
		t.Fatal(err)
	}
	srcPath := filepath.Join(dir, name+".c")
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil { //nolint:gosec // read by gcc only
		t.Fatal(err)
	}
	bin = filepath.Join(dir, name)
	if out, err := exec.CommandContext(context.Background(), "gcc", "-O0", "-o", bin, srcPath).CombinedOutput(); err != nil { //nolint:gosec // fixed test inputs
		t.Fatalf("gcc: %v\n%s", err, out)
	}
	return dir, bin
}

const allocC = `
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
int main(int argc, char **argv) {
	size_t mb = argc > 1 ? (size_t)atoi(argv[1]) : 1;
	char *p = malloc(mb << 20);
	if (!p) { puts("malloc failed"); return 3; }
	memset(p, 1, mb << 20);
	printf("allocated %zu MiB\n", mb);
	return 0;
}`

const forkerC = `
#include <stdio.h>
#include <unistd.h>
int main(void) {
	int n = 0;
	for (;;) {
		pid_t p = fork();
		if (p < 0) { printf("fork failed after %d children\n", n); fflush(stdout); return 0; }
		if (p == 0) { sleep(30); _exit(0); }
		if (++n > 1000) break;
	}
	return 0;
}`

func TestRunMemoryLimit(t *testing.T) {
	requireNsjail(t)
	dir, bin := compileC(t, "alloc", allocC)
	const limit = 64 << 20

	tests := []struct {
		name       string
		mib        string
		wantOOM    bool
		wantStdout string
	}{
		{"under the limit", "30", false, "allocated 30 MiB\n"},
		{"over the limit", "300", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Spec{Argv: []string{bin, tt.mib}, ReadOnlyBinds: []string{dir}, Limits: DefaultLimits()}
			spec.Limits.MemoryBytes = limit
			res, err := Run(context.Background(), spec)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if res.OOMKilled != tt.wantOOM || string(res.Stdout) != tt.wantStdout {
				t.Errorf("OOMKilled=%v stdout=%q, want %v and %q", res.OOMKilled, res.Stdout, tt.wantOOM, tt.wantStdout)
			}
			if tt.wantOOM && res.Signal == 0 {
				t.Errorf("program survived the OOM kill: %+v", res)
			}
			// memory.peak is host-measured and may slightly overshoot memory.max.
			if res.PeakMemoryBytes > limit+limit/10 {
				t.Errorf("PeakMemoryBytes = %d, far above the %d limit", res.PeakMemoryBytes, limit)
			}
			if !tt.wantOOM && res.PeakMemoryBytes < 30<<20 {
				t.Errorf("PeakMemoryBytes = %d, want at least the 30 MiB allocated", res.PeakMemoryBytes)
			}
		})
	}
}

func TestRunProcessLimit(t *testing.T) {
	requireNsjail(t)
	dir, bin := compileC(t, "forker", forkerC)
	spec := Spec{Argv: []string{bin}, ReadOnlyBinds: []string{dir}, Limits: DefaultLimits()}
	spec.Limits.MaxPIDs = 20
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.PIDLimitHit || res.PeakPIDs > 20 || !strings.Contains(string(res.Stdout), "fork failed after") {
		t.Errorf("PIDLimitHit=%v PeakPIDs=%d stdout=%q; want the limit hit, at most 20 pids, and a refused fork", res.PIDLimitHit, res.PeakPIDs, res.Stdout)
	}
	requireNoProcess(t, "forker")
}

// A run that hits its wall limit must leave nothing behind: not the program,
// not its children, not a detached grandchild, and not its cgroup.
func TestRunKillsWholeCgroup(t *testing.T) {
	requireNsjail(t)
	spec := Spec{
		Argv:   sh("sleep 61.37 & sleep 61.37 & (sleep 61.37 &); wait"),
		Limits: DefaultLimits(),
	}
	spec.Limits.WallTime = time.Second
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.TimedOut {
		t.Errorf("TimedOut = false, want true: %+v", res)
	}
	requireNoProcess(t, "61.37")
	requireNoRunCgroups(t)
}

func TestRunCPUTimeMeasured(t *testing.T) {
	requireNsjail(t)
	spec := Spec{Argv: sh("while :; do :; done"), Limits: DefaultLimits()}
	spec.Limits.WallTime = time.Second
	res, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.TimedOut || res.CPUTime < 200*time.Millisecond {
		t.Errorf("TimedOut=%v CPUTime=%v, want a timeout after at least 200ms of CPU", res.TimedOut, res.CPUTime)
	}
}

func requireNoProcess(t *testing.T, needle string) {
	t.Helper()
	paths, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // /proc scan
		if err == nil && strings.Contains(strings.ReplaceAll(string(b), "\x00", " "), needle) &&
			!strings.Contains(string(b), "go.test") && !strings.Contains(string(b), ".test") {
			t.Errorf("process still running after Run: %s: %q", p, b)
		}
	}
}

func requireNoRunCgroups(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(DefaultCgroupRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "job-") {
			t.Errorf("run cgroup leaked: %s", filepath.Join(DefaultCgroupRoot, e.Name()))
		}
	}
}
