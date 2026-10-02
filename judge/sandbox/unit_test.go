package sandbox

import (
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSpecValidate(t *testing.T) {
	good := Spec{Argv: []string{"/bin/true"}, Limits: DefaultLimits()}
	tests := []struct {
		name    string
		mutate  func(*Spec)
		wantErr bool
	}{
		{"valid", func(*Spec) {}, false},
		{"empty argv", func(s *Spec) { s.Argv = nil }, true},
		{"empty program", func(s *Spec) { s.Argv = []string{""} }, true},
		{"no wall time", func(s *Spec) { s.Limits.WallTime = 0 }, true},
		{"no cpu time", func(s *Spec) { s.Limits.CPUTime = 0 }, true},
		{"no output cap", func(s *Spec) { s.Limits.MaxOutputBytes = 0 }, true},
		{"no result cap", func(s *Spec) { s.Limits.MaxResultBytes = 0 }, true},
		{"no memory limit", func(s *Spec) { s.Limits.MemoryBytes = 0 }, true},
		{"no process limit", func(s *Spec) { s.Limits.MaxPIDs = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := good
			tt.mutate(&s)
			if err := s.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNsjailArgs(t *testing.T) {
	spec := Spec{
		Argv:          []string{"/work/prog", "arg1"},
		Env:           []string{"FOO=bar", "PATH=/custom"},
		ReadOnlyBinds: []string{"/work"},
		Limits: Limits{
			WallTime: 1500 * time.Millisecond, CPUTime: 2 * time.Second,
			MaxFileBytes: 3<<20 + 1, MaxOpenFiles: 16, MaxOutputBytes: 10, MaxResultBytes: 10, TmpfsBytes: 4096,
			MemoryBytes: 1 << 20, MaxPIDs: 7, CPUMilliPerSec: 500,
		},
	}
	args := spec.nsjailArgs("/sys/fs/cgroup/leetforce/job-x")
	joined := " " + strings.Join(args, " ") + " "
	wantUID, wantGID := idMaps()

	for _, want := range []string{
		" --user " + wantUID + " ", " --group " + wantGID + " ",
		" --log_fd 3 ", " --pass_fd 4 ", " --time_limit 2 ", " --rlimit_cpu 2 ",
		" --rlimit_fsize 4 ", " --rlimit_nofile 16 ", " --rlimit_core 0 ",
		" --disable_proc ", " -R /usr ", " -R /work ",
		" -m none:/tmp:tmpfs:size=4096 ",
		" --use_cgroupv2 ", " --cgroupv2_mount /sys/fs/cgroup/leetforce/job-x ",
		" --cgroup_mem_max 1048576 ", " --cgroup_mem_swap_max 0 ", " --cgroup_pids_max 7 ", " --cgroup_cpu_ms_per_sec 500 ",
		" -B /dev/null ", " -R /dev/urandom ", " -E FOO=bar ", " -E PATH=/custom ", " -E HOME=/tmp ",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\n%s", want, joined)
		}
	}
	// The network namespace is nsjail's default; the flag that would turn it
	// off must never appear.
	for _, banned := range []string{"disable_clone_newnet", "disable_clone_newuser", "disable_clone_newpid"} {
		if strings.Contains(joined, banned) {
			t.Errorf("args contain %q, which weakens isolation", banned)
		}
	}
	if slices.Contains(args, "PATH=/usr/local/bin:/usr/bin:/bin") {
		t.Error("default PATH must not override the spec's PATH")
	}
	if n := len(args); args[n-3] != "--" || args[n-2] != "/work/prog" || args[n-1] != "arg1" {
		t.Errorf("program must come last after --, got %v", args[n-3:])
	}
}

func TestCappedBuffer(t *testing.T) {
	tests := []struct {
		name         string
		max          int64
		writes       []string
		want         string
		wantExceeded bool
		wantCalls    int
	}{
		{"under cap", 10, []string{"abc", "def"}, "abcdef", false, 0},
		{"exactly cap", 6, []string{"abc", "def"}, "abcdef", false, 0},
		{"over cap truncates", 4, []string{"abc", "def"}, "abcd", true, 1},
		{"callback once", 2, []string{"abc", "def", "ghi"}, "ab", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			b := newCappedBuffer(tt.max, func() { calls++ })
			for _, w := range tt.writes {
				n, err := b.Write([]byte(w))
				if err != nil || n != len(w) {
					t.Fatalf("Write(%q) = %d, %v; want full write and no error", w, n, err)
				}
			}
			if got := string(b.Bytes()); got != tt.want {
				t.Errorf("Bytes() = %q, want %q", got, tt.want)
			}
			if b.Exceeded() != tt.wantExceeded || calls != tt.wantCalls {
				t.Errorf("Exceeded() = %v, calls = %d; want %v, %d", b.Exceeded(), calls, tt.wantExceeded, tt.wantCalls)
			}
		})
	}
}

func TestParseLog(t *testing.T) {
	const exec = "[I][t] Executing '/p' for '[STANDALONE MODE]'\n"
	tests := []struct {
		name string
		log  string
		want termination
	}{
		{
			"normal exit",
			exec + "[I][t] pid=1 ([STANDALONE MODE]) exited with status: 7, (PIDs left: 0)\n",
			termination{started: true, reported: true, exitCode: 7},
		},
		{
			"killed by signal",
			exec + "[I][t] pid=1 ([STANDALONE MODE]) terminated with signal: SIGSEGV (11), (PIDs left: 0)\n",
			termination{started: true, reported: true, exitCode: -1, signal: syscall.SIGSEGV},
		},
		{
			"wall time limit",
			exec + "[I][t] pid=1 run time >= time limit (2 >= 2) ([STANDALONE MODE]). Killing it\n" +
				"[I][t] pid=1 ([STANDALONE MODE]) terminated with signal: SIGKILL (9), (PIDs left: 0)\n",
			termination{started: true, reported: true, exitCode: -1, signal: syscall.SIGKILL, timedOut: true},
		},
		{
			"nothing reported",
			"[I][t] Mode: STANDALONE_ONCE\n",
			termination{},
		},
		{
			"last outcome wins",
			exec + "[I][t] pid=1 ([X]) exited with status: 0, (PIDs left: 0)\n[I][t] pid=1 ([X]) exited with status: 3, (PIDs left: 0)\n",
			termination{started: true, reported: true, exitCode: 3},
		},
		{
			// Regression: when jail setup fails nsjail sometimes logs a SIGKILL
			// instead of exit 255. Without an Executing line the program never ran.
			"setup failure ending in SIGKILL did not start",
			"[E][t][1] buildMountTree():433 Failed to mount\n[I][t] pid=2 ([X]) terminated with signal: SIGKILL (9), (PIDs left: 0)\n",
			termination{reported: true, exitCode: -1, signal: syscall.SIGKILL},
		},
		{
			"text that is not an nsjail info line does not count as a start",
			"[I][t] Mode: STANDALONE_ONCE\nExecuting '/x' for y\n",
			termination{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLog(tt.log)
			if got.started != tt.want.started || got.reported != tt.want.reported || got.exitCode != tt.want.exitCode ||
				got.signal != tt.want.signal || got.timedOut != tt.want.timedOut {
				t.Errorf("parseLog() = %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("nsjail own failure keeps error lines", func(t *testing.T) {
		got := parseLog("[E][t][1] buildMountTree():433 Failed to mount\n[F][t][1] runChild():561 Launching child process failed\n" +
			"[I][t] pid=2 ([X]) exited with status: 255, (PIDs left: 0)\n")
		if len(got.failure) != 2 || got.started {
			t.Errorf("parseLog() = %+v, want 2 failure lines and started=false", got)
		}
	})
}

func TestCeilSeconds(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want int64
	}{
		{0, 1}, {time.Millisecond, 1}, {time.Second, 1}, {1500 * time.Millisecond, 2}, {10 * time.Second, 10},
	}
	for _, tt := range tests {
		if got := ceilSeconds(tt.in); got != tt.want {
			t.Errorf("ceilSeconds(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
