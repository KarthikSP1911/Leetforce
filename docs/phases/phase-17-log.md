# Phase 17 log: KEDA-scaled runner pods (post-launch extension, outside PLAN.md)

**This is not a PLAN.md phase.** `docs/PLAN.md` ends at Phase 16 and `docs/PROGRESS.md` said "there is no Phase 17". This is a post-launch extension requested by the owner, numbered 17 only so the branch, commits and `Refs:` footers have a name. It has no exit criteria from the plan, no phase report and no phase summary.

**Every claim in this log is "written, not run".** Nothing was built, validated, planned, applied, linted, tested or executed. No Docker, Terraform, Ansible, Helm, kubectl, Packer, AWS CLI, ssh, make or Go command was run, and the dev host was not used. The only commands that ran were `git` (status, log, diff, branch, add, commit, push), `ls`/`cat`/`find`/`grep`/`sed`/`printf` on local files (including `sed -i` and `printf >>` edits to files in this repo), one `pkill` of the hung interpreter described under "Mistakes", and reads of public documentation (KEDA 2.20 scaler docs and scaler source, release list). The only validation was re-reading files. Where this log says a file "does" something, read it as "is written to do".

Running log, by Claude, 2026-10-10, on the owner's explicit go-ahead prompt (recap question skipped, plan posted, then started). Branch `feat/17-keda-runners` off `main`. No secrets, keys or public addresses are written here or in any file of the branch; the only IP-like literals are RFC 1918 and link-local ranges, `169.254.169.254`, and the pre-existing documentation address `203.0.113.7` in `terraform.tfvars.example`.

## Rules the owner set (and how they were kept)

| Rule | Kept? |
|---|---|
| Write every file, execute nothing | Yes, with two slips recorded under "Mistakes" (an empty `python`/`python3 -` invocation that hung and was stopped; git's own commit hooks) |
| Nodes: k3s agents on EC2, joined to the existing control host; partly supersedes ADR 0022 | `infra/aws/runner-k3s.tf`, `ansible/roles/k3s_agent`; ADR 0022 status line points to ADR 0029 |
| Node count FIXED by `runner_node_count`, default 0; KEDA scales pods; Pending pods documented; no node autoscaling | Yes. Every resource in `runner-k3s.tf` has `count = 0` at 0. Documented in the ADR, chart values, `infra/aws/README.md`, `k8s/README.md` |
| Least privilege first, no privileged flag; if impossible say so and record a privileged fallback on the isolated pool; gVisor not the default | Default `sandbox.privileged: false`; the ADR states plainly that it is plausible and unproven, and records `sandbox.privileged: true` as the fallback |
| KEDA `minReplicaCount` 1; max, polling, cooldown are values | `keda.*` in `values.yaml`. Note: KEDA documents that `cooldownPeriod` only applies when scaling to zero, so it is inert at min 1 (said in values and the ADR) |
| Push the BRANCH only; no merge to main (a main push triggers deploy.yml); no tag | Done at the end (see "Branch and push") |
| Inspect `git diff --staged` before each commit; `git update-index --chmod=+x` for new scripts | Done; one mode slip fixed (see Mistakes) |
| No brand or logo files touched; no secrets in any file | Yes |

## Decisions

Made by the owner (in the prompt): the node and scaling model, the privilege order, `minReplicaCount: 1`, values-based knobs, branch-only push, the docs to update, the "must be verified" list.

Made by Claude as defaults, all open to change (also in ADR 0029):

| Decision | Choice | Why in one line |
|---|---|---|
| KEDA scaler mode | `redis-streams` on `leetforce:jobs`, no consumer group, `streamLength: 2` | Runners `XDEL` finished jobs, so `XLEN` = waiting + in flight; `XPENDING` misses waiting jobs; lag mode is unreliable with `XDEL` |
| Caching | `useCachedMetrics: true` | Without it every 15 s HPA sync runs a new `XLEN` (5760 a day) |
| `pollingInterval` / `maxReplicaCount` | 60 s / 4 | About 1.4k Redis commands a day for KEDA; four busy pods all day is about 19k a day (above the believed free limit) |
| Capabilities of the entrypoint | `SYS_ADMIN CHOWN SETUID SETGID SETPCAP`, all dropped before the runner starts (`setpriv`) | Smallest set found for: remount cgroup rw, delegate it, change user, empty the bounding set |
| Seccomp / AppArmor | `RuntimeDefault` / Localhost `leetforce-runner-pod` | The owner allowed "unconfined as needed"; this is stricter; the values switch to `Unconfined` seccomp if needed |
| Pod Security | namespace `enforce: privileged`, `warn`/`audit: baseline` | Added capabilities and a Localhost AppArmor profile are admitted by neither baseline nor restricted |
| S3 credentials | node instance role via IMDS, hop limit 2 on the agents, NetworkPolicy allows only `169.254.169.254:80` | No access keys to manage; the one place looser than the standalone runners (hop limit 1) |
| Node role | SSM read of the single parameter `/leetforce/runner/K3S_AGENT_TOKEN` plus the same S3 read policy as the runners | Narrower than `/leetforce/runner/*`: pods get the Redis URL from a Kubernetes Secret, not SSM |
| Join token | stored in SSM under `/leetforce/runner/`, copied by `push-agent-token.sh` | In the path the owner named; the Secret sync skips every non-`LEETFORCE_*` name so it never reaches a pod |
| Node boot | user data installs k3s and the AppArmor profile on stock Ubuntu; no Ansible run on ASG nodes | No AMI exists (three failed builds); the Ansible role covers live hosts and a future AMI |
| Instance refresh | none on the agent ASG | A refresh kills pods undrained |
| Deploy | by hand with `scripts/k3s/deploy-runners.sh`; CI only builds, scans and pushes the image | A first rollout of untested code should be watched |
| Grace period | 330 s (estimate: Go compile 60 s + about 40 tests x 5 s + margin) | Not measured; `XAUTOCLAIM` is the backstop |
| Pod resources | requests 500m / 768 Mi, memory limit 1 Gi, no CPU limit | CFS throttling would distort measured runtime |
| `runner-ASG` default | untouched (`runner_count` stays 1) | Both paths can run; set it to 0 to use pods only |
| Prometheus | pod scrape through the k3s API server proxy, in a separate, not-loaded file | The pod network is unreachable from the owner's PC; a missing credentials file would stop the live Prometheus loading |

## Units of work (all on one branch, committed in order)

| # | Commit | Files | Written, not run |
|---|---|---|---|
| 1 | `build(runner)`: image | `runner/Dockerfile`, `runner/Dockerfile.dockerignore`, `runner/docker-entrypoint.sh`, `scripts/check-nsjail-pin.sh` | Multi-stage build: nsjail from the pinned commit (with submodules), static runner, python3 / g++ / JDK 21 / Go at the `lang.go` paths, user `lfrunner` 10001. The entrypoint remounts its cgroup rw, chowns it, drops every capability with `setpriv`, execs the runner. The `.dockerignore` is `Dockerfile.dockerignore` next to the Dockerfile (BuildKit's per-Dockerfile form, as the API uses) |
| 2 | `feat(k8s)`: namespace | `k8s/runners-namespace.yaml` | `leetforce-runners`, Pod Security enforce `privileged` |
| 3 | `feat(k8s)`: chart | `k8s/charts/leetforce-runner/**` | Deployment, NetworkPolicy, ScaledObject + TriggerAuthentication. The chart never creates a Secret |
| 4 | `feat(ansible)`: role | `ansible/roles/k3s_agent/**`, `ansible/site.yml`, `ansible/inventory/hosts.ini.example` | AppArmor profile, token from SSM into a 0600 file, k3s config with label and taint, pinned install; group `k3s_agents`; hardening role already covers `all` |
| 5 | `feat(infra)`: nodes | `infra/aws/runner-k3s.tf`, `k3s-agent-userdata.sh.tftpl`, `variables.tf`, `outputs.tf`, `terraform.tfvars.example`, `README.md`, `scripts/k3s/push-agent-token.sh`, `scripts/check-nsjail-pin.sh` (now also the k3s pin) | Launch template + ASG, SG, IAM, IMDS hop limit 2 with reasoning in a comment; `infra/neon` untouched; no word "neon" in the new files (the isolation test greps for it) |
| 6 | `feat(k8s)`: scripts | `scripts/k3s/sync-runner-secrets.sh`, `scripts/k3s/deploy-runners.sh` | Secrets from SSM through temp files; KEDA install at chart 2.20.2 (UNVERIFIED version) then the chart, rollback on failure |
| 7 | `ci(ci)`: image job | `.github/workflows/deploy.yml` | Job `runner-image`: build, Trivy, push to private GHCR on main only |
| 8 | `build(ci)`: make | `Makefile` | `build-runner-image`, `lint-runner-chart` (defined only) |
| 9 | `feat(obs)` | `observability/prometheus/k3s-runner-pods.yml`, `k3s-runner-pods-rbac.yaml`, `alerts.yml`, `prometheus.yml` (comment only), `.gitignore` | Scrape through the API server proxy (not loaded), RBAC, the alert `RunnerPodsAtMaxQueueDeep` + a recording rule holding the configured maximum |
| 10 | `docs(docs)` | `docs/adr/0029-*.md`, `docs/adr/0022-*.md`, `docs/FLOW.md`, `docs/PROGRESS.md`, `docs/launch-checklist.md` (section E), `README.md` (cost table, documentation table), `k8s/README.md`, `CLAUDE.md` | Plus this log in the last commit |

## Redis command budget (summary; the working is in ADR 0029)

One pod idle: about 2.9k commands a day (2 per `LEETFORCE_JOB_RECLAIM_EVERY` of 60 s, ADR 0028). KEDA at 60 s with caching: about 1.4k a day (one `XLEN` a minute); without `useCachedMetrics` it would be about 7.2k. A job: about 10 commands. One pod replacing the standalone runner: about 10.4k a day total (about 0.31M a month); four pods busy all day: about 19.1k a day (about 0.57M a month). All computed from the code and intervals, **not measured**.

## Claims register

| Claim | Where made | Basis | Status |
|---|---|---|---|
| `redis-streams` without `consumerGroup` uses `XLEN`; with a group `XPENDING`; with `lagCount` `INFO` + `XINFO GROUPS` | ADR 0029, scaledobject.yaml | KEDA 2.20 docs and scaler source, read while writing | Read, not run |
| `useCachedMetrics` limits scaler queries to the polling interval | ADR 0029 | KEDA docs say it "caches metric values during polling interval"; the 15 s HPA sync is also from the docs | Read, not run |
| `cooldownPeriod` applies only to scaling to zero | values.yaml, ADR | KEDA docs, quoted | Read, not run |
| Runners `XACK` + `XDEL`, so `XLEN` = waiting + in flight | ADR | `queue/queue.go` and FLOW stage 7, read | Read, not run |
| On SIGTERM the runner finishes its job | values.yaml, ADR | `runner/internal/agent/agent.go` `Run` / `Process`, read | Read, not run |
| Kubernetes rejects `allowPrivilegeEscalation: false` with `CAP_SYS_ADMIN` | deployment.yaml | Remembered from the API validation rules | **Unchecked** |
| containerd's default seccomp profile allows `mount`/`unshare`/`clone` NEW* when `CAP_SYS_ADMIN` is in the container's set at start | values.yaml, ADR | Remembered from how the profile is generated | **Unchecked** |
| Remounting `/sys/fs/cgroup` rw from `CAP_SYS_ADMIN` in a container works, and chown of the cgroup files delegates it | entrypoint, ADR | A known recipe; not tried | **Unchecked** |
| `setpriv --reuid ... --bounding-set=-all ...` in one call drops the bounding set without `EPERM` | entrypoint | Not verified; the entrypoint fails closed if it errors | **Unchecked** |
| KEDA resolves `addressFromEnv` through the container's `envFrom` secretRef | scaledobject.yaml | Remembered from the KEDA resolver | **Unchecked** |
| KEDA Helm chart `2.20.2` exists | deploy-runners.sh | A web search result for the newest release; chart numbering assumed to follow | **Unchecked** |
| `alpine/helm:3.16.2` exists | Makefile | Assumed | **Unchecked** |
| k3s `v1.37.1+k3s1` supports `appArmorProfile`, `token-file`, `node-taint` in config.yaml | ansible role, userdata | Same version string the server role pins; keys remembered | **Unchecked** |
| `storage.Open` reaches the node role through IMDSv2 from a pod | values.yaml, TF comment | `credentials.NewIAM("")` in `storage/storage.go`; whether minio-go v7.3.0 speaks IMDSv2 was not checked (the same assumption underlies the standalone runners' `http_tokens = required`) | **Unchecked** |
| AppArmor profile syntax (`abi <abi/4.0>`, `mediate_deleted`, bare `mount,` / `userns,`) parses on Ubuntu 24.04 | the profile file | Modelled on `scripts/runner/usr.local.bin.nsjail` | **Unchecked** |
| Idle command counts and the Upstash limit (about 0.5M a month) | ADR | ADR 0028 arithmetic; the limit was never checked (launch checklist B2) | Computed, not measured |
| Prices (about $15.2 / $1.8 / $3.7 a month per node) | README | The README's own UNVERIFIED list prices | UNVERIFIED |

## Mistakes and notes (honest)

- **Two accidental empty interpreter invocations.** While trying to patch a YAML line I ran `python3 -` (with a heredoc that was empty) and, later, `python -` in the Bash tool. Both hung waiting for input and were stopped with TaskStop; they printed nothing and changed nothing. They were not part of any plan, ran no project code and verify nothing. The `sed` they were chained to did not run in the first case (checked by reading the file); the line was then edited with the Edit tool.
- **Git's own hooks ran on every commit** (`.husky/pre-commit`: lint-staged, which matched no staged file because it only covers `web/`; `.husky/commit-msg`: commitlint on the message). I did not start them and did not bypass them. The first commit attempt was rejected for a body line over 72 characters and retried with a rewrapped message. A later commit printed a commitlint warning (a `path:` token in the body read as a footer) and succeeded; the message is intact.
- **The Bash tool failed to parse several long heredocs** ("unexpected EOF"); nothing ran in those cases. Those files were written with the Write tool instead.
- **Mode bit.** `scripts/k3s/push-agent-token.sh` was first committed as 100644 because `git reset` dropped the earlier `--chmod=+x`; the next commit corrected it to 100755.
- **Found, not fixed (out of scope):** `scripts/k3s/deploy.sh` line 33 contains a literal backslash-n in the `helm upgrade` command (`-n leetforce \n    --set ...`), which would pass a stray `n` argument and break the API deploy. It predates this branch and was not touched.
- **Found, not decided:** the Packer `ansible-local` step lists only the `hardening` and `runner_host` roles in `role_paths`, while `site.yml` also names `k3s_server` (existing) and now `k3s_agent`. Whether Ansible resolves roles of plays whose hosts do not match was not checked; it could affect the AMI build, which already failed three times for other reasons.
- **Known gaps in what was written:** no default-deny NetworkPolicy for the `leetforce` namespace (ADR 0029 consequence); no alert unit test for `RunnerPodsAtMaxQueueDeep`; no drain hook for node replacement; no cleanup of dead pods' Redis consumers; the adversarial suite has no in-pod runner (it needs a test binary built with `-tags adversarial`).

## Must be verified before real use

None of this has been done. The commands are what someone would run; each needs the owner's go-ahead where it is billable (launch checklist, section E).

1. **The image builds.** `make build-runner-image`. Check: the nsjail submodule fetch at the pinned commit, the Ubuntu package names (`libprotobuf32t64`, `libnl-route-3-200`), `/usr/lib/jvm/java-21-openjdk-amd64` and `/etc/java-21-openjdk` exist, `/usr/local/go/bin/go version`, image size, and `scripts/check-nsjail-pin.sh` passes. Then `trivy image` and `trivy config k8s/ infra/ ansible/`.
2. **nsjail starts in a pod.** On a real agent node: the entrypoint logs "delegated ... controllers: memory pids cpu", `/proc/1/status` shows `CapEff`, `CapPrm`, `CapAmb`, `CapBnd` all 0 and `NoNewPrivs: 1`, the runner judges a trivial Python program to AC. If the AppArmor profile is rejected (`apparmor_parser -r`), nothing starts.
3. **The FULL adversarial suite passes in-pod.** This is the gate for any sandbox change (CLAUDE.md). Build the suite as a test binary (`go test -c -tags adversarial ./judge/sandbox`) into a test image, run it in a pod with the same `securityContext` as uid 10001, and run `make test-sandbox` equivalents (AC, WA, TLE, MLE, RE, CE for python, cpp, java, go, as `scripts/test-runner-unprivileged.sh` did on the host). A failure of any test blocks use of the pod path. If the least-privilege path cannot pass, switch `sandbox.privileged` to `true` and run it again.
4. **The cgroup is writable and delegated.** In the pod: the controllers `memory pids cpu` are enabled for the pod cgroup by the kubelet; the runner creates its `runner` leaf and `jobs`; `memory.max`, `pids.max`, `memory.peak` and `cgroup.kill` work; a fork bomb and a memory bomb are contained and leave no cgroup behind. Check there is no other process in the container cgroup (the HTTP probes must not become exec probes).
5. **KEDA scales up and down.** KEDA installs at the pinned chart; `kubectl -n leetforce-runners get scaledobject,hpa`; the trigger reports no error (so `addressFromEnv` resolved and TLS works); enqueue about 20 jobs (`lfq enqueue` or the load test) and watch replicas go 1 to the maximum; with the nodes full, extra pods stay Pending; after the queue drains and the 300 s window, replicas step down one per 120 s.
6. **Scale-down finishes or reclaims the job.** Start a long job, delete the pod it runs on: the log shows the job completing and "runner stopped" within 330 s with exactly one verdict. Then force the case where the grace period is exceeded (kill the pod with a short grace) and confirm `XAUTOCLAIM` gives the job to another runner after about 90 s and still exactly one verdict.
7. **Redis command usage.** With one pod idle for an hour, read the Upstash usage page (or `INFO commandstats`) and compare with about 2.9k a day per pod plus about 1.4k a day for KEDA; repeat with `useCachedMetrics` off to see the difference; confirm the monthly plan limit (checklist B2).
8. **Terraform.** `make tf-validate`; `terraform plan` for `infra/aws` with `runner_node_count = 0` (no resource from `runner-k3s.tf`, only new outputs and the `control_private_ip` output) and with `1`; `make test-destroy-isolation` (the new files must not make it fail); confirm `infra/neon` is untouched. Check the `for_each` expression, the `count`-indexed references and the `file()` path to the AppArmor profile.
9. **Ansible.** `make lint-ansible`; run the `k3s_agent` role on a throwaway host with `k3s_agent_join: false` first; check the Packer `role_paths` question above.
10. **Chart and manifests.** `make lint-runner-chart`; `helm template` diff by eye; `kubeconform` with KEDA's CRD schemas; apply to a throwaway cluster and confirm Pod Security admits the pod and warns at baseline.
11. **Network.** From a runner pod: Redis and S3 reachable, DNS works, the control host, the API pod and a private address are not; `169.254.169.254` answers (IMDSv2, hop limit 2) and S3 reads work with the node role through `storage.Open`; confirm k3s' embedded policy controller enforces egress rules; confirm a judged program still has no network.
12. **Node join.** `push-agent-token.sh` writes the token; the node joins within minutes with the label and taint; kubelet's NodeRestriction accepts the `leetforce.dev/` label; the control SG rules are enough (VXLAN, 6443, 10250); a replaced node leaves a stale Node object.
13. **Observability.** `make test-alerts` accepts the new rule file; load the scrape file with real credentials; the RBAC allows `pods/proxy`; the API server proxy reaches the pod through the NetworkPolicy; the alert fires when the pods are at the maximum and the queue is deep.
14. **CI.** Syntax check the workflow; the `runner-image` job passes Trivy (a fixable HIGH finding fails it); the pushed image is private; the job does not run on a branch.
15. **Cost.** The real per-node cost on the AWS bill (instance, EBS, public IPv4), the GHCR allowance, and the Upstash plan against the budget in ADR 0029.

## Riskiest assumptions

1. **The least-privilege sandbox works inside a pod.** That a root entrypoint with five capabilities, containerd's default seccomp, a permissive Localhost AppArmor profile and a private cgroup namespace allow: remounting the cgroup filesystem read-write, delegating it to uid 10001, nsjail's user namespace with new mounts, and the runner's own cgroup management. Any one of these could fail, and the privileged fallback is a real security downgrade (ADR 0029).
2. **KEDA sees the queue as designed and costs what the arithmetic says.** That `addressFromEnv` resolves through `envFrom`, that `useCachedMetrics` really limits Redis queries to the polling interval, and that `XLEN` stays equal to waiting plus in-flight jobs (dead letters and any entry not `XDEL`ed would inflate it). Otherwise scaling is wrong or the Upstash budget burns faster than ADR 0029 states.
3. **The node role reaches S3 from a pod.** That the minio-go IAM provider in `storage.Open` gets credentials from IMDSv2 across the pod hop with hop limit 2 and the NetworkPolicy exception. If it only speaks IMDSv1, or the policy blocks it, every pod fails to load problems, and the only fix is access keys in the Secret or a code change.

## Branch and push

Branch `feat/17-keda-runners` off `main`. No merge into `main` (a push to `main` triggers `deploy.yml`), no tag. Pushed as a branch only; the push result is recorded in the final message of the session, not here, because it happens after this file is committed.

## File and path index

Generated from `git diff --name-status main..HEAD` (the log itself is added in the last commit).

| Status | Path | Purpose |
|---|---|---|
| A | `runner/Dockerfile` | Runner image |
| A | `runner/Dockerfile.dockerignore` | Excludes problems, tests, other components |
| A | `runner/docker-entrypoint.sh` | Cgroup setup, capability drop, exec runner |
| A | `scripts/check-nsjail-pin.sh` | nsjail commit and k3s version parity check |
| A | `k8s/runners-namespace.yaml` | `leetforce-runners`, Pod Security |
| A | `k8s/charts/leetforce-runner/Chart.yaml` | Chart metadata |
| A | `k8s/charts/leetforce-runner/values.yaml` | All settings, with reasons |
| A | `k8s/charts/leetforce-runner/templates/_helpers.tpl` | Labels |
| A | `k8s/charts/leetforce-runner/templates/deployment.yaml` | Runner pods |
| A | `k8s/charts/leetforce-runner/templates/networkpolicy.yaml` | Egress allow-list |
| A | `k8s/charts/leetforce-runner/templates/scaledobject.yaml` | KEDA ScaledObject + TriggerAuthentication |
| A | `ansible/roles/k3s_agent/defaults/main.yml` | Pinned version, label, taint |
| A | `ansible/roles/k3s_agent/files/leetforce-runner-pod` | AppArmor profile (single source) |
| A | `ansible/roles/k3s_agent/handlers/main.yml` | Load the profile |
| A | `ansible/roles/k3s_agent/tasks/main.yml` | Join the node |
| M | `ansible/site.yml` | New play for `k3s_agents` |
| M | `ansible/inventory/hosts.ini.example` | New group |
| A | `infra/aws/runner-k3s.tf` | Node ASG, SG, IAM |
| A | `infra/aws/k3s-agent-userdata.sh.tftpl` | First-boot join |
| M | `infra/aws/variables.tf` | `runner_node_*` variables |
| M | `infra/aws/outputs.tf` | Control private IP, ASG, SG outputs |
| M | `infra/aws/terraform.tfvars.example` | Commented examples |
| M | `infra/aws/README.md` | Section for the node group |
| A | `scripts/k3s/push-agent-token.sh` | Token to SSM |
| A | `scripts/k3s/sync-runner-secrets.sh` | Runner Secrets from SSM |
| A | `scripts/k3s/deploy-runners.sh` | KEDA + chart rollout |
| M | `.github/workflows/deploy.yml` | `runner-image` job |
| M | `Makefile` | `build-runner-image`, `lint-runner-chart` |
| A | `observability/prometheus/k3s-runner-pods.yml` | Pod scrape config (not loaded) |
| A | `observability/prometheus/k3s-runner-pods-rbac.yaml` | Scraper RBAC |
| M | `observability/prometheus/alerts.yml` | Recording rule + alert |
| M | `observability/prometheus/prometheus.yml` | Comment pointing to the scrape file |
| M | `.gitignore` | `observability/prometheus/k3s/` credentials folder |
| A | `docs/adr/0029-keda-scaled-runner-pods.md` | The decision |
| M | `docs/adr/0022-k3s-control-and-standalone-runners.md` | Status line pointer |
| M | `docs/FLOW.md` | Phase 17 row and section (UNVERIFIED) |
| M | `docs/PROGRESS.md` | Resume point = the verification list |
| M | `docs/launch-checklist.md` | Section E |
| M | `README.md` | Cost table rows, documentation table |
| M | `k8s/README.md` | Runner pods section |
| M | `CLAUDE.md` | Two make targets, current-state note |
| A | `docs/phases/phase-17-log.md` | This file |

Left alone on purpose: `go.work.sum` (modified in the working tree before this session, not mine, never staged), everything under `web/` and `infra/neon`, all brand and logo files.
