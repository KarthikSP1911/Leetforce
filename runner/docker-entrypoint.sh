#!/bin/sh
# Container entrypoint for the runner image (Phase 17, ADR 0029). WRITTEN, NOT RUN.
#
# Why it exists: the runner, started as an unprivileged user, needs a cgroup subtree it may
# write (judge/sandbox/delegate.go). A container's /sys/fs/cgroup is read-only and owned by
# root, and Kubernetes has no field to change that short of `privileged: true`. So the pod
# starts this script as root with only CAP_SYS_ADMIN added, and the script:
#   1. remounts the container's cgroup filesystem read-write,
#   2. hands the container's own cgroup (and only that) to the runner user,
#   3. removes every capability and execs the runner as that user with no_new_privs.
# After step 3 the process is in the same state as the systemd unit of ADR 0014:
# CapEff, CapPrm, CapAmb and CapBnd are 0, NoNewPrivs is 1. A failure at any step stops the
# container (set -e): it must never fall through to running the runner with more privilege
# than intended, or with a cgroup it cannot use.
#
# Started as a non-root user (docker run on a laptop, a host whose cgroup is already
# delegated) the script skips the setup and runs the runner as is.
set -eu

RUN_UID="${LEETFORCE_RUNNER_UID:-10001}"
RUN_GID="${LEETFORCE_RUNNER_GID:-10001}"
RUNNER=/usr/local/bin/runner

if [ "$(id -u)" != 0 ]; then
  echo "entrypoint: not root, starting the runner without cgroup setup" >&2
  exec "$RUNNER" "$@"
fi

# The container's own cgroup as seen from this mount namespace. With a private cgroup
# namespace (containerd's default on cgroup v2) this is "/" and the path is /sys/fs/cgroup;
# with the host namespace it is the pod's cgroup path. The runner computes the same path
# itself from /proc/self/cgroup (sandbox.PrepareDelegatedRoot).
rel="$(sed -n 's/^0:://p' /proc/self/cgroup)"
if [ -z "$rel" ]; then
  echo "entrypoint: /proc/self/cgroup has no cgroup v2 entry; this image needs cgroup v2" >&2
  exit 1
fi
cg="/sys/fs/cgroup${rel}"

# 1. Writable cgroup filesystem. A privileged container already has it, so a failed remount is
#    only fatal when the directory is still not writable.
mount -o remount,rw /sys/fs/cgroup 2>/dev/null || true
if [ ! -w "$cg/cgroup.procs" ]; then
  echo "entrypoint: $cg is not writable and could not be remounted (needs CAP_SYS_ADMIN and an AppArmor profile that allows mount)" >&2
  exit 1
fi

# 2. Delegation: the directory and the three control files, not the whole tree. The memory,
#    pids and cpu controllers must already be enabled by the parent; log what is there.
chown "$RUN_UID:$RUN_GID" "$cg" "$cg/cgroup.procs" "$cg/cgroup.subtree_control" "$cg/cgroup.threads"
echo "entrypoint: delegated $cg to uid $RUN_UID, controllers: $(cat "$cg/cgroup.controllers")" >&2

# 3. Drop everything and become the runner user. exec keeps the runner as PID 1 so SIGTERM from
#    the kubelet reaches its signal handler directly. The runner is the only process in the
#    container cgroup, which cgroup v2 requires before it can delegate controllers (the runner
#    moves itself into a "runner" leaf, then enables controllers on this cgroup).
exec setpriv \
  --reuid "$RUN_UID" --regid "$RUN_GID" --clear-groups \
  --inh-caps=-all --ambient-caps=-all --bounding-set=-all \
  --no-new-privs \
  -- "$RUNNER" "$@"
