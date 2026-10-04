#!/usr/bin/env bash
# Phase 16: backup and restore-verify of the S3 test-bundle prefix (ADR 0011, 0021).
#
# Usage: scripts/backup-s3-bundles.sh [--dry-run] [--out DIR] [--no-restore]
#   backup:  aws s3 sync s3://$LEETFORCE_S3_BUCKET/problems/ -> DIR/problems (default backups/s3)
#   verify:  remote object count and bytes compared with the local copy; sha256 of every
#            local file recorded in DIR/problems.sha256
#   restore: sync the local copy to a SCRATCH prefix restore-drill/<ts>/ in the same bucket,
#            compare count and bytes, pull it back and compare checksums, then delete the
#            scratch prefix (always, via trap). tfstate/ is never read or written.
#   --dry-run     print the plan and check prerequisites only; no S3 calls
#   --no-restore  backup and verify only
#
# Env (environment or ./.env, never printed): LEETFORCE_S3_BUCKET (required),
# LEETFORCE_S3_ACCESS_KEY / LEETFORCE_S3_SECRET_KEY (empty on AWS hosts: IAM role),
# LEETFORCE_S3_ENDPOINT (empty for real AWS S3; set only for RustFS/offline).
# The restore step needs PutObject/DeleteObject on restore-drill/*; the runner role has none.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DRY=0; OUT="backups/s3"; RESTORE=1
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --out) OUT="${2:?--out needs a directory}"; shift ;;
    --no-restore) RESTORE=0 ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done
fail() { echo "backup-s3: $*" >&2; exit 1; }

if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${LEETFORCE_S3_BUCKET:?set LEETFORCE_S3_BUCKET (terraform output in infra/bootstrap)}"
command -v aws >/dev/null || fail "aws cli not found"
command -v sha256sum >/dev/null || fail "sha256sum not found"
# Map the repo's env names to the aws cli only when keys are set; otherwise the default chain (IAM role).
if [ -n "${LEETFORCE_S3_ACCESS_KEY:-}" ]; then
  export AWS_ACCESS_KEY_ID="$LEETFORCE_S3_ACCESS_KEY" AWS_SECRET_ACCESS_KEY="${LEETFORCE_S3_SECRET_KEY:-}"
fi
EP=(); if [ -n "${LEETFORCE_S3_ENDPOINT:-}" ]; then EP=(--endpoint-url "$LEETFORCE_S3_ENDPOINT"); fi
B="$LEETFORCE_S3_BUCKET"; STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
SCRATCH="restore-drill/$STAMP"

echo "plan:"
echo "  1. aws s3 sync s3://$B/problems/ -> $OUT/problems (read-only on S3; never touches tfstate/)"
echo "  2. verify: remote object count/bytes == local file count/bytes; record sha256 of each file"
if [ "$RESTORE" = 1 ]; then
  echo "  3. aws s3 sync $OUT/problems -> s3://$B/$SCRATCH/ ; compare count, bytes, checksums ; delete $SCRATCH/"
else echo "  3. (restore step skipped: --no-restore)"; fi
if [ "$DRY" = 1 ]; then echo "dry-run: prerequisites ok, nothing read or written"; exit 0; fi

# count_remote <prefix>: prints "count bytes" from a recursive listing
count_remote() { aws s3 ls "${EP[@]}" "s3://$B/$1" --recursive | awk '{n++; s+=$3} END{print n+0, s+0}'; }
count_local() { find "$1" -type f | wc -l | tr -d ' '; }
size_local() { find "$1" -type f -printf '%s\n' | awk '{s+=$1} END{print s+0}'; }

mkdir -p "$OUT/problems"; umask 077
aws s3 sync "${EP[@]}" "s3://$B/problems/" "$OUT/problems" --only-show-errors
read -r rn rs <<<"$(count_remote problems/)"
ln="$(count_local "$OUT/problems")"; lsz="$(size_local "$OUT/problems")"
[ "$rn" = "$ln" ] && [ "$rs" = "$lsz" ] || fail "VERIFY FAILED: remote $rn objects/$rs bytes, local $ln/$lsz"
(cd "$OUT" && find problems -type f -print0 | sort -z | xargs -0 sha256sum > problems.sha256)
echo "PASS backup: $ln objects, $lsz bytes, checksums in $OUT/problems.sha256"

if [ "$RESTORE" = 1 ]; then
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'; if aws s3 rm ${EP[*]:-} 's3://$B/$SCRATCH/' --recursive --only-show-errors; then echo 'cleanup: removed $SCRATCH/'; else echo 'cleanup FAILED: remove s3://$B/$SCRATCH/ by hand' >&2; fi" EXIT
  aws s3 sync "${EP[@]}" "$OUT/problems" "s3://$B/$SCRATCH/" --only-show-errors
  read -r xn xs <<<"$(count_remote "$SCRATCH/")"
  [ "$xn" = "$ln" ] && [ "$xs" = "$lsz" ] || fail "RESTORE VERIFY FAILED: scratch $xn objects/$xs bytes, expected $ln/$lsz"
  # Byte-level check: pull the scratch copy back and compare checksums with the recorded ones.
  aws s3 sync "${EP[@]}" "s3://$B/$SCRATCH/" "$tmp/problems" --only-show-errors
  (cd "$tmp" && find problems -type f -print0 | sort -z | xargs -0 sha256sum) | diff -q - "$OUT/problems.sha256" >/dev/null \
    || fail "RESTORE VERIFY FAILED: checksums differ"
  echo "PASS restore: $xn objects in the scratch prefix, checksums identical"
fi
echo "DRILL RESULT: PASS"
