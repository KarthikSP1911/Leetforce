# k8s

Helm chart for the control host (k3s): `helm upgrade --install api k8s/charts/leetforce-api -n leetforce` (`scripts/k3s/deploy.sh` does this in CI). Check with `helm lint` and `helm template`.

| File | What |
|---|---|
| `namespace.yaml` | `leetforce` namespace, Pod Security `restricted` (applied with kubectl, outside the chart) |
| `charts/leetforce-api/` | Helm chart: API Deployment, ConfigMap, Service, Traefik Ingress; `values.yaml` holds image, config, resources, ingress host/TLS |
| `runners-namespace.yaml` | `leetforce-runners` namespace, Pod Security `privileged` (enforce) with `baseline` warn and audit; applied with kubectl by `scripts/k3s/deploy-runners.sh` (Phase 17, never applied) |
| `charts/leetforce-runner/` | Helm chart (Phase 17, written and never rendered or applied): runner Deployment on the tainted runner nodes, egress NetworkPolicy, KEDA `ScaledObject` and `TriggerAuthentication`; see [ADR 0029](../docs/adr/0029-keda-scaled-runner-pods.md) |

Not in the cluster: the standalone runner hosts (separate EC2 hosts from the AMI, ADR 0022), the web app
(planned for Vercel), Redis (Upstash) and Postgres (Neon). Since Phase 17 runners can also be pods in the
`leetforce-runners` namespace on dedicated k3s agent nodes, scaled by KEDA (ADR 0029). That path is
UNVERIFIED: nothing in it has been rendered, built, applied or run.

## Runner pods (Phase 17, unverified)

- KEDA scales **pods** only. The node count is fixed by `runner_node_count` in `infra/aws`; pods that do not fit stay `Pending`.
- Secrets are never in the chart: `runner-env`, `keda-redis` and `ghcr-pull` in `leetforce-runners` come from `scripts/k3s/sync-runner-secrets.sh` (SSM).
- Rollout is by hand on the control host: `scripts/k3s/deploy-runners.sh <sha>` (installs KEDA once, then the chart). CI only builds, scans and pushes the image.
- `sandbox.privileged: true` is the documented fallback if the least-privilege path does not pass the in-pod adversarial suite.
- Check with `make lint-runner-chart` (defined, never run).

Secrets are never in git. `scripts/k3s/sync-secrets.sh` builds the `api-env` and `ghcr-pull`
Secrets from SSM Parameter Store on the control host. The API image holds the hidden tests, so the
GHCR package must stay private.
