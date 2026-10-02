//go:build linux && adversarial

package sandbox

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// The adversarial suite: each test is a hostile program the sandbox must
// contain. Programs are tiny C or shell programs run only inside the sandbox.
// A test passes only if the attack was stopped, Run returned in bounded time,
// and nothing (process or cgroup) was left behind on the host.
// Run with `make test-adversarial`, which also runs the ordinary sandbox tests
// (including the forged-result cases in result_test.go) in the same invocation.

const adversarialDeadline = 30 * time.Second

func adversarial(t *testing.T, spec Spec) *Result {
	t.Helper()
	requireNsjail(t)
	ctx, cancel := context.WithTimeout(context.Background(), adversarialDeadline)
	defer cancel()
	res, err := Run(ctx, spec)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.WallTime > spec.Limits.WallTime+6*time.Second {
		t.Errorf("WallTime = %v for a %v limit: the run was not stopped promptly", res.WallTime, spec.Limits.WallTime)
	}
	requireNoRunCgroups(t)
	return res
}

func cProgram(t *testing.T, name, src string, limits Limits) Spec {
	t.Helper()
	requireNsjail(t)
	dir, bin := compileC(t, name, src)
	return Spec{Argv: []string{bin}, ReadOnlyBinds: []string{dir}, Limits: limits}
}

func shortLimits() Limits {
	l := DefaultLimits()
	l.WallTime = 2 * time.Second
	return l
}

const forkBombC = `
#include <unistd.h>
int main(void) { for (;;) fork(); }`

const threadBombC = `
#include <pthread.h>
#include <unistd.h>
static void *idle(void *a) { (void)a; sleep(60); return 0; }
int main(void) {
	for (;;) { pthread_t t; if (pthread_create(&t, 0, idle, 0) != 0) usleep(1000); }
}`

const memBombC = `
#include <stdlib.h>
#include <string.h>
int main(void) { for (;;) { char *p = malloc(1 << 20); if (p) memset(p, 1, 1 << 20); } }`

// Eight processes each take 20 MiB: none is over the limit alone, together they are.
const aggregateMemC = `
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
int main(void) {
	for (int i = 0; i < 8; i++) {
		if (fork() == 0) { char *p = malloc(20 << 20); if (p) memset(p, 1, 20 << 20); sleep(30); _exit(0); }
	}
	sleep(30);
	return 0;
}`

const spinC = `int main(void) { for (;;) {} }`

const ignoreSignalsC = `
#include <signal.h>
int main(void) { signal(SIGTERM, SIG_IGN); signal(SIGINT, SIG_IGN); signal(SIGHUP, SIG_IGN); for (;;) {} }`

const floodNoNewlineC = `
#include <string.h>
#include <unistd.h>
int main(void) {
	static char buf[65536];
	memset(buf, 'A', sizeof buf);
	for (;;) { write(1, buf, sizeof buf); write(2, buf, sizeof buf); }
}`

// escapeC tries many ways out. Every attempt must fail; any success prints
// ESCAPED so the Go test can fail on it.
const escapeC = `
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <sched.h>
#include <stdio.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/ptrace.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/swap.h>
#include <sys/syscall.h>
#include <sys/sysmacros.h>
#include <sys/types.h>
#include <unistd.h>
#include <linux/if_packet.h>

static int escaped = 0;
#define DENIED(name, expr) do { errno = 0; long r = (long)(expr); \
	if (r < 0) printf("denied %s: %s\n", name, strerror(errno)); \
	else { printf("ESCAPED %s (result %ld)\n", name, r); escaped = 1; } } while (0)

int main(void) {
	printf("uid=%d gid=%d\n", getuid(), getgid());
	if (getuid() == 0) { puts("ESCAPED running as root"); escaped = 1; }

	DENIED("read /etc/shadow", open("/etc/shadow", O_RDONLY));
	DENIED("read /etc/passwd", open("/etc/passwd", O_RDONLY));
	DENIED("read /root", open("/root", O_RDONLY));
	DENIED("read /home", open("/home", O_RDONLY));
	DENIED("read /proc/1/environ", open("/proc/1/environ", O_RDONLY));
	DENIED("read /sys/fs/cgroup", open("/sys/fs/cgroup", O_RDONLY));
	DENIED("dotdot from cwd", open("../../../../etc/shadow", O_RDONLY));
	DENIED("dotdot via /tmp", open("/tmp/../etc/shadow", O_RDONLY));
	DENIED("dotdot via /usr", open("/usr/../etc/shadow", O_RDONLY));
	DENIED("host repo", open("/home/ubuntu/Leetforce", O_RDONLY));
	DENIED("write /usr", open("/usr/x", O_CREAT | O_WRONLY, 0644));
	DENIED("write /bin/sh", open("/bin/sh", O_WRONLY));
	DENIED("write /lib", open("/lib/x", O_CREAT | O_WRONLY, 0644));
	DENIED("open /dev/mem", open("/dev/mem", O_RDONLY));
	DENIED("open /dev/kmsg", open("/dev/kmsg", O_RDONLY));
	DENIED("open /dev/sda", open("/dev/sda", O_RDONLY));
	DENIED("mknod device", mknod("/tmp/null2", S_IFCHR | 0666, makedev(1, 3)));
	DENIED("hard link host file", link("/etc/shadow", "/tmp/hl"));
	if (symlink("/etc", "/tmp/link") == 0) DENIED("read via symlink to /etc", open("/tmp/link/shadow", O_RDONLY));
	DENIED("mount tmpfs", mount("none", "/tmp", "tmpfs", 0, 0));
	DENIED("mount bind", mount("/etc", "/tmp", 0, MS_BIND, 0));
	DENIED("umount /usr", umount("/usr"));
	DENIED("chroot", chroot("/tmp"));
	DENIED("pivot_root", syscall(SYS_pivot_root, "/tmp", "/tmp"));
	DENIED("unshare user ns", unshare(CLONE_NEWUSER));
	DENIED("unshare mount ns", unshare(CLONE_NEWNS));
	DENIED("setns", syscall(SYS_setns, 0, 0));
	DENIED("ptrace traceme", ptrace(PTRACE_TRACEME, 0, 0, 0));
	DENIED("bpf", syscall(SYS_bpf, 0, 0, 0));
	DENIED("keyctl", syscall(SYS_keyctl, 0, 0, 0, 0, 0));
	DENIED("perf_event_open", syscall(SYS_perf_event_open, 0, 0, 0, 0, 0));
	DENIED("init_module", syscall(SYS_init_module, 0, 0, ""));
	DENIED("kexec_load", syscall(SYS_kexec_load, 0, 0, 0, 0));
	DENIED("swapon", swapon("/tmp/none", 0));
	DENIED("setuid(0)", setuid(0));
	DENIED("setgid(0)", setgid(0));
	DENIED("raw icmp socket", socket(AF_INET, SOCK_RAW, IPPROTO_ICMP));
	DENIED("packet socket", socket(AF_PACKET, SOCK_RAW, 0));
	struct rlimit rl;
	if (getrlimit(RLIMIT_NOFILE, &rl) == 0) { rl.rlim_max += 1; DENIED("raise hard nofile limit", setrlimit(RLIMIT_NOFILE, &rl)); }

	puts(escaped ? "RESULT: ESCAPED" : "RESULT: contained");
	return 0;
}`

func TestAdversarialForkBomb(t *testing.T) {
	l := shortLimits()
	l.MaxPIDs = 64
	res := adversarial(t, cProgram(t, "forkbomb", forkBombC, l))
	if !res.PIDLimitHit || res.PeakPIDs > 64 {
		t.Errorf("PIDLimitHit=%v PeakPIDs=%d, want the limit hit and at most 64 processes", res.PIDLimitHit, res.PeakPIDs)
	}
	requireNoProcess(t, "forkbomb")
}

func TestAdversarialThreadBomb(t *testing.T) {
	l := shortLimits()
	l.MaxPIDs = 64
	res := adversarial(t, cProgram(t, "threadbomb", threadBombC, l))
	if !res.PIDLimitHit || res.PeakPIDs > 64 {
		t.Errorf("PIDLimitHit=%v PeakPIDs=%d, want the limit hit and at most 64 threads", res.PIDLimitHit, res.PeakPIDs)
	}
	requireNoProcess(t, "threadbomb")
}

func TestAdversarialMemoryBomb(t *testing.T) {
	l := shortLimits()
	l.MemoryBytes = 64 << 20
	res := adversarial(t, cProgram(t, "membomb", memBombC, l))
	if !res.OOMKilled || res.Signal == 0 {
		t.Errorf("OOMKilled=%v Signal=%v, want the memory bomb OOM-killed", res.OOMKilled, res.Signal)
	}
	if res.PeakMemoryBytes > l.MemoryBytes+l.MemoryBytes/10 {
		t.Errorf("PeakMemoryBytes = %d, far above the %d limit", res.PeakMemoryBytes, l.MemoryBytes)
	}
	requireNoProcess(t, "membomb")
}

func TestAdversarialAggregateMemory(t *testing.T) {
	l := shortLimits()
	l.MemoryBytes = 64 << 20
	res := adversarial(t, cProgram(t, "aggmem", aggregateMemC, l))
	if !res.OOMKilled {
		t.Errorf("OOMKilled = false: 8 x 20 MiB across processes must exceed the 64 MiB group limit")
	}
	if res.PeakMemoryBytes > l.MemoryBytes+l.MemoryBytes/10 {
		t.Errorf("PeakMemoryBytes = %d, far above the %d limit", res.PeakMemoryBytes, l.MemoryBytes)
	}
	requireNoProcess(t, "aggmem")
}

func TestAdversarialTmpfsAndFileSizeLimits(t *testing.T) {
	requireNsjail(t)
	t.Run("tmpfs fills up and writes fail", func(t *testing.T) {
		l := shortLimits()
		l.WallTime = 10 * time.Second
		l.TmpfsBytes = 8 << 20
		l.MaxFileBytes = 64 << 20
		res := adversarial(t, Spec{
			Argv:   sh(`i=0; while [ $i -lt 100 ]; do dd if=/dev/zero of=/tmp/f$i bs=1M count=1 2>/dev/null || { echo full-at-$i; break; }; i=$((i+1)); done`),
			Limits: l,
		})
		out := string(res.Stdout)
		var n int
		if _, err := fmt.Sscanf(out, "full-at-%d", &n); err != nil || n > 9 {
			t.Errorf("stdout=%q, want the tmpfs to refuse writes by the 10th MiB", out)
		}
	})
	t.Run("a file over the size limit kills the writer", func(t *testing.T) {
		l := shortLimits()
		l.WallTime = 10 * time.Second
		l.MaxFileBytes = 1 << 20
		// The shell survives; what matters is what happened to dd and the file.
		res := adversarial(t, Spec{Argv: sh(`dd if=/dev/zero of=/tmp/big bs=1M count=50 2>/dev/null; echo "status=$?"; wc -c < /tmp/big`), Limits: l})
		var status, size int
		if _, err := fmt.Sscanf(string(res.Stdout), "status=%d\n%d", &status, &size); err != nil {
			t.Fatalf("unexpected output %q: %v", res.Stdout, err)
		}
		if status == 0 || size > 1<<20 {
			t.Errorf("dd status=%d, file size=%d; want dd killed and the file capped at 1 MiB", status, size)
		}
	})
}

type infiniteZeros struct{}

func (infiniteZeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestAdversarialInfiniteLoops(t *testing.T) {
	requireNsjail(t)
	t.Run("busy loop hits the wall limit", func(t *testing.T) {
		res := adversarial(t, cProgram(t, "spin", spinC, shortLimits()))
		if !res.TimedOut || res.Signal != syscall.SIGKILL {
			t.Errorf("TimedOut=%v Signal=%v, want a timeout kill", res.TimedOut, res.Signal)
		}
		requireNoProcess(t, "spin")
	})
	t.Run("sleeping forever hits the wall limit", func(t *testing.T) {
		res := adversarial(t, Spec{Argv: []string{"/bin/sleep", "1000"}, Limits: shortLimits()})
		if !res.TimedOut {
			t.Errorf("TimedOut = false for a program that sleeps forever")
		}
	})
	t.Run("ignoring signals does not help", func(t *testing.T) {
		res := adversarial(t, cProgram(t, "ignoresig", ignoreSignalsC, shortLimits()))
		if !res.TimedOut || res.Signal != syscall.SIGKILL {
			t.Errorf("TimedOut=%v Signal=%v, want SIGKILL despite ignored signals", res.TimedOut, res.Signal)
		}
		requireNoProcess(t, "ignoresig")
	})
	t.Run("cpu limit stops a spinner before the wall limit", func(t *testing.T) {
		l := DefaultLimits()
		l.WallTime = 20 * time.Second
		l.CPUTime = time.Second
		res := adversarial(t, cProgram(t, "spincpu", spinC, l))
		// nsjail sets the soft and hard RLIMIT_CPU equal, so the kernel kills with
		// SIGKILL (not SIGXCPU) at the limit. A judge must therefore tell a CPU
		// time-limit kill from the measured CPUTime, not from the signal.
		if res.Signal != syscall.SIGKILL || res.TimedOut || res.CPUTime < 900*time.Millisecond || res.WallTime > 10*time.Second {
			t.Errorf("Signal=%v TimedOut=%v CPUTime=%v WallTime=%v, want a SIGKILL at about 1s of CPU, before the wall limit",
				res.Signal, res.TimedOut, res.CPUTime, res.WallTime)
		}
	})
	t.Run("an unread infinite stdin cannot hold the run", func(t *testing.T) {
		spec := Spec{Argv: sh(`sleep 1; echo done`), Limits: shortLimits(), Stdin: infiniteZeros{}}
		spec.Limits.WallTime = 5 * time.Second
		res := adversarial(t, spec)
		if !strings.Contains(string(res.Stdout), "done") {
			t.Errorf("stdout=%q, want the program to finish normally", res.Stdout)
		}
	})
}

func TestAdversarialOutputFlood(t *testing.T) {
	requireNsjail(t)
	tests := []struct {
		name string
		spec func(t *testing.T) Spec
	}{
		{"stdout lines", func(*testing.T) Spec { return Spec{Argv: []string{"/usr/bin/yes"}} }},
		{"stderr lines", func(*testing.T) Spec { return Spec{Argv: sh(`yes >&2`)} }},
		{"no newlines, both streams", func(t *testing.T) Spec { return cProgram(t, "flood", floodNoNewlineC, DefaultLimits()) }},
		{"binary zeros", func(*testing.T) Spec { return Spec{Argv: []string{"/bin/cat", "/dev/zero"}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec(t)
			spec.Limits = DefaultLimits()
			spec.Limits.MaxOutputBytes = 4096
			spec.Limits.WallTime = 10 * time.Second
			res := adversarial(t, spec)
			if !res.OutputExceeded || len(res.Stdout) > 4096 || len(res.Stderr) > 4096 {
				t.Errorf("OutputExceeded=%v len(stdout)=%d len(stderr)=%d, want the flood capped at 4096", res.OutputExceeded, len(res.Stdout), len(res.Stderr))
			}
			if res.WallTime > 5*time.Second {
				t.Errorf("WallTime = %v, flood was not stopped promptly", res.WallTime)
			}
		})
	}
}

const networkPy = `
import socket
targets = [("127.0.0.1", %d), ("%s", %d), ("1.1.1.1", 53), ("8.8.8.8", 53), ("169.254.169.254", 80)]
for host, port in targets:
    s = socket.socket(); s.settimeout(2)
    try:
        s.connect((host, port)); print("CONNECTED tcp", host, port)
    except OSError as e:
        print("blocked tcp", host, e.errno)
for host in ["1.1.1.1", "8.8.8.8"]:
    u = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        u.sendto(b"x", (host, 53)); print("SENT udp", host)
    except OSError as e:
        print("blocked udp", host, e.errno)
try:
    socket.getaddrinfo("example.com", 80); print("RESOLVED dns")
except OSError:
    print("blocked dns")
srv = socket.socket(); srv.bind(("0.0.0.0", 0)); srv.listen(1)
c = socket.socket(); c.settimeout(2)
try:
    c.connect(("127.0.0.1", srv.getsockname()[1])); print("loopback-self-ok")
except OSError as e:
    print("blocked loopback", e.errno)
`

func TestAdversarialNetwork(t *testing.T) {
	requireNsjail(t)

	// A real service on the host, listening on every address. If the sandbox
	// reaches it, the listener counts the connection.
	var accepted atomic.Int64
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	hostIP := privateIPv4(t)

	l := shortLimits()
	l.WallTime = 25 * time.Second
	res := adversarial(t, Spec{
		Argv:   []string{"/usr/bin/python3", "-c", fmt.Sprintf(networkPy, port, hostIP, port)},
		Limits: l,
	})
	out := string(res.Stdout)
	for _, bad := range []string{"CONNECTED", "SENT", "RESOLVED"} {
		if strings.Contains(out, bad) {
			t.Errorf("the sandbox reached the network (%q):\n%s", bad, out)
		}
	}
	if got := accepted.Load(); got != 0 {
		t.Errorf("the host service accepted %d connection(s) from the sandbox", got)
	}
	// 5 tcp + 2 udp + 1 dns attempts; the sandbox's own private loopback is
	// allowed (it reaches nothing outside) and is reported separately.
	if strings.Count(out, "blocked") < 8 || !strings.Contains(out, "loopback-self-ok") {
		t.Errorf("expected 8 blocked attempts and a private loopback, got:\n%s\nstderr: %s", out, res.Stderr)
	}
}

func privateIPv4(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatalf("interface addresses: %v", err)
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			return ipn.IP.String()
		}
	}
	t.Skip("host has no non-loopback IPv4 address")
	return ""
}

func TestAdversarialFilesystemAndPrivilegeEscape(t *testing.T) {
	spec := cProgram(t, "escape", escapeC, shortLimits())
	res := adversarial(t, spec)
	out := string(res.Stdout)
	if strings.Contains(out, "ESCAPED") || !strings.Contains(out, "RESULT: contained") {
		t.Errorf("an escape attempt succeeded or the probe did not finish:\n%s\nstderr: %s", out, res.Stderr)
	}
	if !strings.Contains(out, "uid=65534") {
		t.Errorf("the program did not run as the unprivileged user:\n%s", out)
	}
}

func TestAdversarialOrphansAreKilled(t *testing.T) {
	requireNsjail(t)
	res := adversarial(t, Spec{
		Argv:   sh(`(setsid sleep 73.17 &); (sleep 73.17 &); nohup sleep 73.17 >/dev/null 2>&1 & exit 0`),
		Limits: DefaultLimits(),
	})
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 for the launcher", res.ExitCode)
	}
	requireNoProcess(t, "73.17")
}

// A final check after the other attacks: the run cgroup root is empty and no
// nsjail or attack process is left on the host.
func TestAdversarialZHostSurvives(t *testing.T) {
	requireNsjail(t)
	requireNoRunCgroups(t)
	for _, name := range []string{"nsjail", "forkbomb", "threadbomb", "membomb", "aggmem", "spin", "ignoresig", "flood"} {
		requireNoProcess(t, name)
	}
}
