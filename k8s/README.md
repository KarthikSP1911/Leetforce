# k8s

Manifests for the control host (k3s). `base/` is a Kustomize base: `kubectl apply -k k8s/base`.

| File | What |
|---|---|
| `base/namespace.yaml` | `leetforce` namespace, Pod Security `restricted` |
| `base/api-*.yaml` | API Deployment, ConfigMap, Service, Traefik Ingress |

Not in the cluster: runners (separate EC2 hosts from the AMI, ADR 0021 follow-up), the web app
(planned for Vercel), Redis (Upstash) and Postgres (Neon).

Secrets are never in git. `scripts/k3s/sync-secrets.sh` builds the `api-env` and `ghcr-pull`
Secrets from SSM Parameter Store on the control host. The API image holds the hidden tests, so the
GHCR package must stay private.
