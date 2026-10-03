#!/usr/bin/env bash
# Build the cluster's Secrets from SSM Parameter Store. Run on the control host, which
# has the IAM role that may read /leetforce/*. Secrets never touch git or the command line.
#   api-env    every parameter under /leetforce/api, one env var per parameter name
#   ghcr-pull  image pull credentials from /leetforce/deploy/ghcr_user and ghcr_token
set -euo pipefail
prefix="${LEETFORCE_SSM_PREFIX:-/leetforce}"
region="${AWS_REGION:-ap-south-1}"
ns=leetforce
kubectl="${KUBECTL:-k3s kubectl}"

umask 077
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ssm() { aws ssm "$@" --region "$region" --with-decryption --output text; }

ssm get-parameters-by-path --path "$prefix/api" --query 'Parameters[].[Name,Value]' |
  while IFS=$'\t' read -r name value; do printf '%s=%s\n' "${name##*/}" "$value"; done >"$work/api.env"

for need in DATABASE_URL LEETFORCE_REDIS_URL LEETFORCE_S3_BUCKET; do
  grep -q "^$need=." "$work/api.env" || { echo "sync-secrets: $prefix/api/$need is missing in SSM" >&2; exit 1; }
done

$kubectl create namespace "$ns" --dry-run=client -o yaml | $kubectl apply -f - >/dev/null
$kubectl -n "$ns" create secret generic api-env --from-env-file="$work/api.env" \
  --dry-run=client -o yaml | $kubectl apply -f - >/dev/null
echo "api-env: $(wc -l <"$work/api.env") variables"

user="$(ssm get-parameter --name "$prefix/deploy/ghcr_user" --query Parameter.Value 2>/dev/null || true)"
token="$(ssm get-parameter --name "$prefix/deploy/ghcr_token" --query Parameter.Value 2>/dev/null || true)"
if [ -n "$user" ] && [ -n "$token" ]; then
  auth="$(printf '%s:%s' "$user" "$token" | base64 -w0)"
  printf '{"auths":{"ghcr.io":{"auth":"%s"}}}' "$auth" >"$work/dockerconfig.json"
  $kubectl -n "$ns" create secret generic ghcr-pull --type=kubernetes.io/dockerconfigjson \
    --from-file=.dockerconfigjson="$work/dockerconfig.json" --dry-run=client -o yaml | $kubectl apply -f - >/dev/null
  echo "ghcr-pull: written"
else
  echo "ghcr-pull: skipped (no ghcr_user/ghcr_token in SSM; fine if the image is public)" >&2
fi
