# 0026. Helm chart for the API (replaces the Kustomize base)

Status: ACCEPTED (owner asked for Helm charts "wherever needed", after Phase 16)

## Context
ADR 0022 deployed the API with a Kustomize base. CI changed the image tag by editing a copy of
`kustomization.yaml` with `sed`. The owner asked for Helm charts wherever they are needed.

## Decision
- `k8s/charts/leetforce-api` is the API chart (Deployment, ConfigMap, Service, Ingress). Image
  repository and tag, config keys, resources, replicas and the Ingress host/TLS are `values.yaml`.
  `scripts/k3s/deploy.sh` runs `helm upgrade --install ... --set image.tag=<sha>` and
  `helm rollback` on a failed rollout.
- The Namespace (Pod Security `restricted`) stays a plain manifest, `k8s/namespace.yaml`, applied
  with kubectl: `sync-secrets.sh` creates the namespace before the chart exists and Helm refuses to
  adopt a resource it did not create.
- Secrets stay outside the chart (`sync-secrets.sh` from SSM); the chart only names them.
- The `helm` binary (pinned, checksum-verified tarball) is installed on the control host by the
  `k3s_server` Ansible role.
- Only the API needs a chart. Runners are systemd services (ADR 0022), the web app is for Vercel,
  and Prometheus/Grafana/Loki run in Compose. If observability moves into k3s, use upstream charts
  (kube-prometheus-stack, loki) with our values rather than hand-written manifests.

## Alternatives
- Keep Kustomize: fewer moving parts, but image tag by `sed` and no real parameters.
- `k3s` HelmChart CRD: no CLI needed, but the rollback and `--set` flow is less direct.

## Consequences
- One more tool (`helm`) on the control host; `helm lint` / `helm template` check the chart.
- A config change rolls pods through a `checksum/config` annotation.
- Not yet run against a real cluster: the cloud is unproven (Phase 13). Verified with `helm lint`
  and `helm template` only (alpine/helm 3.16.2 in Docker).
