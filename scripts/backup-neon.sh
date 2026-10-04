#!/usr/bin/env bash
# Phase 16: logical backup of the Neon database with pg_dump (custom format).
#
# Usage: scripts/backup-neon.sh [--dry-run] [--out DIR]
#   --dry-run  print the plan and check prerequisites only; touches nothing
#   --out DIR  where to write the dump (default: backups/neon, git-ignored)
#
# Reads LEETFORCE_MIGRATE_DATABASE_URL (direct, non-pooler endpoint; preferred for
# pg_dump) or DATABASE_URL from the environment or ./.env. The URL and password are never
# printed. Read-only against the database. Prints the dump path on the last line.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DRY=0; OUT="backups/neon"
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --out) OUT="${2:?--out needs a directory}"; shift ;;
    -h|--help) sed -n '2,11p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

if [ -f .env ]; then set -a; . ./.env; set +a; fi
URL="${LEETFORCE_MIGRATE_DATABASE_URL:-${DATABASE_URL:-}}"
fail() { echo "backup-neon: $*" >&2; exit 1; }
[ -n "$URL" ] || fail "set LEETFORCE_MIGRATE_DATABASE_URL or DATABASE_URL (or put it in .env)"
for t in pg_dump psql sha256sum; do command -v "$t" >/dev/null || fail "$t not found (install postgresql-client matching the Neon major version)"; done

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DUMP="$OUT/leetforce-neon-$STAMP.dump"
echo "plan: pg_dump --format=custom --no-owner --no-privileges -> $DUMP (+ .counts, .sha256)"
echo "      source = database named by the URL in env (not printed); read-only"
if [ "$DRY" = 1 ]; then echo "dry-run: prerequisites ok, nothing written"; exit 0; fi

mkdir -p "$OUT"; umask 077
pg_dump --format=custom --no-owner --no-privileges --file="$DUMP" "$URL"
# A containerised pg_dump ignores our umask, so force the mode: the dump holds emails, hashes and source.
chmod 600 "$DUMP"
# Row counts and goose version at dump time, for the restore drill to compare against.
psql "$URL" -XAtq -v ON_ERROR_STOP=1 -c "select 'goose_max_version=' || coalesce(max(version_id),0) from goose_db_version where is_applied" > "$DUMP.counts"
psql "$URL" -XAtq -v ON_ERROR_STOP=1 -c "select table_name from information_schema.tables where table_schema='public' and table_type='BASE TABLE' and table_name <> 'goose_db_version' order by 1" |
while read -r t; do
  n="$(psql "$URL" -XAtq -v ON_ERROR_STOP=1 -c "select count(*) from public.\"$t\"")"
  echo "$t=$n"
done >> "$DUMP.counts"
sha256sum "$DUMP" > "$DUMP.sha256"
echo "backup ok: $(du -h "$DUMP" | cut -f1), tables recorded: $(($(wc -l < "$DUMP.counts") - 1))"
echo "$DUMP"
