# 0029. Containerized runners on k3s agent nodes, scaled by KEDA

Status: PROPOSED / UNVERIFIED. Everything here was written and nothing was run: no image was built, no
Terraform, Helm, Ansible, Docker or kubectl command was executed, and no cluster, node or pod exists. Every
sentence below that sounds like a fact about behaviour is a prediction from reading code and documentation.
The list of what must be proved before real use is in [phase-17-log.md](../phases/phase-17-log.md).
Partly supersedes [ADR 0022](0022-k3s-control-and-standalone-runners.md) (runners as standalone hosts only) and
sits next to [ADR 0014](0014-runner-privilege-model.md) (the privilege model it weakens, see below).

## Context
Runners are EC2 hosts in a fixed-size Auto Scaling group (`runner_count`, ADR 0022). Nothing adds or removes
runners with load. The owner asked for a second, containerized path: runner pods that a controller scales with
the queue, on nodes that join the existing k3s control host. Decisions already made by the owner:

- Nodes are k3s agents on EC2. Their count is FIXED by the Terraform variable `runner_node_count` (default 0,
  so nothing is created until it is set). KEDA scales **pods only**. Pods that do not fit stay `Pending`;
  node autoscaling is out of scope.
- Sandbox privilege: least privilege first. If that cannot plausibly work, say so plainly and record a
  privileged fallback restricted to the isolated node pool. gVisor is not the default.
- KEDA `minReplicaCount` is 1. `maxReplicaCount`, `pollingInterval` and `cooldownPeriod` are chart values.

## Decision
1. **Image.** `runner/Dockerfile`: Ubuntu 24.04; static runner binary; nsjail built from the same pinned commit
   as the Ansible role (`scripts/check-nsjail-pin.sh` compares them); python3, g++, OpenJDK 21 at
   `/usr/lib/jvm/java-21-openjdk-amd64` with `/etc/java-21-openjdk`, and Go at `/usr/local/go`, the paths
   `judge/lang/lang.go` hard-codes. No problems or tests are in the image; the runner fetches bundles from S3.
2. **Nodes.** `infra/aws/runner-k3s.tf`: launch template and ASG (desired = min = max = `runner_node_count`),
   label `leetforce.dev/pool=runner`, taint `leetforce.dev/runner=true:NoSchedule`, join token read from SSM
   at boot, k3s at the same pinned version as the server. Ansible role `k3s_agent` does the same for a live
   host. Security groups admit the k3s agent ports (VXLAN 8472, kubelet 10250, API 6443) between the control
   host and the agents only.
3. **Pods.** `k8s/charts/leetforce-runner`: one Deployment, namespace `leetforce-runners`, pinned to the
   runner nodes by selector and toleration, `terminationGracePeriodSeconds: 330`, an egress NetworkPolicy
   (DNS, Redis, HTTPS to public addresses; pod, service, VPC and link-local ranges blocked; the metadata
   address allowed for S3, see 6), HTTP probes only, no CPU limit.
4. **Scaling.** A KEDA `ScaledObject` with a `redis-streams` trigger on `leetforce:jobs` (below).
5. **Delivery.** CI builds, Trivy-scans and pushes the image to private GHCR on `main`. Nothing rolls it out:
   `scripts/k3s/deploy-runners.sh` is run by hand on the control host (installs KEDA once, syncs Secrets,
   `helm upgrade --install`, rolls back on a failed rollout).
6. **S3 credentials.** No access keys. `storage.Open` falls back to the host IAM role when no keys are set, so
   the pod uses the node's instance role through IMDSv2. That needs the hop limit on the agent nodes to be 2
   (pods are one network hop away), which is the one place this ADR is looser than the standalone runners.

### Scaler: `streamLength` (XLEN), not `pendingEntriesCount` or `lagCount`
KEDA 2.20's `redis-streams` scaler has three modes, chosen by two settings (checked against the KEDA 2.20
documentation and the scaler source while writing; not run):

| Mode | Selected by | Redis command per poll | What it counts |
|---|---|---|---|
| `streamLength` | no `consumerGroup` | `XLEN` | every entry still in the stream |
| `pendingEntriesCount` | `consumerGroup` set | `XPENDING` | entries delivered to a consumer and not yet acknowledged |
| `lagCount` | `consumerGroup` set and `lagCount` set | `INFO`, `XINFO GROUPS`, maybe `XLEN` | entries not yet delivered to the group |

Our runners `XACK` **and `XDEL`** a job when it is finished (`queue/queue.go`, FLOW stage 7). So the stream
holds exactly the jobs that still need a runner: waiting plus in flight. That is what `XLEN` measures, and it
is the work we want pods for. The other two fail:

- `pendingEntriesCount` sees only delivered jobs. Ten jobs queued behind one busy runner read as 1, so nothing
  scales up when it matters.
- `lagCount` sees only undelivered jobs, needs Redis 7, costs up to three commands per poll (an `INFO` every
  time), and Redis may report a group's lag as null once entries have been deleted with `XDEL` (Redis's
  documented caveat for the `lag` field; how KEDA treats a null lag was not checked), which is exactly what
  our runners do to every finished job.

The target is `streamLength: 2` unfinished jobs per pod. A pod judges one job at a time, so 2 means one running
and at most one queued behind it. With 1 a burst of three jobs would start three pods, each taking a minute or
more to start (the image is large), and the jobs would finish first. Because finished jobs leave the stream,
scale-down tracks real completion and the metric never counts finished work. Dead letters live in a separate
stream (`leetforce:jobs:dead`) and are not counted.

`minReplicaCount: 1` makes the scale-from-zero concern (lag mode is the only mode KEDA documents as able to
scale to zero) irrelevant. KEDA documents that `cooldownPeriod` applies only when scaling to zero, so with a
minimum of 1 it has no effect; it is kept as a value because the owner asked for it. Scale-down pacing is the
HPA stabilization window (`keda.scaleDownStabilizationSeconds`, 300 s) and one pod per 120 s.

## Redis command budget (against [ADR 0028](0028-redis-command-budget.md))
Computed from the code and the intervals, **not measured**. Upstash counts every command, and a blocking read
that times out counts as one. ADR 0028 left one API and one runner at about 9k commands a day; the free plan is
believed to be about 0.5M a month, about 16.7k a day, and was never checked (launch checklist B2).

**Per runner pod, idle.** `Receive` issues `XAUTOCLAIM` (at most once per `LEETFORCE_JOB_RECLAIM_EVERY`) and one
`XREADGROUP BLOCK` capped at the same interval. With the 60 s default that is 2 commands per minute:
2 x 1440 = **about 2.9k a day per pod**, the same figure ADR 0028 gives for a runner (2.8k). Every extra pod
that is up costs that much for as long as it lives. Starting a pod adds a few commands (`PING`, `XGROUP CREATE`
for the existing group, the first poll).

**Per job.** About 10 commands: `XREADGROUP`, the already-published check, the judging status `XADD`, a
heartbeat every `MinIdle / 3` = 10 s (6 a minute, one Lua call each), the verdict publish, and `XACK` plus
`XDEL`. A job of ten seconds is about 8; one of two minutes about 20. At a thousand jobs a day this is about
10k, the size of the idle polling, so it matters only under load.

**Per KEDA poll.** One `XLEN` per `pollingInterval`, **provided `useCachedMetrics: true` is set** (it is, in
the ScaledObject). Without it, the HPA asks the KEDA metrics server for the value on every HPA sync (15 s by
default in kube-controller-manager) and each ask runs a fresh `XLEN`, so the rate would be 5760 a day
regardless of `pollingInterval`, on top of KEDA's own polling. With caching:

| `pollingInterval` | `XLEN` per day |
|---|---|
| 15 s | 5760 |
| 30 s | 2880 |
| **60 s (default)** | **1440** |
| 120 s | 720 |

The default is 60 s: a queue that grows is noticed within a minute, then the HPA acts within its 15 s sync. If
KEDA reconnects it also issues connection-setup commands; that is not counted.

| Scenario (idle, steady state) | Commands a day | Per 30 days |
|---|---|---|
| Today: one API plus one standalone runner (ADR 0028) | about 9k | about 0.27M |
| One pod (the minimum) replaces the standalone runner, plus KEDA at 60 s | about 9k + 1.4k = **10.4k** | **about 0.31M** |
| Pods and the standalone runner both up, plus KEDA | about 13.3k | about 0.40M |
| Four pods (the default maximum) up all day, KEDA at 60 s, no standalone runner | 6.1k (API) + 11.6k + 1.4k = **19.1k** | **about 0.57M** |

The last row is above the believed free limit. It only matters if the queue keeps four pods busy around the
clock; the HPA scales back after 5 minutes of a short queue. If the budget bites: raise `pollingInterval` to
120 s (saves 0.7k a day), set `LEETFORCE_JOB_RECLAIM_EVERY` to 120 s (saves 1.4k a day per pod; a crashed
runner's job is then reclaimed within `MinIdle + 120 s` = 150 s instead of 90 s), lower `maxReplicaCount`, or
set `runner_count = 0` so the standalone hosts do not poll as well. Every figure here must be checked against the
Upstash usage page after a day (verification list).

## Sandbox privilege: what was chosen, whether it can work, and what it costs
**Why root is needed at all.** `judge/sandbox/delegate.go` (the unprivileged path of ADR 0014) wants a cgroup
subtree it owns. Under systemd, `Delegate=` gives that. In a container, `/sys/fs/cgroup` is mounted read-only
and owned by root, and Kubernetes has no field to change either short of `privileged: true`.

**Least-privilege design (the default, `sandbox.privileged: false`).**
1. The container starts as root with `drop: [ALL]` plus five capabilities: `SYS_ADMIN` (remount the cgroup
   filesystem read-write), `CHOWN` (hand the container's own cgroup to the runner user), `SETUID`/`SETGID`
   (change user) and `SETPCAP` (empty the bounding set).
2. `runner/docker-entrypoint.sh` does exactly that, then `exec setpriv` to uid 10001 with
   `--bounding-set=-all --inh-caps=-all --ambient-caps=-all --no-new-privs`. The runner never executes a line of
   its code with a capability, and ends in the state ADR 0014 measured under systemd (CapEff, CapPrm, CapAmb,
   CapBnd all 0, NoNewPrivs 1). If any step fails the container exits; it never falls through to a runner with
   a cgroup it cannot use or with more privilege than intended.
3. `allowPrivilegeEscalation` stays at its default `true`: Kubernetes rejects `false` together with
   `CAP_SYS_ADMIN`. The entrypoint sets `no_new_privs` itself.
4. Seccomp is `RuntimeDefault`. containerd's default profile gates `mount`, `unshare`, `setns`, `clone` with
   `CLONE_NEW*` and `pivot_root` on `CAP_SYS_ADMIN` being in the container's capability set at start, so adding
   it lets nsjail's calls through while the profile still blocks the rest of the dangerous set. This is a
   statement about how the profile is generated, taken from memory of containerd's source, not checked on a
   cluster. If the in-pod suite shows it is wrong, `sandbox.seccompProfile: Unconfined` is the next step.
5. AppArmor is a Localhost profile, `leetforce-runner-pod` (`ansible/roles/k3s_agent/files/`), installed on the
   nodes. Ubuntu 24.04 sets `kernel.apparmor_restrict_unprivileged_userns=1`, which strips capabilities inside a
   user namespace created by a process whose profile lacks `userns`. containerd's default container profile has
   none, and an "unconfined" container is restricted by the sysctl too (that is why ADR 0014 needed its profile).
   The new profile allows files, capabilities, mounts, pivot_root, ptrace, signals, unix sockets, network and
   `userns`: it removes AppArmor from the picture apart from the one permission that is needed, as the nsjail
   profile does. The isolation of judged code remains nsjail's namespaces, cgroups and seccomp filter.
6. The pod is `hostUsers: true` (the default). A pod user namespace would make the container's cgroup mount a
   "locked" mount that cannot be remounted read-write.

**Can it plausibly work?** Plausibly, not demonstrably. Remounting `/sys/fs/cgroup` read-write from a container
that holds `CAP_SYS_ADMIN` is a known pattern (Docker-in-Docker recipes use it), and nsjail inside an
unprivileged user namespace worked on the host under the same profile idea. What nobody has shown is the
combination: a user namespace, new mounts and cgroup writes **inside a pod's cgroup namespace**, under the node's
containerd version, runc, kernel and AppArmor 4. Specific things that could stop it: the cgroup root of the
container not being a cgroup the kernel lets an unprivileged user delegate (the no-internal-processes rule if
anything else is in that cgroup), controllers (`memory`, `pids`, `cpu`) not enabled by the kubelet for the pod
cgroup, seccomp rejecting a call nsjail makes, or the default AppArmor/seccomp semantics differing from the
above. If this cannot be made to pass the full adversarial suite, do not paper over it: set
`sandbox.privileged: true`.

**Privileged fallback (`sandbox.privileged: true`).** The container gets every capability and a writable cgroup
filesystem; the entrypoint is unchanged. It is acceptable only because the pods run on the tainted, dedicated
runner nodes (selector, toleration, namespace, NetworkPolicy), never next to the API. It is a real step down:
a kernel exploit from a judged program that escapes the sandbox lands as root in a privileged container on the
node, not as an unprivileged user.

**Trade-off against ADR 0014.** ADR 0014's runner has no capability at any moment and runs on a host that holds
nothing else; an escape lands as `lfrunner` with a role that can read problem bundles and runner parameters.
Here: (a) the entrypoint runs as root with five capabilities for milliseconds before any runner code; (b) the
node is a cluster member, so a compromised node has a foothold on the cluster network (VXLAN reaches the
control host and the API pod; the `leetforce` namespace has no NetworkPolicy today) and its kubelet identity can
read the Secrets mounted into runner pods (`runner-env`, `ghcr-pull`), including the Redis credential, which
the standalone runner host also holds; (c) IMDS hop limit 2 lets a process in the pod reach the node role
(judged programs cannot: they have no network at all, and the pod NetworkPolicy allows only
`169.254.169.254:80`). ADR 0022 rejected "runners as k3s agents" because it adds a cluster path to hosts that
run untrusted code; this ADR accepts that cost for elasticity, which is the part of ADR 0022 it supersedes.
A default-deny NetworkPolicy for the `leetforce` namespace would reduce (b); it is not written here.

## Alternatives
- **Keep the EC2 ASG and add a Terraform scaling policy on queue depth.** Keeps ADR 0014's model intact and no
  cluster path from runner hosts. Against it: the queue depth must be published as a CloudWatch metric (a
  sidecar or the API sampler plus an IAM grant), instance boot and the unbuilt AMI make scale-out slow (minutes)
  and the AMI has never built, a scale-in terminates an instance mid-job unless a lifecycle hook drains it, and
  a scaling policy needs a CloudWatch alarm per direction. It also scales whole instances, not pods. This is the
  honest runner-up and the right choice if the privilege cost above is unacceptable.
- **EKS.** About 0.10 USD an hour for the control plane (UNVERIFIED), a custom-VPC temptation, and Fargate cannot
  run this workload (no privileged, no user namespaces). Rejected by the cost rules and the existing k3s.
- **gVisor as the pod runtime.** Avoids the capability problem but is ADR 0013's rejected default: about 18x
  slower on syscall-heavy work, about 85 ms more per job, and three adversarial tests fail under it. The owner
  said not to default to it.
- **Scale to zero.** Owner decision: minimum 1. It would also need lag mode (the only mode KEDA documents as
  scaling to zero), which does not suit our `XDEL`.
- **Node autoscaling (Cluster Autoscaler, Karpenter).** Out of scope by owner decision.

## Consequences
- **Everything is unverified.** The most likely first failures, in order: the image build (nsjail submodules and
  libraries), nsjail refusing to start in the pod (cgroup, userns, AppArmor), the full adversarial suite
  failing inside the pod, KEDA unable to resolve `addressFromEnv` through `envFrom`, and the NetworkPolicy
  blocking something the runner needs.
- **Scale-down kills a pod that may be judging.** The Deployment controller picks which pod to delete; it does
  not prefer idle ones. On `SIGTERM` the runner stops taking jobs and finishes the one it has (`Agent.Run` runs
  `Process` on a context that ignores the cancellation), then exits; `terminationGracePeriodSeconds` (330 s,
  an estimate from the language limits: Go compile 60 s plus tests) bounds the wait. `SIGKILL` after that leaves the
  job pending and `XAUTOCLAIM` gives it to another runner after `MinIdle` plus the reclaim interval (about
  90 s). One more case: a blocking read abandoned at shutdown can still receive a job (ADR 0028); it is then
  reclaimed the same way. Nothing is lost in any case; the verdict is only later.
- **Pending pods.** When pods outnumber what the fixed nodes can hold (a `t3.small` is expected to fit one pod at
  the default requests; not checked), the rest stay `Pending`, KEDA keeps asking for them, and the alert for "at maximum" does not count
  them. Raise `runner_node_count` and apply; nothing does it automatically.
- **No CPU limit** on the pod, on purpose: CFS throttling would distort the CPU time the sandbox measures. The
  sandbox bounds each job's CPU itself. The pod's memory limit bounds all sandboxes together (the job cgroups
  are children of the pod cgroup).
- **Consumer names.** Each pod registers a Redis consumer named after the pod; a deleted pod's consumer stays in
  the group (small memory, harmless to correctness, its pending entries are reclaimed). Nothing deletes them.
- **Node replacement.** The ASG has no instance refresh and no drain hook. A replaced node leaves a stale Node
  object; delete it by hand. Nodes boot stock Ubuntu and do not get the Ansible hardening role.
- **Image size.** About 1.5 GB (UNVERIFIED), so the first pod on a node waits for a pull. The tag is the commit
  sha and `imagePullPolicy` is the default (`IfNotPresent` for a non-`latest` tag).
- **Cost.** Each node is an EC2 instance, an EBS volume and a public IPv4 address (README cost table). Pods are
  free beyond the node.
- **Both runner paths can run at once** against the same consumer group. That is safe (the queue is built for
  many consumers) and doubles the idle polling; set `runner_count = 0` to use only pods.
- The CI image job runs on `main` only and cannot deploy anything. Merging this branch to `main` would build
  and push an image; the owner chose to push the branch only.
