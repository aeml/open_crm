#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/backup-common.sh
source "$SCRIPT_DIR/lib/backup-common.sh"

backup_require_command docker
backup_require_command sha256sum
backup_load_environment "${1:-$PWD}"
backup_acquire_lock restore-drill

started_epoch="$(date +%s)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/open-crm-restore.XXXXXX")"
chmod 700 "$temporary_dir"
drill_container="open-crm-restore-drill-$(date -u +%Y%m%d%H%M%S)-$$"
cleanup() {
  local exit_code="$?"
  if [[ "$exit_code" -ne 0 ]]; then
    docker logs "$drill_container" >&2 2>/dev/null || true
  fi
  docker rm -f "$drill_container" >/dev/null 2>&1 || true
  rm -rf -- "$temporary_dir"
  if [[ "$exit_code" -ne 0 ]]; then
    local completed_at duration_seconds failure_payload
    completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    duration_seconds="$(( $(date +%s) - started_epoch ))"
    failure_payload="$(printf '{"status":"failed","startedAt":"%s","completedAt":"%s","durationSeconds":%d,"exitCode":%d}' \
      "$started_at" "$completed_at" "$duration_seconds" "$exit_code")"
    backup_write_status "$BACKUP_STATUS_DIR/last-restore-drill-attempt.json" "$failure_payload" || true
    echo "restore_drill_failed exit_code=$exit_code duration_seconds=$duration_seconds completed_at=$completed_at" >&2
  fi
  return "$exit_code"
}
trap cleanup EXIT

RESTIC_MOUNTS=(-v "$temporary_dir:/restore")
snapshot="${RESTORE_SNAPSHOT:-latest}"
[[ "$snapshot" =~ ^(latest|[a-f0-9]{8,64})$ ]] || backup_error "RESTORE_SNAPSHOT must be latest or a hexadecimal snapshot ID"
restic_run restore "$snapshot" \
  --host "$BACKUP_HOST_TAG" --tag "$BACKUP_TAG" --target /restore

dump_file="$(find "$temporary_dir" -type f -name open_crm.dump -print -quit)"
metadata_file="$(find "$temporary_dir" -type f -name metadata.txt -print -quit)"
[[ -n "$dump_file" && -f "$dump_file" ]] || backup_error "restored snapshot did not contain open_crm.dump"
[[ -n "$metadata_file" && -f "$metadata_file" ]] || backup_error "restored snapshot did not contain metadata.txt"

expected_sha256="$(sed -n 's/^dump_sha256=//p' "$metadata_file" | head -n 1)"
source_revision="$(sed -n 's/^source_revision=//p' "$metadata_file" | head -n 1)"
actual_sha256="$(sha256sum "$dump_file" | awk '{print $1}')"
[[ -n "$expected_sha256" && "$actual_sha256" == "$expected_sha256" ]] || backup_error "restored dump checksum does not match backup metadata"

postgres_container_id="$("${COMPOSE[@]}" ps -q postgres)"
[[ -n "$postgres_container_id" ]] || backup_error "production Postgres container is not running"
compose_network="$(docker inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{"\n"}}{{end}}' "$postgres_container_id" | head -n 1)"
[[ -n "$compose_network" ]] || backup_error "unable to determine the Compose network"

docker run -d --name "$drill_container" --network "$compose_network" \
  -e POSTGRES_DB=open_crm_restore_drill \
  -e POSTGRES_USER=open_crm_restore \
  -e POSTGRES_PASSWORD=restore-drill-only \
  postgres:16.14@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20 >/dev/null

for attempt in $(seq 1 30); do
  # The official image briefly accepts connections on a temporary server and
  # then restarts it after first-run initialization. Wait for that boundary so
  # a restore cannot race the expected shutdown.
  if docker logs "$drill_container" 2>&1 | grep -q 'PostgreSQL init process complete' && \
    docker exec "$drill_container" pg_isready -U open_crm_restore -d open_crm_restore_drill >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 30 ]]; then
    docker logs "$drill_container" >&2
    backup_error "restore-drill PostgreSQL did not become ready"
  fi
  sleep 1
done

docker cp "$dump_file" "$drill_container:/tmp/open_crm.dump"
docker exec "$drill_container" pg_restore \
  -U open_crm_restore -d open_crm_restore_drill \
  --no-owner --no-acl /tmp/open_crm.dump

if [[ "${RESTORE_DRILL_SKIP_MIGRATIONS:-false}" != "true" ]]; then
  "${COMPOSE[@]}" run --rm \
    -e DATABASE_URL="postgres://open_crm_restore:restore-drill-only@$drill_container:5432/open_crm_restore_drill?sslmode=disable" \
    migrate
fi

schema_migration_count="$(docker exec "$drill_container" psql -U open_crm_restore -d open_crm_restore_drill -Atc 'SELECT COUNT(*) FROM schema_migrations')"
organization_count="$(docker exec "$drill_container" psql -U open_crm_restore -d open_crm_restore_drill -Atc 'SELECT COUNT(*) FROM organizations')"
background_jobs_table="$(docker exec "$drill_container" psql -U open_crm_restore -d open_crm_restore_drill -Atc "SELECT COALESCE(to_regclass('public.background_jobs')::text, '')")"
[[ "$schema_migration_count" =~ ^[1-9][0-9]*$ ]] || backup_error "restored schema migration ledger is empty"
[[ "$organization_count" =~ ^[0-9]+$ ]] || backup_error "restored organizations table is unreadable"
[[ "$background_jobs_table" == "background_jobs" ]] || backup_error "restored schema is missing background_jobs"

completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
duration_seconds="$(( $(date +%s) - started_epoch ))"
status_payload="$(printf '{"status":"succeeded","startedAt":"%s","completedAt":"%s","durationSeconds":%d,"snapshot":"%s","dumpSha256":"%s","sourceRevision":"%s","schemaMigrations":%d,"organizations":%d}' \
  "$started_at" "$completed_at" "$duration_seconds" "$snapshot" "$actual_sha256" "$source_revision" "$schema_migration_count" "$organization_count")"
backup_write_status "$BACKUP_STATUS_DIR/last-restore-drill.json" "$status_payload"
backup_write_status "$BACKUP_STATUS_DIR/last-restore-drill-attempt.json" "$status_payload"

echo "restore_drill_succeeded snapshot=$snapshot duration_seconds=$duration_seconds schema_migrations=$schema_migration_count organizations=$organization_count completed_at=$completed_at"
