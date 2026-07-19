#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/backup-common.sh
source "$SCRIPT_DIR/lib/backup-common.sh"

backup_require_command docker
backup_require_command sha256sum
backup_load_environment "${1:-$PWD}"
backup_acquire_lock backup

started_epoch="$(date +%s)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/open-crm-backup.XXXXXX")"
chmod 700 "$temporary_dir"
cleanup() {
  local exit_code="$?"
  rm -rf -- "$temporary_dir"
  if [[ "$exit_code" -ne 0 ]]; then
    local completed_at duration_seconds failure_payload
    completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    duration_seconds="$(( $(date +%s) - started_epoch ))"
    failure_payload="$(printf '{"status":"failed","startedAt":"%s","completedAt":"%s","durationSeconds":%d,"exitCode":%d}' \
      "$started_at" "$completed_at" "$duration_seconds" "$exit_code")"
    backup_write_status "$BACKUP_STATUS_DIR/last-backup-attempt.json" "$failure_payload" || true
    echo "backup_failed exit_code=$exit_code duration_seconds=$duration_seconds completed_at=$completed_at" >&2
  fi
  return "$exit_code"
}
trap cleanup EXIT

dump_file="$temporary_dir/open_crm.dump"
metadata_file="$temporary_dir/metadata.txt"

"${COMPOSE[@]}" exec -T postgres \
  pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  --format=custom --compress=9 --no-owner --no-acl > "$dump_file"
chmod 600 "$dump_file"

"${COMPOSE[@]}" exec -T postgres pg_restore --list < "$dump_file" >/dev/null
dump_sha256="$(sha256sum "$dump_file" | awk '{print $1}')"
source_revision="$(git -C "$BACKUP_DEPLOY_PATH" rev-parse HEAD 2>/dev/null || printf 'unknown')"
cat > "$metadata_file" <<EOF
created_at=$started_at
dump_sha256=$dump_sha256
source_revision=$source_revision
postgres_database=$POSTGRES_DB
EOF
chmod 600 "$metadata_file"

RESTIC_MOUNTS=(-v "$temporary_dir:/data:ro")
backup_output="$(restic_run backup /data --host "$BACKUP_HOST_TAG" --tag "$BACKUP_TAG" --json)"
snapshot_id="$(printf '%s\n' "$backup_output" | sed -n 's/.*"snapshot_id":"\([^"]*\)".*/\1/p' | tail -n 1)"
[[ -n "$snapshot_id" ]] || backup_error "Restic did not report a snapshot ID"

restic_run forget \
  --host "$BACKUP_HOST_TAG" --tag "$BACKUP_TAG" \
  --keep-daily "${BACKUP_KEEP_DAILY:-7}" \
  --keep-weekly "${BACKUP_KEEP_WEEKLY:-5}" \
  --keep-monthly "${BACKUP_KEEP_MONTHLY:-12}" \
  --prune
restic_run check

completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
duration_seconds="$(( $(date +%s) - started_epoch ))"
status_payload="$(printf '{"status":"succeeded","startedAt":"%s","completedAt":"%s","durationSeconds":%d,"snapshotId":"%s","dumpSha256":"%s","sourceRevision":"%s"}' \
  "$started_at" "$completed_at" "$duration_seconds" "$snapshot_id" "$dump_sha256" "$source_revision")"
backup_write_status "$BACKUP_STATUS_DIR/last-backup.json" "$status_payload"
backup_write_status "$BACKUP_STATUS_DIR/last-backup-attempt.json" "$status_payload"

echo "backup_succeeded snapshot_id=$snapshot_id duration_seconds=$duration_seconds completed_at=$completed_at"
