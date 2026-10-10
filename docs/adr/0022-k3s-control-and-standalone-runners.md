# ADR 0022: k3s for the control host, standalone runner hosts, SSM for secrets, SSM Run Command for deploys

Status: accepted for the code; not yet exercised in the cloud (Phase 13 in progress). Partly superseded by [ADR 0029](0029-keda-scaled-runner-pods.md) (Phase 17, proposed, unverified): runners may also run as KEDA-scaled pods on k3s agent nodes; the standalone runner hosts below remain.

## Context
Phase 13 puts the flow in the cloud and must survive losing a runner. The owner chose GHCR for images, the web app on Vercel later, and testing access restricted to the owner's IP. Runners execute untrusted code in nsjail, which needs namespaces and cgroups that the Phase 6 unprivileged systemd unit already provides.

## Decision
- **Control host:** one `t3.small` running k3s (pinned `v1.37.1+k3s1`, installed by the `k3s_server` Ansible role). It runs the API behind the bundled Traefik. Manifests are a Kustomize base in `k8s/base` (namespace with Pod Security `restricted`, non-root numeric user, read-only root filesystem, probes). The web app is not in the cluster (Vercel later) and Redis and Postgres stay hosted (Upstash, Neon).
- **Runners:** not k3s agents. Each is an EC2 host started from the AMI with the `leetforce-runner` systemd unit, in an Auto Scaling group (`runner_count`, EC2 health checks) so a lost host is replaced. First boot reads `/leetforce/runner/*` from SSM into `/etc/leetforce/runner.env` and enables the service.
- **Secrets:** SSM Parameter Store (SecureString). `scripts/k3s/push-ssm.sh` writes (dry run by default), and `scripts/k3s/sync-secrets.sh` on the control host builds the `api-env` and `ghcr-pull` Secrets. Nothing is stored in git or on a command line.
- **Deploys:** GitHub Actions builds and scans the image, pushes to private GHCR, then runs `scripts/k3s/deploy.sh <sha>` on the control host through SSM Run Command, using a GitHub OIDC role (no stored AWS keys) trusted only for the `production` environment.

## Alternatives
- Runners as pods: the sandbox would need privileged containers and the Phase 6 hardening would be redone.
- Runners as k3s agents on their own hosts: adds a cluster control path to hosts that run untrusted code, for no gain at this size.
- SSH from CI: the security groups only admit the owner's IP, and a stored key would be a long-lived secret.
- External Secrets operator: more moving parts than a short script for three secrets.
- EKS: a recurring control-plane fee.

## Consequences
- Found by testing on a throwaway kind cluster: the kubelet cannot verify a distroless user given by name, so `runAsUser: 65532` is numeric.
- The control host's IMDS hop limit is 2 so pods can use the instance role; a compromised API pod could reach that role (SSM read, S3 read/write on `problems/`). Runner hosts keep hop limit 1.
- Anyone who can approve a deployment to the `production` environment can run commands on the control host as root through SSM; add required reviewers.
- The ASG does not drain in-flight jobs: they are reclaimed through `XAUTOCLAIM` after `LEETFORCE_JOB_MIN_IDLE` (30 s default), with a delivery limit. EC2-only health checks do not replace a running host whose service is dead.
- The API image contains the hidden tests, so the GHCR package must stay private.
- Open: the AMI has not been built yet (three attempts failed: build tooling, then a scan timeout, then base-image vulnerabilities), so none of this has run on real hosts.
