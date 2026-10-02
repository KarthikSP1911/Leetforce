//go:build linux && adversarial

package sandbox

import "testing"

// Every call here must be refused by the seccomp policy (EPERM), by number so
// the program builds with any kernel headers. 424-460 are the same on x86-64 and arm64.
const syscallProbeC = probeC + `
#include <sys/ptrace.h>
#include <sys/swap.h>
#include <sys/time.h>
#include <sys/uio.h>
static long sc(long n, long a, long b, long c, long d, long e) { return syscall(n, a, b, c, d, e, 0); }
int main(void) {
	SECCOMP("ptrace attach pid 1", ptrace(PTRACE_ATTACH, 1, 0, 0));
	SECCOMP("ptrace traceme", ptrace(PTRACE_TRACEME, 0, 0, 0));
	SECCOMP("ptrace seize self", ptrace(PTRACE_SEIZE, getpid(), 0, 0));
	struct iovec iov = {&escaped, sizeof escaped};
	SECCOMP("process_vm_readv", process_vm_readv(1, &iov, 1, &iov, 1, 0));
	SECCOMP("process_vm_writev", process_vm_writev(1, &iov, 1, &iov, 1, 0));
	SECCOMP("kcmp", sc(SYS_kcmp, 1, 1, 0, 0, 0));
	SECCOMP("pidfd_getfd", sc(438, 0, 0, 0, 0, 0));
	SECCOMP("mount", mount("none", "/tmp", "tmpfs", 0, 0));
	SECCOMP("mount bind", mount("/usr", "/tmp", 0, MS_BIND, 0));
	SECCOMP("mount remount", mount("none", "/usr", 0, MS_REMOUNT | MS_BIND, 0));
	SECCOMP("umount2", umount2("/usr", MNT_DETACH));
	SECCOMP("pivot_root", syscall(SYS_pivot_root, "/tmp", "/tmp"));
	SECCOMP("chroot", chroot("/tmp"));
	SECCOMP("setns", syscall(SYS_setns, 0, CLONE_NEWNS));
	SECCOMP("unshare user", unshare(CLONE_NEWUSER));
	SECCOMP("unshare mount", unshare(CLONE_NEWNS));
	SECCOMP("unshare net", unshare(CLONE_NEWNET));
	SECCOMP("unshare pid", unshare(CLONE_NEWPID));
	SECCOMP("unshare uts", unshare(CLONE_NEWUTS));
	SECCOMP("unshare ipc", unshare(CLONE_NEWIPC));
	SECCOMP("unshare cgroup", unshare(CLONE_NEWCGROUP));
	// clone with namespace flags is the usual way around a filter that only
	// blocks unshare; a child that gets created must exit at once.
	long nsflags[] = {CLONE_NEWUSER, CLONE_NEWNS, CLONE_NEWNET, CLONE_NEWPID, CLONE_NEWUTS, CLONE_NEWIPC, CLONE_NEWCGROUP,
		CLONE_NEWUSER | CLONE_NEWNS | CLONE_NEWNET | CLONE_NEWPID};
	for (unsigned i = 0; i < sizeof nsflags / sizeof *nsflags; i++) {
		char name[48];
		snprintf(name, sizeof name, "clone flags %#lx", nsflags[i]);
		errno = 0;
		long r = syscall(SYS_clone, nsflags[i] | SIGCHLD, 0, 0, 0, 0);
		if (r == 0) _exit(0);
		if (r < 0 && errno == EPERM) printf("denied %s: seccomp\n", name);
		else { printf("NOT-FILTERED %s (result %ld errno %d)\n", name, r, errno); escaped = 1; }
	}
	// clone3 carries its flags in memory where seccomp cannot read them, so
	// it is answered ENOSYS and libc falls back to clone.
	errno = 0;
	long r3 = syscall(435, 0, 0);
	if (r3 < 0 && errno == ENOSYS) puts("denied clone3: ENOSYS"); else { printf("NOT-FILTERED clone3 (%ld, errno %d)\n", r3, errno); escaped = 1; }
	// Only unix, IPv4 and IPv6 sockets exist; every other family is kernel attack surface.
	int bad_families[] = {16 /*netlink*/, 17 /*packet*/, 15 /*key*/, 38 /*alg*/, 40 /*vsock*/, 31 /*bluetooth*/, 44 /*xdp*/, 9 /*x25*/, 4 /*ipx*/, 10 + 20 /*unused*/};
	for (unsigned i = 0; i < sizeof bad_families / sizeof *bad_families; i++) {
		char name[48];
		snprintf(name, sizeof name, "socket family %d", bad_families[i]);
		SECCOMP(name, socket(bad_families[i], SOCK_RAW, 0));
	}
	int sv[2];
	SECCOMP("socketpair AF_INET", socketpair(AF_INET, SOCK_STREAM, 0, sv));
	DENIED("socket AF_UNIX works", (socket(AF_UNIX, SOCK_STREAM, 0) >= 0) ? -1 : 0);
	DENIED("socketpair AF_UNIX works", socketpair(AF_UNIX, SOCK_STREAM, 0, sv) == 0 ? -1 : 0);
	// New mount API.
	SECCOMP("fsopen", sc(430, 0, 0, 0, 0, 0));
	SECCOMP("fsconfig", sc(431, 0, 0, 0, 0, 0));
	SECCOMP("fsmount", sc(432, 0, 0, 0, 0, 0));
	SECCOMP("fspick", sc(433, 0, 0, 0, 0, 0));
	SECCOMP("open_tree", sc(428, -100, (long)"/", 0, 0, 0));
	SECCOMP("move_mount", sc(429, 0, 0, 0, 0, 0));
	SECCOMP("mount_setattr", sc(442, 0, 0, 0, 0, 0));
	SECCOMP("name_to_handle_at", sc(SYS_name_to_handle_at, -100, (long)"/", 0, 0, 0));
	SECCOMP("open_by_handle_at", sc(SYS_open_by_handle_at, -100, 0, 0, 0, 0));
	SECCOMP("io_uring_setup", sc(425, 1, 0, 0, 0, 0));
	SECCOMP("io_uring_enter", sc(426, 0, 0, 0, 0, 0));
	SECCOMP("io_uring_register", sc(427, 0, 0, 0, 0, 0));
	SECCOMP("userfaultfd", sc(SYS_userfaultfd, 0, 0, 0, 0, 0));
	SECCOMP("bpf", sc(SYS_bpf, 0, 0, 0, 0, 0));
	SECCOMP("perf_event_open", sc(SYS_perf_event_open, 0, 0, 0, 0, 0));
	SECCOMP("keyctl", sc(SYS_keyctl, 0, 0, 0, 0, 0));
	SECCOMP("add_key", sc(SYS_add_key, 0, 0, 0, 0, 0));
	SECCOMP("request_key", sc(SYS_request_key, 0, 0, 0, 0, 0));
	SECCOMP("init_module", sc(SYS_init_module, 0, 0, 0, 0, 0));
	SECCOMP("finit_module", sc(SYS_finit_module, -1, 0, 0, 0, 0));
	SECCOMP("delete_module", sc(SYS_delete_module, 0, 0, 0, 0, 0));
	SECCOMP("kexec_load", sc(SYS_kexec_load, 0, 0, 0, 0, 0));
	SECCOMP("kexec_file_load", sc(SYS_kexec_file_load, -1, -1, 0, 0, 0));
	SECCOMP("reboot", sc(SYS_reboot, 0, 0, 0, 0, 0));
	SECCOMP("swapon", swapon("/tmp/none", 0));
	SECCOMP("swapoff", swapoff("/tmp/none"));
	SECCOMP("acct", sc(SYS_acct, 0, 0, 0, 0, 0));
	SECCOMP("quotactl", sc(SYS_quotactl, 0, 0, 0, 0, 0));
	SECCOMP("syslog", sc(SYS_syslog, 3, 0, 0, 0, 0));
	SECCOMP("sethostname", sethostname("x", 1));
	SECCOMP("setdomainname", setdomainname("x", 1));
	struct timeval tv = {0, 0};
	SECCOMP("settimeofday", settimeofday(&tv, 0));
	struct timespec ts = {0, 0};
	SECCOMP("clock_settime", clock_settime(CLOCK_REALTIME, &ts));
	SECCOMP("clock_adjtime", sc(SYS_clock_adjtime, CLOCK_REALTIME, 0, 0, 0, 0));
	SECCOMP("adjtimex", sc(SYS_adjtimex, 0, 0, 0, 0, 0));
	SECCOMP("mbind", sc(SYS_mbind, 0, 0, 0, 0, 0));
	SECCOMP("set_mempolicy", sc(SYS_set_mempolicy, 0, 0, 0, 0, 0));
	SECCOMP("move_pages", sc(SYS_move_pages, 1, 0, 0, 0, 0));
	SECCOMP("migrate_pages", sc(SYS_migrate_pages, 1, 0, 0, 0, 0));
	SECCOMP("vhangup", vhangup());
	SECCOMP("lookup_dcookie", sc(SYS_lookup_dcookie, 0, 0, 0, 0, 0));
#ifdef SYS_iopl
	SECCOMP("iopl", sc(SYS_iopl, 3, 0, 0, 0, 0));
	SECCOMP("ioperm", sc(SYS_ioperm, 0, 1, 1, 0, 0));
	SECCOMP("modify_ldt", sc(SYS_modify_ldt, 0, 0, 0, 0, 0));
#endif
	FINISH();
}`

func TestAdversarialDangerousSyscalls(t *testing.T) {
	res := adversarial(t, cProgram(t, "syscalls", syscallProbeC, shortLimits()))
	requireContained(t, res)
}
