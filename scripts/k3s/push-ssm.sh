#!/usr/bin/env bash
# Write the API's settings from the local .env into SSM Parameter Store (SecureString).
# Run on the owner's machine with AWS_PROFILE set. Dry run by default: it prints the
# parameter NAMES it would write and never a value. Pass --apply to write.
#
# Layout (names are the environment variables the API reads):
#   /leetforce/api/DATABASE_URL            from .env
#   /leetforce/api/LEETFORCE_REDIS_URL     from .env
#   /leetforce/api/LEETFORCE_S3_BUCKET     terraform output in infra/bootstrap
#   /leetforce/deploy/ghcr_user            $GHCR_USER   (optional)
#   /leetforce/deploy/ghcr_token           $GHCR_TOKEN  (optional, read:packages only)
# Runner settings live under /leetforce/runner/* (the runner host role can read only those).
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
prefix="${LEETFORCE_SSM_PREFIX:-/leetforce}"
region="${AWS_REGION:-ap-south-1}"
apply=0
[ "${1:-}" = "--apply" ] && apply=1

env_get() { # env_get NAME: value of NAME from .env without sourcing the file
  sed -n "s/^$1=//p" "$here/.env" | head -n1 | sed -e "s/^'//" -e "s/'$//" -e 's/^"//' -e 's/"$//'
}

declare -A params
params["$prefix/api/DATABASE_URL"]="$(env_get DATABASE_URL)"
params["$prefix/api/LEETFORCE_REDIS_URL"]="$(env_get LEETFORCE_REDIS_URL)"
params["$prefix/api/LEETFORCE_S3_BUCKET"]="$(terraform -chdir="$here/infra/bootstrap" output -raw bucket)"
[ -n "${GHCR_USER:-}" ] && params["$prefix/deploy/ghcr_user"]="$GHCR_USER"
[ -n "${GHCR_TOKEN:-}" ] && params["$prefix/deploy/ghcr_token"]="$GHCR_TOKEN"

status=0
for name in "${!params[@]}"; do
  if [ -z "${params[$name]}" ]; then
    echo "MISSING: $name has no value" >&2
    status=1
    continue
  fi
  if [ "$apply" = 1 ]; then
    aws ssm put-parameter --region "$region" --name "$name" --type SecureString \
      --overwrite --value "${params[$name]}" >/dev/null
    echo "wrote $name"
  else
    echo "would write $name"
  fi
done
[ "$apply" = 1 ] || echo "(dry run: pass --apply to write)"
exit "$status"
