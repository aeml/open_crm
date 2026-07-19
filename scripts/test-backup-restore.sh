#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/open-crm-backup-test.XXXXXX")"
project_name="opencrm_backup_test_$$"
cleanup() {
  COMPOSE_PROJECT_NAME="$project_name" docker compose \
    -f "$REPO_ROOT/docker-compose.deploy.yml" \
    --env-file "$test_root/test.env" down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf -- "$test_root"
}
trap cleanup EXIT

mkdir -p "$test_root/restic-repository" "$test_root/status"
printf '%s\n' 'backup-drill-encryption-password' > "$test_root/restic-password"
chmod 600 "$test_root/restic-password"

cat > "$test_root/test.env" <<EOF
POSTGRES_DB=open_crm_backup_test
POSTGRES_USER=open_crm
POSTGRES_PASSWORD=open_crm
ALLOWED_ORIGINS=http://localhost:5173
EMAIL_FROM_NAME=Open CRM
OPEN_CRM_ENV_FILE=$test_root/test.env
RESTIC_REPOSITORY=/repository
RESTIC_LOCAL_REPOSITORY_PATH=$test_root/restic-repository
RESTIC_PASSWORD_FILE=$test_root/restic-password
RESTIC_IMAGE=restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510
BACKUP_ALLOW_LOCAL_REPOSITORY=true
BACKUP_STATUS_DIR=$test_root/status
BACKUP_HOST_TAG=open-crm-backup-test
BACKUP_TAG=open-crm-postgres-test
BACKUP_KEEP_DAILY=2
BACKUP_KEEP_WEEKLY=1
BACKUP_KEEP_MONTHLY=1
EOF
chmod 600 "$test_root/test.env"

export COMPOSE_PROJECT_NAME="$project_name"
export BACKUP_ENV_FILE="$test_root/test.env"
export BACKUP_COMPOSE_FILE="$REPO_ROOT/docker-compose.deploy.yml"

docker compose -f "$BACKUP_COMPOSE_FILE" --env-file "$BACKUP_ENV_FILE" up -d postgres
docker compose -f "$BACKUP_COMPOSE_FILE" --env-file "$BACKUP_ENV_FILE" build migrate
docker compose -f "$BACKUP_COMPOSE_FILE" --env-file "$BACKUP_ENV_FILE" run --rm migrate
docker compose -f "$BACKUP_COMPOSE_FILE" --env-file "$BACKUP_ENV_FILE" exec -T postgres \
  psql -U open_crm -d open_crm_backup_test -v ON_ERROR_STOP=1 \
  -c "INSERT INTO organizations (name, slug, business_type, subscription_status) VALUES ('Backup Drill Organization', 'backup-drill-organization', 'general', 'trialing')"

"$REPO_ROOT/scripts/init-backup-repository.sh" "$REPO_ROOT"
"$REPO_ROOT/scripts/backup-postgres.sh" "$REPO_ROOT"

# A failed dump must preserve the last verified success while recording the
# failed attempt for alerting. Then restore the healthy latest-attempt state.
sed 's/^POSTGRES_DB=.*/POSTGRES_DB=missing_backup_database/' \
  "$test_root/test.env" > "$test_root/failing.env"
chmod 600 "$test_root/failing.env"
if BACKUP_ENV_FILE="$test_root/failing.env" "$REPO_ROOT/scripts/backup-postgres.sh" "$REPO_ROOT"; then
  echo "backup unexpectedly succeeded for a missing database" >&2
  exit 1
fi
grep -q '"status":"succeeded"' "$test_root/status/last-backup.json"
grep -q '"status":"failed"' "$test_root/status/last-backup-attempt.json"
"$REPO_ROOT/scripts/backup-postgres.sh" "$REPO_ROOT"

"$REPO_ROOT/scripts/extract-backup.sh" "$REPO_ROOT" "$test_root/extracted.dump"
docker compose -f "$BACKUP_COMPOSE_FILE" --env-file "$BACKUP_ENV_FILE" exec -T postgres \
  pg_restore --list < "$test_root/extracted.dump" >/dev/null
"$REPO_ROOT/scripts/restore-drill.sh" "$REPO_ROOT"

grep -q '"status":"succeeded"' "$test_root/status/last-backup.json"
grep -q '"status":"succeeded"' "$test_root/status/last-backup-attempt.json"
grep -q '"status":"succeeded"' "$test_root/status/last-restore-drill.json"
grep -q '"status":"succeeded"' "$test_root/status/last-restore-drill-attempt.json"
grep -q '"organizations":1' "$test_root/status/last-restore-drill.json"
if grep -R -a -q 'Backup Drill Organization' "$test_root/restic-repository"; then
  echo "backup repository exposed plaintext database content" >&2
  exit 1
fi

# Production configuration must never silently accept a local repository.
grep -v '^BACKUP_ALLOW_LOCAL_REPOSITORY=' "$test_root/test.env" > "$test_root/local-rejected.env"
chmod 600 "$test_root/local-rejected.env"
if BACKUP_ENV_FILE="$test_root/local-rejected.env" "$REPO_ROOT/scripts/init-backup-repository.sh" "$REPO_ROOT"; then
  echo "local backup repository was not rejected" >&2
  exit 1
fi

echo "backup_restore_acceptance_succeeded"
