//go:build linux && adversarial

package sandbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Phase 6 additions to the adversarial suite. Same rules as adversarial_test.go:
// hostile programs run only inside the sandbox, and each test asserts the
// attack was contained, not merely that it ran.

// probeC prefixes every C probe: DENIED needs any failure, SECCOMP needs EPERM,
// which is what the seccomp policy returns (the kernel's own answer for these
// calls with zero arguments would be EFAULT, EINVAL or ENOSYS).
const probeC = `
#define _GNU_SOURCE
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <sched.h>
#include <time.h>
#include <sys/sysmacros.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/mount.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

static int escaped = 0;
#define DENIED(name, expr) do { errno = 0; long r_ = (long)(expr); \
	if (r_ < 0) printf("denied %s: %s\n", name, strerror(errno)); \
	else { printf("ESCAPED %s (result %ld)\n", name, r_); escaped = 1; } } while (0)
#define SECCOMP(name, expr) do { errno = 0; long r_ = (long)(expr); \
	if (r_ < 0 && errno == EPERM) printf("denied %s: seccomp\n", name); \
	else { printf("NOT-FILTERED %s (result %ld errno %d)\n", name, r_, errno); escaped = 1; } } while (0)
#define FINISH() do { puts(escaped ? "RESULT: ESCAPED" : "RESULT: contained"); return 0; } while (0)
`

const procSysProbeC = probeC + `
int main(void) {
	const char *paths[] = {
		"/proc", "/proc/self", "/proc/self/mem", "/proc/self/maps", "/proc/self/exe", "/proc/self/root",
		"/proc/self/cwd", "/proc/self/fd", "/proc/self/environ", "/proc/1/root", "/proc/1/environ",
		"/proc/1/cmdline", "/proc/sys/kernel/core_pattern", "/proc/sys/kernel/modprobe", "/proc/sysrq-trigger",
		"/proc/kcore", "/proc/kallsyms", "/proc/mounts", "/proc/cpuinfo", "/proc/net/tcp", "/proc/version",
		"/proc/sched_debug", "/proc/keys", "/proc/self/mountinfo", "/proc/self/ns/mnt", "/proc/self/cgroup",
		"/sys", "/sys/fs/cgroup", "/sys/fs/cgroup/cgroup.procs", "/sys/kernel", "/sys/kernel/debug",
		"/sys/class/net", "/sys/devices/system/cpu", "/sys/firmware", "/sys/module",
		"/dev/shm", "/dev/pts", "/dev/ptmx", "/dev/tty", "/dev/console", "/dev/fd", "/dev/stdin",
		"/dev/random", "/dev/full", "/dev/net/tun", "/dev/fuse", "/dev/kvm", "/dev/loop0", "/dev/disk",
		"/dev/mem", "/dev/kmem", "/dev/port", "/dev/kmsg", "/dev/sda", "/dev/nvme0n1", "/dev/xvda",
		"/dev/vda", "/dev/mapper", "/dev/snd", "/dev/dri", "/dev/hugepages", "/dev/mqueue",
	};
	for (unsigned i = 0; i < sizeof paths / sizeof *paths; i++) {
		struct stat st;
		errno = 0;
		int fd = open(paths[i], O_RDONLY | O_NONBLOCK);
		int sr = stat(paths[i], &st);
		if (fd >= 0 || sr == 0) { printf("ESCAPED reachable %s (open=%d stat=%d)\n", paths[i], fd, sr); escaped = 1; }
		if (fd >= 0) close(fd);
	}
	// Mounting a fresh proc, sysfs or cgroup would undo --disable_proc.
	SECCOMP("mount proc", mount("proc", "/tmp", "proc", 0, 0));
	SECCOMP("mount sysfs", mount("sysfs", "/tmp", "sysfs", 0, 0));
	SECCOMP("mount cgroup2", mount("none", "/tmp", "cgroup2", 0, 0));
	SECCOMP("mount devtmpfs", mount("devtmpfs", "/tmp", "devtmpfs", 0, 0));
	SECCOMP("mount overlay", mount("overlay", "/tmp", "overlay", 0, "lowerdir=/usr"));
	SECCOMP("mount fuse", mount("none", "/tmp", "fuse", 0, 0));
	SECCOMP("mount binfmt_misc", mount("none", "/tmp", "binfmt_misc", 0, 0));
	// The root directory holds only what the sandbox puts there.
	DIR *d = opendir("/");
	struct dirent *e;
	const char *allowed[] = {".", "..", "usr", "lib", "lib64", "bin", "tmp", "dev", "var", "etc"};
	while (d && (e = readdir(d))) {
		int ok = 0;
		for (unsigned i = 0; i < sizeof allowed / sizeof *allowed; i++) if (!strcmp(e->d_name, allowed[i])) ok = 1;
		if (!ok) { printf("ESCAPED unexpected entry in /: %s\n", e->d_name); escaped = 1; }
		else printf("root entry %s\n", e->d_name);
	}
	FINISH();
}`

// fdLeakC lists every descriptor the program starts with, then tries to use
// the fds the host test deliberately left open and inheritable.
const fdLeakC = probeC + `
int main(void) {
	char buf[256];
	int open_fds[8], n = 0;
	for (int fd = 0; fd < 4096; fd++) {
		if (fcntl(fd, F_GETFD) == -1) continue;
		if (n < 8) open_fds[n] = fd;
		n++;
		printf("open fd %d\n", fd);
		if (fd > 4) {
			ssize_t got = pread(fd, buf, sizeof buf - 1, 0);
			if (got > 0) { buf[got] = 0; printf("ESCAPED fd %d leaks: %s\n", fd, buf); escaped = 1; }
			else printf("ESCAPED fd %d is open\n", fd), escaped = 1;
		}
	}
	// 3 is nsjail's log; it must be closed before the program runs so the
	// program cannot write into the log the host parses.
	if (fcntl(3, F_GETFD) != -1) { puts("ESCAPED nsjail log fd 3 is open"); escaped = 1; }
	if (write(3, "[I] forged\n", 11) >= 0) { puts("ESCAPED wrote to fd 3"); escaped = 1; }
	// Descriptors cannot be conjured above the limit either.
	struct rlimit rl;
	getrlimit(RLIMIT_NOFILE, &rl);
	int count = 0;
	while (open("/dev/null", O_RDONLY) >= 0 && count < 100000) count++;
	printf("opened %d more fds (limit %lu)\n", count, (unsigned long)rl.rlim_cur);
	if (count > (int)rl.rlim_cur) { puts("ESCAPED nofile limit not enforced"); escaped = 1; }
	FINISH();
}`

// pathProbeC takes the path of a host file the sandbox must not see (argv[1])
// and the directory its own binary was mounted from (argv[2]).
const pathProbeC = probeC + `
static void try_read(const char *label, const char *path) {
	char buf[64];
	int fd = open(path, O_RDONLY);
	if (fd >= 0) {
		ssize_t n = read(fd, buf, sizeof buf - 1);
		printf("ESCAPED %s: opened %s (read %zd)\n", label, path, n);
		escaped = 1;
		close(fd);
	} else printf("denied %s: %s\n", label, strerror(errno));
}
int main(int argc, char **argv) {
	if (argc < 3) return 2;
	const char *secret = argv[1], *bindDir = argv[2];
	char path[512];
	try_read("secret by path", secret);
	const char *host[] = {"/etc/hostname", "/etc/os-release", "/etc/passwd", "/etc/shadow", "/etc/resolv.conf", "/etc/hosts",
		"/etc/ssh/sshd_config", "/etc/machine-id", "/root/.bashrc", "/home/ubuntu/.ssh/authorized_keys",
		"/home/ubuntu/.aws/credentials", "/home/ubuntu/Leetforce/.env", "/home/ubuntu/lf-a/.env", "/home/ubuntu/lf-b/.env",
		"/var/log/syslog", "/var/log/auth.log", "/var/run/docker.sock", "/run/containerd/containerd.sock",
		"/run/systemd/private", "/boot/vmlinuz", "/opt", "/mnt", "/srv", "/media", "/sbin/init", "/snap",
		"/usr/local/go/../../../etc/shadow", "/tmp/../etc/shadow", "/dev/../etc/shadow", "/bin/../etc/shadow",
		"/var/lib/cloud/instance/user-data.txt", "/var/lib/cloud/instances", "/sys/class/dmi/id/product_uuid"};
	for (unsigned i = 0; i < sizeof host / sizeof *host; i++) try_read(host[i], host[i]);
	// Dot-dot out of the directory the program was bind-mounted from.
	snprintf(path, sizeof path, "%s/../%s", bindDir, strrchr(secret, '/') + 1);
	try_read("dotdot from bind dir to sibling", path);
	snprintf(path, sizeof path, "%s/../../../../../../../../etc/shadow", bindDir);
	try_read("dotdot from bind dir to /etc/shadow", path);
	// The classic chroot escape: hold a directory fd, climb above it.
	int root = open("/", O_RDONLY | O_DIRECTORY);
	if (fchdir(root) == 0) for (int i = 0; i < 64; i++) (void)!chdir("..");
	char cwd[256];
	if (getcwd(cwd, sizeof cwd)) printf("cwd after climbing: %s\n", cwd);
	try_read("etc/shadow relative after climbing", "etc/shadow");
	struct stat st;
	if (stat("etc", &st) == 0) { puts("ESCAPED etc visible after climbing"); escaped = 1; }
	// Symlinks planted in the writable tmpfs.
	const char *targets[] = {"/", "/etc", "/proc/self/root", "../../../../../../etc", "/var/tmp", "/home", "/root", "/sys", "/dev"};
	for (unsigned i = 0; i < sizeof targets / sizeof *targets; i++) {
		char link_[64], through[160];
		snprintf(link_, sizeof link_, "/tmp/l%u", i);
		if (symlink(targets[i], link_) != 0) continue;
		snprintf(through, sizeof through, "%s/shadow", link_);
		try_read("symlink to a host dir", through);
		snprintf(through, sizeof through, "%s/%s", link_, strrchr(secret, '/') + 1);
		try_read("symlink to find secret", through);
	}
	snprintf(path, sizeof path, "%s", secret);
	if (symlink(secret, "/tmp/direct") == 0) try_read("symlink straight to secret", "/tmp/direct");
	if (symlink("/tmp/loop2", "/tmp/loop1") == 0 && symlink("/tmp/loop1", "/tmp/loop2") == 0) {
		errno = 0;
		if (open("/tmp/loop1", O_RDONLY) >= 0 || errno != ELOOP) { puts("ESCAPED symlink loop misbehaved"); escaped = 1; }
	}
	// O_PATH handle on a system dir, then climb with openat.
	int usr = open("/usr", O_PATH | O_DIRECTORY);
	int fd = openat(usr, "../etc/shadow", O_RDONLY);
	if (fd >= 0) { puts("ESCAPED openat ../ from /usr"); escaped = 1; }
	// The read-only mounts stay read-only whatever the program tries.
	DENIED("create in bind dir", open("newfile", O_CREAT | O_WRONLY, 0644));
	snprintf(path, sizeof path, "%s/planted", bindDir);
	DENIED("create next to own binary", open(path, O_CREAT | O_WRONLY, 0644));
	snprintf(path, sizeof path, "%s/%s", bindDir, strrchr(argv[0], '/') + 1);
	DENIED("overwrite own binary", open(path, O_WRONLY));
	DENIED("truncate own binary", truncate(path, 0));
	DENIED("chmod own binary", chmod(path, 0777));
	DENIED("unlink own binary", unlink(path));
	DENIED("rename over /bin/sh", rename("/tmp/direct", "/bin/sh"));
	DENIED("unlink /usr/bin/env", unlink("/usr/bin/env"));
	DENIED("chmod /usr/bin/env", chmod("/usr/bin/env", 04777));
	DENIED("link /usr/bin/env", link("/usr/bin/env", "/usr/bin/env2"));
	DENIED("mkdir in /lib", mkdir("/lib/x", 0755));
	DENIED("remount rw", mount("none", "/usr", 0, MS_REMOUNT | MS_BIND, 0));
	DENIED("mknod", mknod("/tmp/dev", S_IFBLK | 0666, makedev(8, 0)));
	FINISH();
}`

const abstractConnectC = probeC + `
int main(int argc, char **argv) {
	if (argc < 3) return 2;
	// argv[1] is an abstract socket name the host listens on, argv[2] a
	// filesystem socket path. Neither belongs to this sandbox.
	struct sockaddr_un a = {.sun_family = AF_UNIX};
	int s = socket(AF_UNIX, SOCK_STREAM, 0);
	memcpy(a.sun_path + 1, argv[1], strlen(argv[1]));
	DENIED("connect to host abstract socket", connect(s, (struct sockaddr *)&a, offsetof(struct sockaddr_un, sun_path) + 1 + strlen(argv[1])));
	struct sockaddr_un b = {.sun_family = AF_UNIX};
	strncpy(b.sun_path, argv[2], sizeof b.sun_path - 1);
	int s2 = socket(AF_UNIX, SOCK_STREAM, 0);
	DENIED("connect to host path socket", connect(s2, (struct sockaddr *)&b, sizeof b));
	DENIED("connect to docker.sock", ({ strcpy(b.sun_path, "/var/run/docker.sock"); connect(s2, (struct sockaddr *)&b, sizeof b); }));
	FINISH();
}`

// Listens on a fixed port on every address so the host can try to reach in.
const listenC = probeC + `
#include <netinet/in.h>
int main(int argc, char **argv) {
	int port = argc > 1 ? atoi(argv[1]) : 38999;
	int s = socket(AF_INET, SOCK_STREAM, 0);
	int one = 1;
	setsockopt(s, SOL_SOCKET, SO_REUSEADDR, &one, sizeof one);
	struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(port), .sin_addr.s_addr = INADDR_ANY};
	if (bind(s, (struct sockaddr *)&a, sizeof a) != 0 || listen(s, 4) != 0) { puts("listen failed"); return 1; }
	puts("listening");
	fflush(stdout);
	struct timeval tv = {4, 0};
	setsockopt(s, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof tv);
	int c = accept(s, 0, 0);
	if (c >= 0) { puts("ACCEPTED a connection from outside"); return 1; }
	puts("nobody connected");
	return 0;
}`

const ifaceC = probeC + `
#include <ifaddrs.h>
#include <net/if.h>
#include <netinet/in.h>
#include <arpa/inet.h>
int main(void) {
	struct ifaddrs *ifa, *p;
	if (getifaddrs(&ifa) != 0) { puts("no interfaces"); FINISH(); }
	for (p = ifa; p; p = p->ifa_next) {
		printf("interface %s\n", p->ifa_name);
		if (strcmp(p->ifa_name, "lo") != 0) { printf("ESCAPED interface %s\n", p->ifa_name); escaped = 1; }
	}
	// With no route out, even a UDP "connect" (no packet sent) must fail.
	int u = socket(AF_INET, SOCK_DGRAM, 0);
	struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(53)};
	inet_pton(AF_INET, "8.8.8.8", &a.sin_addr);
	DENIED("udp connect 8.8.8.8", connect(u, (struct sockaddr *)&a, sizeof a));
	int t = socket(AF_INET6, SOCK_STREAM, 0);
	struct sockaddr_in6 b = {.sin6_family = AF_INET6, .sin6_port = htons(53)};
	inet_pton(AF_INET6, "2001:4860:4860::8888", &b.sin6_addr);
	t = t < 0 ? 0 : t;
	DENIED("tcp6 connect", connect(t, (struct sockaddr *)&b, sizeof b));
	FINISH();
}`

const inodeFloodC = `
#include <fcntl.h>
#include <stdio.h>
#include <unistd.h>
int main(void) {
	char name[64];
	for (long i = 0; i < 5000000; i++) {
		snprintf(name, sizeof name, "/tmp/f%ld", i);
		int fd = open(name, O_CREAT | O_WRONLY, 0600);
		if (fd < 0) { printf("stop-at %ld\n", i); return 0; }
		close(fd);
	}
	puts("NOLIMIT");
	return 0;
}`

const fsizeC = probeC + `
int main(void) {
	signal(SIGXFSZ, SIG_IGN);
	int fd = open("/tmp/big", O_CREAT | O_RDWR, 0600);
	DENIED("fallocate past RLIMIT_FSIZE", fallocate(fd, 0, 0, 64 << 20));
	DENIED("ftruncate past RLIMIT_FSIZE", ftruncate(fd, 1L << 30));
	lseek(fd, 1L << 30, SEEK_SET);
	DENIED("sparse write past RLIMIT_FSIZE", write(fd, "x", 1));
	struct stat st;
	fstat(fd, &st);
	if (st.st_size > (1 << 20)) { printf("ESCAPED file is %ld bytes\n", (long)st.st_size); escaped = 1; }
	FINISH();
}`

const bigWriteC = `
#include <sys/mman.h>
#include <unistd.h>
int main(void) {
	size_t n = 64 << 20;
	char *p = mmap(0, n, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
	for (;;) { write(1, p, n); }
}`

const tinyWritesC = `
#include <unistd.h>
int main(void) { for (;;) { write(1, "x", 1); write(2, "y", 1); write(4, "z", 1); } }`

const cppThreadBomb = `
#include <chrono>
#include <thread>
int main() {
	for (;;) {
		try { std::thread([] { std::this_thread::sleep_for(std::chrono::seconds(60)); }).detach(); }
		catch (...) { std::this_thread::sleep_for(std::chrono::milliseconds(1)); }
	}
}`

const goBomb = `package main

import (
	"os"
	"runtime"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		for {
			if _, _, e := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0); e != 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}
	for {
		go func() { runtime.LockOSThread(); time.Sleep(time.Minute) }()
		time.Sleep(50 * time.Microsecond)
	}
}`

const javaBomb = `public class Bomb {
	public static void main(String[] a) {
		for (;;) {
			try { Thread t = new Thread(() -> { try { Thread.sleep(60000); } catch (InterruptedException e) {} }); t.setDaemon(true); t.start(); }
			catch (Throwable e) { try { Thread.sleep(1); } catch (InterruptedException x) {} }
		}
	}
}`

const pythonForkBomb = `
import os, time
while True:
    try:
        if os.fork() == 0:
            time.sleep(60)
            os._exit(0)
    except OSError:
        time.sleep(0.001)
`

const pythonThreadBomb = `
import threading, time
while True:
    try:
        threading.Thread(target=time.sleep, args=(60,), daemon=True).start()
    except RuntimeError:
        time.sleep(0.001)
`

// requireContained fails unless the probe finished and reported containment.
func requireContained(t *testing.T, res *Result) {
	t.Helper()
	out := string(res.Stdout)
	for _, bad := range []string{"ESCAPED", "NOT-FILTERED"} {
		if strings.Contains(out, bad) {
			t.Errorf("containment failure (%s):\n%s\nstderr: %s", bad, out, res.Stderr)
		}
	}
	if !strings.Contains(out, "RESULT: contained") {
		t.Errorf("the probe did not finish (exit %d, signal %v):\n%s\nstderr: %s", res.ExitCode, res.Signal, out, res.Stderr)
	}
}

func TestAdversarialProcSysAndDevProbing(t *testing.T) {
	res := adversarial(t, cProgram(t, "procsys", procSysProbeC, shortLimits()))
	requireContained(t, res)
}

func TestAdversarialFDInheritance(t *testing.T) {
	requireNsjail(t)
	secret := plantHostSecret(t)
	// Inheritable (no CLOEXEC) descriptors in the test process: a file with a
	// secret, a directory and a listening socket. Whatever nsjail's parent
	// holds open must not survive into the program.
	f, err := os.Open(secret) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var dups []int
	for _, src := range []int{int(f.Fd())} {
		fd, err := syscall.Dup(src)
		if err != nil {
			t.Fatal(err)
		}
		dups = append(dups, fd)
	}
	dirFD, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	dups = append(dups, dirFD)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	lf, _ := ln.(*net.TCPListener).File() // File() duplicates without CLOEXEC
	defer func() { _ = lf.Close() }()
	defer func() {
		for _, fd := range dups {
			_ = syscall.Close(fd)
		}
	}()
	res := adversarial(t, cProgram(t, "fdleak", fdLeakC, shortLimits()))
	requireContained(t, res)
	out := string(res.Stdout)
	if strings.Contains(out, "open fd 3\n") || !strings.Contains(out, "open fd 4\n") {
		t.Errorf("want fd 3 closed and the result fd 4 open, got:\n%s", out)
	}
	if strings.Contains(out, "TOPSECRET") {
		t.Errorf("a leaked descriptor exposed the secret:\n%s", out)
	}
}

// plantHostSecret writes a world-readable file on the host, outside any path
// the sandbox is given, with a marker no hostile program should ever print.
func plantHostSecret(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("/var/tmp", "lf-secret-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	if _, err := f.WriteString("TOPSECRET-host-file\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := os.Chmod(f.Name(), 0o644); err != nil { //nolint:gosec // readable on purpose: only the sandbox view may hide it
		t.Fatal(err)
	}
	return f.Name()
}

func TestAdversarialHostFilesAndWorkDirEscape(t *testing.T) {
	secret := plantHostSecret(t)
	spec := cProgram(t, "pathprobe", pathProbeC, shortLimits())
	spec.Argv = append(spec.Argv, secret, spec.ReadOnlyBinds[0])
	res := adversarial(t, spec)
	requireContained(t, res)
	if strings.Contains(string(res.Stdout), "TOPSECRET") {
		t.Errorf("the host secret reached the program output")
	}
}

func TestAdversarialNetworkSurfaces(t *testing.T) {
	requireNsjail(t)
	t.Run("only a private loopback exists", func(t *testing.T) {
		requireContained(t, adversarial(t, cProgram(t, "ifaces", ifaceC, shortLimits())))
	})
	t.Run("host unix sockets are unreachable", func(t *testing.T) {
		name := fmt.Sprintf("lf-adv-%d", os.Getpid())
		abs, err := net.Listen("unix", "@"+name)
		if err != nil {
			t.Skipf("abstract socket: %v", err)
		}
		defer func() { _ = abs.Close() }()
		sockPath := filepath.Join("/var/tmp", name+".sock")
		fs, err := net.Listen("unix", sockPath)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = fs.Close(); _ = os.Remove(sockPath) }()
		_ = os.Chmod(sockPath, 0o666) //nolint:gosec // reachable by uid 65534 if the path were visible
		var accepted int
		for _, l := range []net.Listener{abs, fs} {
			go func() {
				for {
					c, err := l.Accept()
					if err != nil {
						return
					}
					accepted++
					_ = c.Close()
				}
			}()
		}
		spec := cProgram(t, "unixsock", abstractConnectC, shortLimits())
		spec.Argv = append(spec.Argv, name, sockPath)
		requireContained(t, adversarial(t, spec))
		if accepted != 0 {
			t.Errorf("the host accepted %d unix connection(s) from the sandbox", accepted)
		}
	})
	t.Run("host cannot connect to a sandbox listener", func(t *testing.T) {
		const port = "38999"
		l := shortLimits()
		l.WallTime = 8 * time.Second
		spec := cProgram(t, "listen", listenC, l)
		spec.Argv = append(spec.Argv, port)
		done := make(chan *Result, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), adversarialDeadline)
			defer cancel()
			res, _ := Run(ctx, spec)
			done <- res
		}()
		var reached int
		deadline := time.Now().Add(3 * time.Second)
		for _, host := range []string{"127.0.0.1", privateIPv4(t)} {
			for time.Now().Before(deadline) {
				c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 100*time.Millisecond)
				if err == nil {
					reached++
					_ = c.Close()
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		res := <-done
		if res != nil && !strings.Contains(string(res.Stdout), "listening") {
			t.Errorf("the sandbox listener never started, so the test proved nothing: %q %q", res.Stdout, res.Stderr)
		}
		if res == nil || reached != 0 || strings.Contains(string(res.Stdout), "ACCEPTED") {
			t.Errorf("a host connection reached the sandbox listener (reached=%d, result=%v)", reached, res)
		}
		requireNoRunCgroups(t)
	})
}

func TestAdversarialDiskAndInodeExhaustion(t *testing.T) {
	requireNsjail(t)
	t.Run("a flood of tiny files is stopped by the tmpfs or the memory limit", func(t *testing.T) {
		l := shortLimits()
		l.WallTime = 15 * time.Second
		l.MemoryBytes = 64 << 20
		l.TmpfsBytes = 32 << 20
		res := adversarial(t, cProgram(t, "inodes", inodeFloodC, l))
		if strings.Contains(string(res.Stdout), "NOLIMIT") {
			t.Errorf("5M files were created: nothing bounded the tmpfs")
		}
		if !res.OOMKilled && !strings.Contains(string(res.Stdout), "stop-at") {
			t.Errorf("neither ENOSPC nor an OOM kill ended the flood: stdout=%q signal=%v", res.Stdout, res.Signal)
		}
		if res.PeakMemoryBytes > l.MemoryBytes+l.MemoryBytes/10 {
			t.Errorf("PeakMemoryBytes = %d, far above the %d limit", res.PeakMemoryBytes, l.MemoryBytes)
		}
	})
	t.Run("tmpfs data counts against the memory limit", func(t *testing.T) {
		l := shortLimits()
		l.WallTime = 15 * time.Second
		l.MemoryBytes = 32 << 20
		l.TmpfsBytes = 200 << 20
		l.MaxFileBytes = 200 << 20
		res := adversarial(t, Spec{Argv: sh(`dd if=/dev/zero of=/tmp/fill bs=1M count=150 2>/dev/null; echo "dd=$?"`), Limits: l})
		if res.PeakMemoryBytes > l.MemoryBytes+l.MemoryBytes/10 {
			t.Errorf("PeakMemoryBytes = %d, far above the %d limit", res.PeakMemoryBytes, l.MemoryBytes)
		}
		if !res.OOMKilled && !strings.Contains(string(res.Stdout), "dd=1") {
			t.Errorf("150 MiB landed in a 32 MiB group: stdout=%q signal=%v oom=%v", res.Stdout, res.Signal, res.OOMKilled)
		}
	})
	t.Run("RLIMIT_FSIZE stops fallocate, ftruncate and sparse writes", func(t *testing.T) {
		l := shortLimits()
		l.MaxFileBytes = 1 << 20
		requireContained(t, adversarial(t, cProgram(t, "fsize", fsizeC, l)))
	})
}

func TestAdversarialHugeOutputShapes(t *testing.T) {
	requireNsjail(t)
	tests := []struct {
		name string
		src  string
	}{
		{"one write of 64 MiB", bigWriteC},
		{"one byte at a time on stdout, stderr and the result fd", tinyWritesC},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := DefaultLimits()
			l.MaxOutputBytes = 4096
			l.MaxResultBytes = 4096
			l.WallTime = 10 * time.Second
			res := adversarial(t, cProgram(t, "bigout", tt.src, l))
			if !res.OutputExceeded || len(res.Stdout) > 4096 || len(res.Stderr) > 4096 || len(res.ResultData) > 4096 {
				t.Errorf("OutputExceeded=%v stdout=%d stderr=%d result=%d, want everything capped at 4096",
					res.OutputExceeded, len(res.Stdout), len(res.Stderr), len(res.ResultData))
			}
			if res.WallTime > 5*time.Second {
				t.Errorf("WallTime = %v, the flood was not stopped promptly", res.WallTime)
			}
			requireNoProcess(t, "bigout")
		})
	}
}

// bombCase describes a process or thread bomb written in one language.
type bombCase struct {
	name string
	spec func(t *testing.T) Spec
	// kill is the substring requireNoProcess must not find afterwards.
	kill string
}

func TestAdversarialBombsPerLanguage(t *testing.T) {
	requireNsjail(t)
	cases := []bombCase{
		{"python fork bomb", func(*testing.T) Spec { return Spec{Argv: []string{"/usr/bin/python3", "-c", pythonForkBomb}} }, "python3"},
		{"python thread bomb", func(*testing.T) Spec { return Spec{Argv: []string{"/usr/bin/python3", "-c", pythonThreadBomb}} }, "python3"},
		{"shell fork bomb", func(*testing.T) Spec { return Spec{Argv: sh(`f() { f | f & }; f; sleep 60`)} }, "sleep 60"},
		{"c++ thread bomb", func(t *testing.T) Spec {
			return compiledBomb(t, "cppbomb", cppThreadBomb, "g++", "-O0", "-pthread", "-static", "-x", "c++")
		}, "cppbomb"},
		{"go thread bomb (locked goroutines)", func(t *testing.T) Spec { return goBombSpec(t) }, "gobomb"},
		{"go fork bomb", func(t *testing.T) Spec { s := goBombSpec(t); s.Argv = append(s.Argv, "fork"); return s }, "gobomb"},
		{"java thread bomb", javaBombSpec, "Bomb"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec(t)
			spec.Limits = shortLimits()
			spec.Limits.WallTime = 4 * time.Second
			spec.Limits.MaxPIDs = 32
			spec.Limits.MemoryBytes = 384 << 20
			res := adversarial(t, spec)
			if !res.PIDLimitHit || res.PeakPIDs > 32 {
				t.Errorf("PIDLimitHit=%v PeakPIDs=%d signal=%v oom=%v, want the limit hit and at most 32 tasks\nstderr: %.300s",
					res.PIDLimitHit, res.PeakPIDs, res.Signal, res.OOMKilled, res.Stderr)
			}
			requireNoProcess(t, tt.kill)
		})
	}
}

// compiledBomb builds source with a host compiler (the program itself only
// ever runs in the sandbox) into a directory that is bind-mounted read-only.
func compiledBomb(t *testing.T, name, src, compiler string, flags ...string) Spec {
	t.Helper()
	requireNsjail(t)
	if _, err := exec.LookPath(compiler); err != nil {
		t.Skipf("%s not installed", compiler)
	}
	dir := bombDir(t)
	srcPath := filepath.Join(dir, name+".src")
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil { //nolint:gosec // read by the compiler only
		t.Fatal(err)
	}
	bin := filepath.Join(dir, name)
	args := append(append([]string{}, flags...), "-o", bin, srcPath)
	if out, err := exec.CommandContext(context.Background(), compiler, args...).CombinedOutput(); err != nil { //nolint:gosec // fixed test inputs
		t.Fatalf("%s: %v\n%s", compiler, err, out)
	}
	return Spec{Argv: []string{bin}, ReadOnlyBinds: []string{dir}}
}

func bombDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/var/tmp", "lf-sandbox-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // must be traversable by the jailed uid
		t.Fatal(err)
	}
	return dir
}

func goBombSpec(t *testing.T) Spec {
	t.Helper()
	requireNsjail(t)
	goBin := "/usr/local/go/bin/go"
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("go toolchain not at /usr/local/go")
	}
	dir := bombDir(t)
	src := filepath.Join(dir, "gobomb.go")
	if err := os.WriteFile(src, []byte(goBomb), 0o644); err != nil { //nolint:gosec // read by the compiler only
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "gobomb")
	cmd := exec.CommandContext(context.Background(), goBin, "build", "-o", bin, src) //nolint:gosec // fixed test inputs
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOTELEMETRY=off", "GOFLAGS=-mod=mod",
		"GOCACHE="+filepath.Join(dir, "cache"), "HOME="+dir, "GOPATH="+filepath.Join(dir, "gopath"), "GO111MODULE=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return Spec{Argv: []string{bin}, ReadOnlyBinds: []string{dir}, Env: []string{"GOMAXPROCS=2"}}
}

func javaBombSpec(t *testing.T) Spec {
	t.Helper()
	requireNsjail(t)
	const home = "/usr/lib/jvm/java-21-openjdk-amd64"
	if _, err := os.Stat(home + "/bin/javac"); err != nil {
		t.Skip("JDK not installed")
	}
	dir := bombDir(t)
	if err := os.WriteFile(filepath.Join(dir, "Bomb.java"), []byte(javaBomb), 0o644); err != nil { //nolint:gosec // read by javac only
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(context.Background(), home+"/bin/javac", "-d", dir, filepath.Join(dir, "Bomb.java")).CombinedOutput(); err != nil { //nolint:gosec // fixed test inputs
		t.Fatalf("javac: %v\n%s", err, out)
	}
	return Spec{
		Argv: []string{home + "/bin/java", "-Xmx64m", "-Xms16m", "-Xss256k", "-XX:+UseSerialGC", "-XX:-UsePerfData",
			"-XX:TieredStopAtLevel=1", "-cp", dir, "Bomb"},
		Env:           []string{"LD_LIBRARY_PATH=" + home + "/lib:" + home + "/lib/server"},
		ReadOnlyBinds: []string{dir, "/etc/java-21-openjdk"},
	}
}
