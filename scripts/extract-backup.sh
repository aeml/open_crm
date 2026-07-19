#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/backup-common.sh
source "$SCRIPT_DIR/lib/backup-common.sh"

backup_require_command docker
backup_require_command install
backup_require_command sha256sum
backup_load_environment "${1:-$PWD}"
backup_acquire_lock restore-extract

destination="${2:-}"
[[ -n "$destination" ]] || backup_error "destination dump path is required"
destination="$(backup_absolute_file "$destination")"
metadata_destination="$destination.metadata.txt"
[[ ! -e "$destination" ]] || backup_error "refusing to overwrite: $destination"
[[ ! -e "$metadata_destination" ]] || backup_error "refusing to overwrite: $metadata_destination"

temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/open-crm-extract.XXXXXX")"
chmod 700 "$temporary_dir"
cleanup() {
  local exit_code="$?"
  rm -rf -- "$temporary_dir"
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
actual_sha256="$(sha256sum "$dump_file" | awk '{print $1}')"
[[ -n "$expected_sha256" && "$actual_sha256" == "$expected_sha256" ]] || backup_error "restored dump checksum does not match backup metadata"

install --mode=600 "$dump_file" "$destination"
install --mode=600 "$metadata_file" "$metadata_destination"
echo "backup_extracted snapshot=$snapshot destination=$destination dump_sha256=$actual_sha256"
