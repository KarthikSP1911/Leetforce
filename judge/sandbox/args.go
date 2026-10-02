package sandbox

import (
	"strconv"
	"strings"
	"time"
)

const (
	// jailUID is the unprivileged user the program runs as, both inside the
	// user namespace and as the mapped host user. Mapping to a non-root host
	// uid keeps file permission checks from treating the program as root.
	jailUID = 65534

	// nsjailLogFD is the descriptor nsjail writes its log to. It is closed
	// before the sandboxed program starts, so user code cannot forge it.
	nsjailLogFD = 3

	jailTmp = "/tmp"
)

// systemBinds are mounted read-only in every sandbox.
var systemBinds = []string{"/usr", "/lib", "/lib64", "/bin"}

// seccompPolicy denies syscalls a judged program never needs. Everything else
// is allowed; the namespaces, empty network and read-only mounts do the rest.
const seccompPolicy = "POLICY deny { ERRNO(1) { " +
	"ptrace, mount, pivot_root, chroot, setns, unshare, bpf, " +
	"kexec_load, init_module, finit_module, delete_module, perf_event_open, " +
	"keyctl, add_key, request_key, reboot, swapon, swapoff" +
	" } } USE deny DEFAULT ALLOW"

var defaultEnv = map[string]string{
	"PATH": "/usr/local/bin:/usr/bin:/bin",
	"HOME": jailTmp,
}

// nsjailArgs builds the nsjail command line for a spec. cgroupDir is the
// run's cgroup directory, in which nsjail creates its own limited child cgroup.
// A new network
// namespace is nsjail's default and is deliberately not disabled, so the
// program has no network interface beyond a down loopback.
func (s Spec) nsjailArgs(cgroupDir string) []string {
	l := s.Limits
	uidMap := strconv.Itoa(jailUID) + ":" + strconv.Itoa(jailUID) + ":1"

	args := []string{
		"-Mo",
		"--user", uidMap,
		"--group", uidMap,
		"--hostname", "sandbox",
		"--log_fd", strconv.Itoa(nsjailLogFD),
		"--time_limit", strconv.FormatInt(ceilSeconds(l.WallTime), 10),
		"--cwd", jailTmp,
		"--disable_proc",
		"--rlimit_cpu", strconv.FormatInt(ceilSeconds(l.CPUTime), 10),
		"--rlimit_fsize", strconv.FormatUint(bytesToMiBCeil(l.MaxFileBytes), 10),
		"--rlimit_nofile", strconv.FormatUint(orDefault(l.MaxOpenFiles, 64), 10),
		"--rlimit_core", "0",
		"--seccomp_string", seccompPolicy,
		"--use_cgroupv2",
		"--cgroupv2_mount", cgroupDir,
		"--cgroup_mem_max", strconv.FormatUint(l.MemoryBytes, 10),
		"--cgroup_mem_swap_max", "0",
		"--cgroup_pids_max", strconv.FormatUint(l.MaxPIDs, 10),
		"--cgroup_cpu_ms_per_sec", strconv.FormatUint(orDefault(l.CPUMilliPerSec, 1000), 10),
	}

	for _, p := range systemBinds {
		args = append(args, "-R", p)
	}
	for _, p := range s.ReadOnlyBinds {
		args = append(args, "-R", p)
	}
	// Device nodes real programs need (JVMs and runtimes open /dev/null and /dev/urandom).
	args = append(args, "-B", "/dev/null", "-R", "/dev/zero", "-R", "/dev/urandom")
	if l.TmpfsBytes > 0 {
		args = append(args, "-m", "none:"+jailTmp+":tmpfs:size="+strconv.FormatUint(l.TmpfsBytes, 10))
	}

	for _, kv := range s.environment() {
		args = append(args, "-E", kv)
	}

	args = append(args, "--")
	return append(args, s.Argv...)
}

// environment merges defaults with the spec's own variables; the spec wins.
func (s Spec) environment() []string {
	set := make(map[string]bool, len(s.Env))
	for _, kv := range s.Env {
		key, _, _ := strings.Cut(kv, "=")
		set[key] = true
	}
	env := append([]string(nil), s.Env...)
	for _, key := range []string{"PATH", "HOME"} {
		if !set[key] {
			env = append(env, key+"="+defaultEnv[key])
		}
	}
	return env
}

func ceilSeconds(d time.Duration) int64 {
	secs := int64((d + time.Second - 1) / time.Second)
	if secs < 1 {
		return 1
	}
	return secs
}

// bytesToMiBCeil converts to the MiB unit nsjail expects for --rlimit_fsize.
func bytesToMiBCeil(b uint64) uint64 {
	const mib = 1 << 20
	if b == 0 {
		return 1
	}
	return (b + mib - 1) / mib
}

func orDefault(v, def uint64) uint64 {
	if v == 0 {
		return def
	}
	return v
}
