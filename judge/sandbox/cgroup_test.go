package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCgroupStats(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  cgroupStats
	}{
		{
			"typical finished run",
			map[string]string{
				"memory.peak":   "42967040\n",
				"memory.events": "low 0\nhigh 0\nmax 35\noom 1\noom_kill 1\noom_group_kill 0\n",
				"pids.peak":     "20\n",
				"pids.events":   "max 1\n",
				"cpu.stat":      "usage_usec 30781\nuser_usec 10182\nsystem_usec 20599\n",
			},
			cgroupStats{PeakMemoryBytes: 42967040, CPUTime: 30781 * time.Microsecond, OOMKills: 1, PeakPIDs: 20, PIDLimitHits: 1},
		},
		{"missing files read as zero", map[string]string{}, cgroupStats{}},
		{
			"garbage is zero, not a crash",
			map[string]string{"memory.peak": "not-a-number", "cpu.stat": "usage_usec\n", "memory.events": "oom_kill x\n"},
			cgroupStats{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tt.files)
			if got := (&cgroupJob{dir: dir}).stats(); got != tt.want {
				t.Errorf("stats() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRemoveTree(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "job")
	if err := os.MkdirAll(filepath.Join(dir, "NSJAIL.1", "inner"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := removeTree(dir); err != nil {
		t.Fatalf("removeTree() error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory still exists: %v", err)
	}
	if err := removeTree(dir); err != nil {
		t.Errorf("removing an already-removed tree must succeed, got %v", err)
	}
}
