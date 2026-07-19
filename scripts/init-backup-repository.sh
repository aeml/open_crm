#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/backup-common.sh
source "$SCRIPT_DIR/lib/backup-common.sh"

backup_require_command docker
backup_load_environment "${1:-$PWD}"

if restic_run snapshots >/dev/null 2>&1; then
  echo "backup_repository_ready repository=$RESTIC_REPOSITORY"
  exit 0
fi

restic_run init
restic_run check
echo "backup_repository_initialized repository=$RESTIC_REPOSITORY"
