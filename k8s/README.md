# k8s

Helm chart for the control host (k3s): `helm upgrade --install api k8s/charts/leetforce-api -n leetforce` (`scripts/k3s/deploy.sh` does this in CI). Check with `helm lint` and `helm template`.

| File | What |
|---|---|
| `namespace.yaml` | `leetforce` namespace, Pod Security `restricted` (applied with kubectl, outside the chart) |
| `charts/leetforce-api/` | Helm chart: API Deployment, ConfigMap, Service, Traefik Ingress; `values.yaml` holds image, config, resources, ingress host/TLS |

Not in the cluster: runners (separate EC2 hosts from the AMI, ADR 0021 follow-up), the web app
(planned for Vercel), Redis (Upstash) and Postgres (Neon).

Secrets are never in git. `scripts/k3s/sync-secrets.sh` builds the `api-env` and `ghcr-pull`
Secrets from SSM Parameter Store on the control host. The API image holds the hidden tests, so the
GHCR package must stay private.
