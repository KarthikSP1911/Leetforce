#!/usr/bin/env bash
# arena.sh up|down|status: bring the LeetForce cloud hosts (infra/aws) up or down.
#
# Only infra/aws is ever touched. infra/neon holds the database and is never
# applied or destroyed from here. Both `up` and `down` bill or delete real
# resources, so each asks you to type a phrase; CLAUDE.md also requires explicit
# confirmation in chat before Claude runs `up` or `down` at all.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STACK="$ROOT/infra/aws"
TF="${TERRAFORM:-terraform}"

usage() {
  echo "usage: scripts/arena.sh up|down|status" >&2
  exit 2
}

tf() { "$TF" -chdir="$STACK" "$@"; }

confirm() { # confirm <phrase> <prompt>
  local phrase="$1" answer
  [ -t 0 ] || {
    echo "arena.sh: confirmation needs an interactive terminal" >&2
    exit 1
  }
  read -r -p "$2 Type '$phrase' to continue: " answer
  [ "$answer" = "$phrase" ] || {
    echo "arena.sh: cancelled" >&2
    exit 1
  }
}

[ $# -eq 1 ] || usage
[ -d "$STACK" ] || {
  echo "arena.sh: $STACK not found" >&2
  exit 1
}
tf init -input=false >/dev/null

case "$1" in
status)
  tf state list 2>/dev/null | sed 's/^/  /' || true
  tf output 2>/dev/null || echo "  (nothing applied)"
  ;;
up)
  tf plan -input=false -out="$STACK/arena.tfplan"
  echo "The plan above creates billable EC2, EBS and public IPv4 (see the README cost table)."
  confirm "up leetforce-aws" "Apply it?"
  tf apply -input=false "$STACK/arena.tfplan"
  rm -f "$STACK/arena.tfplan"
  ;;
down)
  tf plan -destroy -input=false
  echo "This destroys everything in infra/aws. infra/neon (the database) is a separate stack and is not touched."
  confirm "destroy leetforce-aws" "Destroy it?"
  tf destroy -input=false -auto-approve
  ;;
*) usage ;;
esac
