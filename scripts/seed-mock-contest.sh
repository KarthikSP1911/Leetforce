#!/usr/bin/env bash
# Phase 14: create a mock contest (slug mock-contest) with three existing
# problems (A fizz-count, B reverse-words, C max-subarray-sum) and three
# throwaway users (mock1, mock2, mock3). Idempotent: an older mock contest, its
# registrations and its contest submissions are removed first.
#
# The contest starts a minute ago and lasts MOCK_MINUTES (default 60), so it is
# running. The problems must already be in the database (the API syncs them from
# problems/ at start). Needs psql, curl, DATABASE_URL, and a running API at
# LEETFORCE_API (default http://127.0.0.1:8080) for the sign-ups.
# Writes to the real database; delete it with: scripts/seed-mock-contest.sh --clean
set -euo pipefail

cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi
: "${DATABASE_URL:?set DATABASE_URL (or put it in .env)}"
API="${LEETFORCE_API:-http://127.0.0.1:8080}"
MINUTES="${MOCK_MINUTES:-60}"
PASSWORD="${MOCK_PASSWORD:-mock-contest-pw-1}"

clean() {
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -qAt <<'SQL'
DELETE FROM submissions WHERE contest_id IN (SELECT id FROM contests WHERE slug = 'mock-contest');
DELETE FROM contests WHERE slug = 'mock-contest';
SQL
}

if [ "${1:-}" = "--clean" ]; then
  clean
  psql "$DATABASE_URL" -qAt -c "DELETE FROM submissions WHERE user_id IN (SELECT id FROM users WHERE username IN ('mock1','mock2','mock3')); DELETE FROM users WHERE username IN ('mock1','mock2','mock3')"
  echo "mock contest and mock users removed"
  exit 0
fi

for u in mock1 mock2 mock3; do
  code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/auth/signup" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$u@example.com\",\"username\":\"$u\",\"password\":\"$PASSWORD\"}")"
  case "$code" in
    201) echo "created user $u" ;;
    409|422) echo "user $u already exists ($code)" ;;
    *) echo "sign-up of $u failed with HTTP $code" >&2; exit 1 ;;
  esac
done

clean
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -v mins="$MINUTES" -qAt <<'SQL'
WITH c AS (
  INSERT INTO contests (id, slug, title, starts_at, ends_at)
  VALUES (gen_random_uuid(), 'mock-contest', 'LeetForce Mock Contest',
          now() - interval '1 minute', now() + (:'mins' || ' minutes')::interval)
  RETURNING id
)
INSERT INTO contest_problems (contest_id, problem_slug, label, position, points)
SELECT c.id, v.slug, v.label, v.pos, 100
  FROM c, (VALUES ('fizz-count', 'A', 1), ('reverse-words', 'B', 2), ('max-subarray-sum', 'C', 3))
       AS v(slug, label, pos);
SQL
echo "mock contest ready: slug mock-contest, 3 problems, ${MINUTES} minutes"
