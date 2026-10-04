#!/usr/bin/env bash
# Phase 16: restore drill. Restores a pg_dump into a SCRATCH target, verifies it against
# the source counts, then removes the scratch target. Never writes to the source database.
#
# Usage: scripts/restore-drill-neon.sh [--dry-run] [--mode branch|scratchdb] [--dump FILE]
#   --mode branch     create a temporary Neon branch (Neon API; may consume compute hours
#                     while it exists), reset its public schema, restore into it, delete it.
#   --mode scratchdb  restore into a temporary database on the server named by
#                     LEETFORCE_SCRATCH_ADMIN_URL (e.g. a local Postgres), then drop it.
#   --dump FILE       use an existing dump (default: take a fresh one with backup-neon.sh)
#   --dry-run         print the plan and check prerequisites only; creates nothing
#
# Env (from the environment or ./.env, never printed): DATABASE_URL or
# LEETFORCE_MIGRATE_DATABASE_URL (source); branch mode also NEON_API_KEY and
# NEON_PROJECT_ID (optional NEON_PARENT_BRANCH_ID, NEON_DATABASE, NEON_ROLE);
# scratchdb mode also LEETFORCE_SCRATCH_ADMIN_URL (role needs CREATEDB).
# Verifies: per-table row counts equal the source counts recorded at dump time, goose max
# applied version equal, and a sample query on the scratch copy succeeds.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DRY=0; MODE="scratchdb"; DUMP=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --mode) MODE="${2:?--mode needs branch|scratchdb}"; shift ;;
    --dump) DUMP="${2:?--dump needs a file}"; shift ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done
fail() { echo "restore-drill: $*" >&2; exit 1; }
case "$MODE" in branch|scratchdb) ;; *) fail "--mode must be branch or scratchdb" ;; esac

if [ -f .env ]; then set -a; . ./.env; set +a; fi
SRC="${LEETFORCE_MIGRATE_DATABASE_URL:-${DATABASE_URL:-}}"
[ -n "$SRC" ] || fail "set DATABASE_URL or LEETFORCE_MIGRATE_DATABASE_URL"
for t in pg_dump pg_restore psql; do command -v "$t" >/dev/null || fail "$t not found"; done
if [ "$MODE" = branch ]; then
  for t in curl jq; do command -v "$t" >/dev/null || fail "$t not found"; done
  : "${NEON_API_KEY:?set NEON_API_KEY (Neon console > Account > API keys)}"
  : "${NEON_PROJECT_ID:?set NEON_PROJECT_ID (the id infra/neon imports)}"
else
  : "${LEETFORCE_SCRATCH_ADMIN_URL:?set LEETFORCE_SCRATCH_ADMIN_URL (admin URL of a scratch Postgres, e.g. local)}"
fi

echo "plan ($MODE mode):"
echo "  1. dump: ${DUMP:-fresh pg_dump via scripts/backup-neon.sh (read-only on source)}"
if [ "$MODE" = branch ]; then
  echo "  2. create Neon branch drill-<ts> via the Neon API (read_write endpoint)"
  echo "  3. drop and recreate schema public on the branch; pg_restore the dump into it"
else
  echo "  2. create database leetforce_drill_<ts> on LEETFORCE_SCRATCH_ADMIN_URL"
  echo "  3. pg_restore the dump into it"
fi
echo "  4. verify: row counts per table, goose version, sample query"
echo "  5. cleanup (always, via trap): delete the branch / drop the database"
if [ "$DRY" = 1 ]; then echo "dry-run: prerequisites ok, nothing created"; exit 0; fi

if [ -z "$DUMP" ]; then DUMP="$("$ROOT/scripts/backup-neon.sh" | tail -n1)"; fi
[ -f "$DUMP" ] && [ -f "$DUMP.counts" ] || fail "dump or $DUMP.counts missing"
STAMP="$(date -u +%Y%m%d%H%M%S)"
TARGET=""; BRANCH_ID=""; DBNAME=""
API="https://console.neon.tech/api/v2/projects/${NEON_PROJECT_ID:-}"

cleanup() {
  set +e
  if [ -n "$BRANCH_ID" ]; then
    if curl -fsS -X DELETE -H "Authorization: Bearer $NEON_API_KEY" "$API/branches/$BRANCH_ID" >/dev/null; then
      echo "cleanup: deleted Neon branch $BRANCH_ID"
    else echo "cleanup FAILED: delete branch $BRANCH_ID by hand" >&2; fi
  fi
  if [ -n "$DBNAME" ]; then
    if psql "$LEETFORCE_SCRATCH_ADMIN_URL" -XAtq -c "drop database if exists \"$DBNAME\" with (force)" >/dev/null; then
      echo "cleanup: dropped database $DBNAME"
    else echo "cleanup FAILED: drop database $DBNAME by hand" >&2; fi
  fi
}
trap cleanup EXIT

if [ "$MODE" = branch ]; then
  body="$(jq -n --arg n "drill-$STAMP" --arg p "${NEON_PARENT_BRANCH_ID:-}" \
    '{branch: ({name:$n} + (if $p=="" then {} else {parent_id:$p} end)), endpoints:[{type:"read_write"}]}')"
  resp="$(curl -fsS -X POST -H "Authorization: Bearer $NEON_API_KEY" -H 'Content-Type: application/json' -d "$body" "$API/branches")"
  BRANCH_ID="$(jq -r '.branch.id' <<<"$resp")"
  [ -n "$BRANCH_ID" ] && [ "$BRANCH_ID" != null ] || fail "branch create returned no id"
  uri="$(curl -fsS -H "Authorization: Bearer $NEON_API_KEY" \
    "$API/connection_uri?branch_id=$BRANCH_ID&database_name=${NEON_DATABASE:-neondb}&role_name=${NEON_ROLE:-neondb_owner}" | jq -r .uri)"
  [ -n "$uri" ] && [ "$uri" != null ] || fail "no connection uri for the scratch branch"
  TARGET="$uri"
  # A branch starts as a copy of its parent; wipe it so the restore is what we verify.
  psql "$TARGET" -XAtq -v ON_ERROR_STOP=1 -c "drop schema public cascade; create schema public;" >/dev/null
else
  DBNAME="leetforce_drill_$STAMP"
  psql "$LEETFORCE_SCRATCH_ADMIN_URL" -XAtq -v ON_ERROR_STOP=1 -c "create database \"$DBNAME\"" >/dev/null
  # Swap the database name in the admin URL path (keeps credentials and query string, never printed).
  TARGET="$(printf '%s' "$LEETFORCE_SCRATCH_ADMIN_URL" | sed -E "s#^([a-z]+://[^/]+)/[^?]*#\1/$DBNAME#")"
fi

pg_restore --no-owner --no-privileges --exit-on-error --dbname="$TARGET" "$DUMP"
echo "restored $(du -h "$DUMP" | cut -f1) dump into the scratch target"

bad=0
want_goose="$(grep '^goose_max_version=' "$DUMP.counts" | cut -d= -f2)"
got_goose="$(psql "$TARGET" -XAtq -c "select coalesce(max(version_id),0) from goose_db_version where is_applied")"
if [ "$want_goose" = "$got_goose" ]; then echo "PASS goose version $got_goose"; else echo "FAIL goose version want=$want_goose got=$got_goose"; bad=1; fi
while IFS='=' read -r t want; do
  [ "$t" = goose_max_version ] && continue
  got="$(psql "$TARGET" -XAtq -c "select count(*) from public.\"$t\"")"
  if [ "$want" = "$got" ]; then echo "PASS rows $t=$got"; else echo "FAIL rows $t want=$want got=$got"; bad=1; fi
done < "$DUMP.counts"
# Sample query: the restored schema is usable, not just present.
if sample="$(psql "$TARGET" -XAtq -c "select count(*) from public.problems" 2>/dev/null)"; then
  echo "PASS sample query: select count(*) from problems = $sample"
else
  echo "FAIL sample query on public.problems"; bad=1
fi
if [ "$bad" = 0 ]; then echo "DRILL RESULT: PASS"; else echo "DRILL RESULT: FAIL" >&2; exit 1; fi
