#!/usr/bin/env bash
# Phase 12 exit check: destroying infra/aws can never reach infra/neon.
# Static, free and offline (no cloud credentials): it inspects the Terraform
# configuration, the providers, the state paths and arena.sh itself.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF="${TERRAFORM:-terraform}"
fail=0
check() { # check <description> <command...>
  local d="$1"
  shift
  if "$@" >/dev/null 2>&1; then echo "PASS: $d"; else
    echo "FAIL: $d"
    fail=1
  fi
}

for s in aws neon; do "$TF" -chdir="$ROOT/infra/$s" init -backend=false -input=false >/dev/null; done

check "infra/aws declares no neon provider" bash -c "! $TF -chdir=$ROOT/infra/aws providers | grep -qi neon"
check "infra/aws has no neon_ resource" bash -c "! grep -rq 'neon_' $ROOT/infra/aws --include=*.tf"
check "infra/aws code never references neon (comments aside)" bash -c "! cat $ROOT/infra/aws/*.tf | grep -v '^ *#' | grep -qi neon"
check "infra/neon declares no aws provider" bash -c "! $TF -chdir=$ROOT/infra/neon providers | grep -qi hashicorp/aws"
state_key() { grep -A4 'backend "s3"' "$1/versions.tf" | sed -n 's/.*key *= *"\([^"]*\)".*/\1/p'; }
aws_key="$(state_key "$ROOT/infra/aws")"
neon_key="$(state_key "$ROOT/infra/neon")"
check "each stack has its own state key in the S3 bucket" bash -c "[ -n '$aws_key' ] && [ -n '$neon_key' ] && [ '$aws_key' != '$neon_key' ]"
check "neon project has prevent_destroy" grep -q 'prevent_destroy = true' "$ROOT/infra/neon/main.tf"
check "arena.sh only runs terraform in infra/aws" bash -c "
  grep -q 'STACK=\"\$ROOT/infra/aws\"' $ROOT/scripts/arena.sh && ! grep -q 'infra/neon' <(grep -v '^#' $ROOT/scripts/arena.sh | grep -v echo)"
check "arena.sh down needs the typed phrase" grep -q 'confirm "destroy leetforce-aws"' "$ROOT/scripts/arena.sh"

# Needs credentials, so it is only a bonus: the destroy plan must name no neon resource.
if [ -n "${AWS_PROFILE:-}${AWS_ACCESS_KEY_ID:-}" ] && [ -f "$ROOT/infra/aws/terraform.tfvars" ]; then
  check "plan -destroy in infra/aws mentions no neon_ resource" bash -c "
    ! $TF -chdir=$ROOT/infra/aws plan -destroy -input=false -no-color | grep -q neon_"
else
  echo "SKIP: plan -destroy (no AWS credentials or terraform.tfvars)"
fi

[ "$fail" -eq 0 ] && echo "ISOLATION_OK" || {
  echo "ISOLATION_FAILED"
  exit 1
}
