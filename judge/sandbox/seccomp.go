package sandbox

import (
	"runtime"
	"strings"
)

// The seccomp policy is a strong denylist: every syscall is allowed unless it
// is listed here. A deny-by-default allowlist was considered and not chosen:
// the four runtimes together use well over a hundred syscalls that vary with
// libc, kernel and JVM versions (clone3, faccessat2, rseq, statx and pidfd_open
// each appeared in a recent release), so an allowlist would break judging after
// routine upgrades, while the denylist below removes every syscall that is
// either a way out of the sandbox or has no use in a program that reads stdin
// and writes stdout. Defence in depth stays with the namespaces (user, mount,
// pid, net, ipc, uts, cgroup), the empty network, read-only mounts, dropped
// capabilities and the cgroup limits; the policy only shrinks the kernel
// attack surface they have to protect.
//
// Syscalls the runtimes need (measured with strace -f on the dev host; all are
// allowed and none is on the denylist). Python: openat, read, write, mmap,
// getdents64, ioctl, futex, getrandom, epoll_create1, fcntl, plus fork, vfork,
// clone, execve, wait4, pipe2 and socketpair(AF_UNIX) for subprocess and
// multiprocessing. C++ (static, glibc): brk, mmap, clone3 (falls back to clone),
// futex, rseq, madvise, set_robust_list, prlimit64, getrandom. Go: clone,
// futex, nanosleep, sched_getaffinity, sigaltstack, madvise, tgkill, epoll_*,
// pidfd_open, pidfd_send_signal, eventfd2 (go build adds vfork, wait4, waitid,
// fallocate, copy_file_range, renameat, utimensat, flock). Java: clone3,
// futex, sched_yield, sched_getaffinity, sysinfo, statfs, flock, mkdir, unlink,
// prctl, faccessat2, and socket(AF_UNIX/AF_INET/AF_INET6) with connect to
// /var/run/nscd/socket during name-service lookups (it fails harmlessly: there
// is no such path and no network). Compilers add fchdir, chmod, dup2, umask.
// So the policy must keep fork, vfork, clone (thread flags), clone3's ENOSYS
// fallback, socket for AF_UNIX/INET/INET6, flock, prctl and the pidfd calls
// that are not listed below.

// denied syscalls return EPERM. Why each group is here:
//
//   - tracing and cross-process memory: ptrace, process_vm_*, kcmp,
//     pidfd_getfd (steal descriptors from another process).
//   - mounts and namespaces: mount, umount, pivot_root, chroot, setns,
//     unshare, and the new mount API (fsopen, fsconfig, fsmount, fspick,
//     open_tree, move_mount, mount_setattr), which does everything mount does.
//   - privileged kernel interfaces: bpf, userfaultfd (widens kernel race
//     windows), perf_event_open, io_uring_* (large, frequently exploited
//     surface), keyctl and friends, module loading, kexec, reboot, swap,
//     acct, quotactl, syslog, lookup_dcookie.
//   - host-wide state: sethostname, setdomainname, settimeofday,
//     clock_settime, clock_adjtime, adjtimex, vhangup.
//   - file handle escapes: name_to_handle_at and open_by_handle_at
//     (the classic container breakout, which bypasses mount namespaces).
//   - NUMA policy: mbind, set_mempolicy, migrate_pages, move_pages
//     (rarely used, a past source of kernel bugs, and move_pages can reach other processes).
//   - x86 only: iopl, ioperm (port I/O) and modify_ldt (LDT manipulation).
var denied = []string{
	"ptrace", "process_vm_readv", "process_vm_writev", "kcmp", "pidfd_getfd",
	"mount", "umount", "pivot_root", "chroot", "setns", "unshare",
	"fsopen", "fsconfig", "fsmount", "fspick", "open_tree", "move_mount", "mount_setattr",
	"bpf", "userfaultfd", "perf_event_open",
	"io_uring_setup", "io_uring_enter", "io_uring_register",
	"keyctl", "add_key", "request_key",
	"init_module", "finit_module", "delete_module", "kexec_load", "kexec_file_load",
	"reboot", "swapon", "swapoff", "acct", "quotactl", "syslog", "lookup_dcookie",
	"sethostname", "setdomainname", "settimeofday", "clock_settime", "clock_adjtime", "adjtimex", "vhangup",
	"name_to_handle_at", "open_by_handle_at",
	"mbind", "set_mempolicy", "migrate_pages", "move_pages",
}

// x86Denied names syscalls that exist only on x86; kafel rejects names the
// architecture does not have.
var x86Denied = []string{"iopl", "ioperm", "modify_ldt"}

// namespaceCloneFlags is every CLONE_NEW* flag (CLONE_NEWTIME, NEWNS,
// NEWCGROUP, NEWUTS, NEWIPC, NEWUSER, NEWPID, NEWNET). unshare is denied
// outright, so clone with one of these is the only other way to create a
// namespace; ordinary threads and processes use none of them.
const namespaceCloneFlags = "0x7e020080"

// seccompPolicy builds the kafel policy string passed to nsjail.
//
//   - clone is denied when any CLONE_NEW* flag is set (EPERM).
//   - clone3 is answered ENOSYS: its flags live in a struct seccomp cannot
//     inspect, so it cannot be filtered safely. glibc 2.34+ and the JVM fall
//     back to clone; Go already uses clone.
//   - socket and socketpair are limited to AF_UNIX, AF_INET and AF_INET6.
//     Netlink, packet, raw-kernel, vsock, alg, key and bluetooth sockets are
//     kernel attack surface that no judged program needs (the network
//     namespace is empty, so even the allowed families reach nothing).
func seccompPolicy() string {
	names := append([]string(nil), denied...)
	if runtime.GOARCH == "amd64" {
		names = append(names, x86Denied...)
	}
	rules := []string{
		strings.Join(names, ", "),
		"clone(flags) { (flags & " + namespaceCloneFlags + ") != 0 }",
		"socket(domain) { domain != 1 && domain != 2 && domain != 10 }",
		"socketpair(domain) { domain != 1 }",
	}
	return "POLICY deny { ERRNO(1) { " + strings.Join(rules, ", ") + " }, ERRNO(38) { clone3 } } USE deny DEFAULT ALLOW"
}
