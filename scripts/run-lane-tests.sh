#!/usr/bin/env bash
# run-lane-tests.sh: verifier of one backend lane (plans/261015-student-code-editor, Phase 0).
#
# Usage: bash scripts/run-lane-tests.sh [test files...]
#   Test files may be repo-relative (internal/service/x_test.go) or workspace-relative
#   (backend/internal/service/x_test.go); their packages run first for fast feedback, then the full suite.
#
# Steps (stops at the first failure and prints which step failed):
#   1. test DB: $DB_NAME, or study_test_<branch slug>; created if absent
#   2. migrate + seed base roles/permissions into it (go run ./cmd/seed -mode=base)
#   3. assert system_roles holds TEACHER and TEACHER_APPLICANT (inserts a missing one)
#   4. CI=true go vet ./...
#   5. CI=true go test -p 1 on the packages of the given test files
#   6. CI=true go test -p 1 ./...
# CI=true makes pgtest FAIL instead of SKIP when Postgres is unreachable (internal/testutil/pgtest/pgtest.go).
#
# Postgres access: `psql` on PATH (DB_HOST/DB_PORT/DB_USER/DB_PASSWORD from the environment or .env), otherwise
# `docker exec` into the container named by PG_CONTAINER (default study_postgres). Neither -> exit 2.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 2

STEP="setup"
fail() {
  echo "[run-lane-tests] FAILED at step: $STEP" >&2
  exit 1
}
log() { echo "[run-lane-tests] $*"; }

# Read one KEY from .env without sourcing it (values may hold characters the shell would interpret). Strips CR:
# a CRLF .env otherwise yields "study_user\r" and psql reports a role that "does not exist".
env_file_value() {
  [ -f .env ] || return 0
  grep -E "^$1=" .env | tail -n 1 | cut -d= -f2- | tr -d '\r' | sed -e "s/^[\"']//" -e "s/[\"']\$//"
}
for key in DB_HOST DB_PORT DB_USER DB_PASSWORD; do
  if [ -z "${!key:-}" ]; then
    export "$key=$(env_file_value "$key")"
  fi
done
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-study_user}"

if [ -z "${DB_NAME:-}" ]; then
  slug="$(git rev-parse --abbrev-ref HEAD 2>/dev/null | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9\n' '_' | tr -d '\n')"
  [ -n "$slug" ] || { echo "[run-lane-tests] cannot derive a DB name: set DB_NAME" >&2; exit 2; }
  DB_NAME="study_test_${slug}"
  DB_NAME="${DB_NAME:0:63}"
fi
export DB_NAME
case "$DB_NAME" in
  study_test_*) ;;
  *) echo "[run-lane-tests] refusing DB_NAME=$DB_NAME: lane tests only run against study_test_* databases" >&2; exit 2 ;;
esac

PG_CONTAINER="${PG_CONTAINER:-study_postgres}"
if command -v psql >/dev/null 2>&1; then
  log "postgres via psql on $DB_HOST:$DB_PORT"
  run_sql() { PGPASSWORD="${DB_PASSWORD:-}" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -v ON_ERROR_STOP=1 -tA -d "$1" -c "$2"; }
elif command -v docker >/dev/null 2>&1 && docker inspect -f '{{.State.Running}}' "$PG_CONTAINER" 2>/dev/null | grep -q true; then
  log "postgres via docker exec $PG_CONTAINER (no psql on PATH)"
  run_sql() { docker exec -i "$PG_CONTAINER" psql -U "$DB_USER" -v ON_ERROR_STOP=1 -tA -d "$1" -c "$2"; }
else
  echo "[run-lane-tests] no psql on PATH and container $PG_CONTAINER is not running: cannot prepare $DB_NAME" >&2
  exit 2
fi

STEP="1 create test DB $DB_NAME"
exists="$(run_sql postgres "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'")" || fail
if [ "$exists" != "1" ]; then
  log "creating database $DB_NAME"
  run_sql postgres "CREATE DATABASE \"$DB_NAME\"" >/dev/null || fail
else
  log "database $DB_NAME exists"
fi

STEP="2 migrate + seed base into $DB_NAME"
log "$STEP"
go run ./cmd/seed -mode=base >/dev/null || fail

STEP="3 assert TEACHER and TEACHER_APPLICANT roles"
for role in TEACHER TEACHER_APPLICANT; do
  n="$(run_sql "$DB_NAME" "SELECT count(*) FROM system_roles WHERE name = '$role' AND deleted_at IS NULL")" || fail
  if [ "$n" = "0" ]; then
    log "inserting missing system role $role"
    run_sql "$DB_NAME" "INSERT INTO system_roles (id, name, status, created_at, updated_at) VALUES (gen_random_uuid(), '$role', 'active', now(), now())" >/dev/null || fail
  fi
done

STEP="4 go vet ./..."
log "$STEP"
CI=true go vet ./... || fail

if [ "$#" -gt 0 ]; then
  STEP="5 go test (given files)"
  pkgs=()
  for f in "$@"; do
    f="${f#backend/}"
    f="${f#./}"
    [ -e "$f" ] || { echo "[run-lane-tests] no such test file: $f" >&2; STEP="5 resolve test file $f"; fail; }
    pkg="./$(dirname "$f")"
    case " ${pkgs[*]-} " in *" $pkg "*) ;; *) pkgs+=("$pkg") ;; esac
  done
  log "$STEP: ${pkgs[*]}"
  CI=true go test -p 1 -count=1 "${pkgs[@]}" || fail
fi

STEP="6 go test -p 1 ./..."
log "$STEP (DB_NAME=$DB_NAME)"
CI=true go test -p 1 -count=1 -timeout 25m ./... || fail

log "all steps passed (DB_NAME=$DB_NAME)"
