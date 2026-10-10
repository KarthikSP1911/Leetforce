#!/usr/bin/env bash
# Build the runner namespace's Secrets from SSM Parameter Store (Phase 17, ADR 0029).
# WRITTEN, NEVER RUN. Run on the control host, whose IAM role may read /leetforce/*.
# Secrets never touch git or a command line: values pass through mode 0600 temp files.
#   runner-env  every LEETFORCE_* parameter under /leetforce/runner, one env var each, plus
#               KEDA_REDIS_ADDRESS (host:port taken from LEETFORCE_REDIS_URL, not secret) which
#               KEDA reads through addressFromEnv. Other names (for example K3S_AGENT_TOKEN, the
#               agent join token that lives in the same SSM path) are skipped on purpose.
#   keda-redis  keys username and password from the same URL, for the TriggerAuthentication
#   ghcr-pull   image pull credentials, as sync-secrets.sh builds for the leetforce namespace
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
prefix="${LEETFORCE_SSM_PREFIX:-/leetforce}"
region="${AWS_REGION:-ap-south-1}"
ns=leetforce-runners
kubectl="${KUBECTL:-k3s kubectl}"
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

umask 077
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ssm() { aws ssm "$@" --region "$region" --with-decryption --output text; }

ssm get-parameters-by-path --path "$prefix/runner" --query 'Parameters[].[Name,Value]' |
  while IFS=$'\t' read -r name value; do
    key="${name##*/}"
    case "$key" in
      LEETFORCE_[A-Z0-9_]*) printf '%s=%s\n' "$key" "$value" ;;
    esac
  done >"$work/runner.env"

for need in LEETFORCE_REDIS_URL LEETFORCE_S3_ENDPOINT LEETFORCE_S3_BUCKET LEETFORCE_S3_USE_TLS; do
  grep -q "^$need=." "$work/runner.env" || { echo "sync-runner-secrets: $prefix/runner/$need is missing in SSM" >&2; exit 1; }
done

# redis[s]://[user[:password]@]host[:port][/db][?options]  ->  user, password (percent-decoded), host:port
url="$(sed -n 's/^LEETFORCE_REDIS_URL=//p' "$work/runner.env" | head -n1)"
rest="${url#*://}"
hostport="${rest##*@}"
hostport="${hostport%%[/?]*}"
cred=""
[[ "$rest" == *@* ]] && cred="${rest%@*}"
user="${cred%%:*}"
pass=""
[[ "$cred" == *:* ]] && pass="${cred#*:}"
[ -n "$user" ] || user=default
[[ "$hostport" == *:* ]] || hostport="$hostport:6379"
pass="$(printf '%b' "${pass//%/\\x}")"
if [ -z "$hostport" ] || [ -z "$pass" ]; then
  echo "sync-runner-secrets: could not read host and password from LEETFORCE_REDIS_URL" >&2
  exit 1
fi
printf 'KEDA_REDIS_ADDRESS=%s\n' "$hostport" >>"$work/runner.env"
printf '%s' "$user" >"$work/username"
printf '%s' "$pass" >"$work/password"

# The namespace manifest carries the Pod Security labels (the chart cannot own it, like namespace.yaml).
$kubectl apply -f "$here/k8s/runners-namespace.yaml" >/dev/null
$kubectl -n "$ns" create secret generic runner-env --from-env-file="$work/runner.env" \
  --dry-run=client -o yaml | $kubectl apply -f - >/dev/null
echo "runner-env: $(wc -l <"$work/runner.env") variables"
$kubectl -n "$ns" create secret generic keda-redis \
  --from-file=username="$work/username" --from-file=password="$work/password" \
  --dry-run=client -o yaml | $kubectl apply -f - >/dev/null
echo "keda-redis: written"

guser="$(ssm get-parameter --name "$prefix/deploy/ghcr_user" --query Parameter.Value 2>/dev/null || true)"
gtoken="$(ssm get-parameter --name "$prefix/deploy/ghcr_token" --query Parameter.Value 2>/dev/null || true)"
if [ -n "$guser" ] && [ -n "$gtoken" ]; then
  auth="$(printf '%s:%s' "$guser" "$gtoken" | base64 -w0)"
  printf '{"auths":{"ghcr.io":{"auth":"%s"}}}' "$auth" >"$work/dockerconfig.json"
  $kubectl -n "$ns" create secret generic ghcr-pull --type=kubernetes.io/dockerconfigjson \
    --from-file=.dockerconfigjson="$work/dockerconfig.json" --dry-run=client -o yaml | $kubectl apply -f - >/dev/null
  echo "ghcr-pull: written"
else
  echo "ghcr-pull: skipped (no ghcr_user/ghcr_token in SSM; fine if the image is public)" >&2
fi
