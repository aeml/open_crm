#!/usr/bin/env bash

# Shared helpers for encrypted PostgreSQL backup and restore-drill scripts.
# The caller must enable `set -euo pipefail` before sourcing this file.

BACKUP_COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/env-file.sh
source "$BACKUP_COMMON_DIR/env-file.sh"

RESTIC_IMAGE_DEFAULT="restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510"
BACKUP_TAG_DEFAULT="open-crm-postgres"

backup_error() {
  echo "backup_error: $*" >&2
  exit 1
}

backup_require_command() {
  command -v "$1" >/dev/null 2>&1 || backup_error "$1 is required"
}

backup_absolute_file() {
  local path="$1"
  local directory
  directory="$(cd "$(dirname "$path")" && pwd -P)"
  printf '%s/%s\n' "$directory" "$(basename "$path")"
}

backup_load_environment() {
  local requested_path="${1:-}"
  if [[ -z "$requested_path" ]]; then
    backup_error "deployment path is required"
  fi

  BACKUP_DEPLOY_PATH="$(cd "$requested_path" && pwd -P)"
  BACKUP_ENV_FILE="${BACKUP_ENV_FILE:-$BACKUP_DEPLOY_PATH/.env.production}"
  BACKUP_COMPOSE_FILE="${BACKUP_COMPOSE_FILE:-$BACKUP_DEPLOY_PATH/docker-compose.deploy.yml}"

  [[ -f "$BACKUP_ENV_FILE" ]] || backup_error "missing environment file: $BACKUP_ENV_FILE"
  [[ -f "$BACKUP_COMPOSE_FILE" ]] || backup_error "missing compose file: $BACKUP_COMPOSE_FILE"

  open_crm_load_env_keys "$BACKUP_ENV_FILE" \
    POSTGRES_USER \
    POSTGRES_DB \
    RESTIC_IMAGE \
    RESTIC_REPOSITORY \
    RESTIC_PASSWORD_FILE \
    RESTIC_BACKEND_ENV_FILE \
    RESTIC_LOCAL_REPOSITORY_PATH \
    BACKUP_ALLOW_LOCAL_REPOSITORY \
    BACKUP_TAG \
    BACKUP_HOST_TAG \
    BACKUP_STATUS_DIR \
    BACKUP_KEEP_DAILY \
    BACKUP_KEEP_WEEKLY \
    BACKUP_KEEP_MONTHLY \
    RESTORE_SNAPSHOT \
    RESTORE_DRILL_SKIP_MIGRATIONS

  : "${POSTGRES_USER:=open_crm}"
  : "${POSTGRES_DB:=open_crm}"
  : "${RESTIC_IMAGE:=$RESTIC_IMAGE_DEFAULT}"
  : "${BACKUP_TAG:=$BACKUP_TAG_DEFAULT}"
  : "${BACKUP_HOST_TAG:=open-crm}"
  : "${BACKUP_STATUS_DIR:=$BACKUP_DEPLOY_PATH/var/backup-status}"

  [[ -n "${RESTIC_REPOSITORY:-}" ]] || backup_error "RESTIC_REPOSITORY is required"
  [[ -n "${RESTIC_PASSWORD_FILE:-}" ]] || backup_error "RESTIC_PASSWORD_FILE is required"
  [[ -f "$RESTIC_PASSWORD_FILE" ]] || backup_error "RESTIC_PASSWORD_FILE does not exist"

  RESTIC_PASSWORD_FILE="$(backup_absolute_file "$RESTIC_PASSWORD_FILE")"
  local password_mode
  password_mode="$(stat -c '%a' "$RESTIC_PASSWORD_FILE")"
  if (( (8#$password_mode & 077) != 0 )); then
    backup_error "RESTIC_PASSWORD_FILE must not be group/world accessible (current mode $password_mode)"
  fi

  RESTIC_DOCKER_REPOSITORY="$RESTIC_REPOSITORY"
  RESTIC_LOCAL_MOUNT=()
  case "$RESTIC_REPOSITORY" in
    /*|local:*)
      [[ "${BACKUP_ALLOW_LOCAL_REPOSITORY:-false}" == "true" ]] || backup_error "local Restic repositories are test-only; configure an off-host repository"
      [[ -n "${RESTIC_LOCAL_REPOSITORY_PATH:-}" ]] || backup_error "RESTIC_LOCAL_REPOSITORY_PATH is required for test-only local repositories"
      mkdir -p "$RESTIC_LOCAL_REPOSITORY_PATH"
      RESTIC_LOCAL_REPOSITORY_PATH="$(cd "$RESTIC_LOCAL_REPOSITORY_PATH" && pwd -P)"
      RESTIC_DOCKER_REPOSITORY="/repository"
      RESTIC_LOCAL_MOUNT=(-v "$RESTIC_LOCAL_REPOSITORY_PATH:/repository")
      ;;
  esac

  RESTIC_BACKEND_ENV_ARGS=()
  if [[ -n "${RESTIC_BACKEND_ENV_FILE:-}" ]]; then
    [[ -f "$RESTIC_BACKEND_ENV_FILE" ]] || backup_error "RESTIC_BACKEND_ENV_FILE does not exist"
    RESTIC_BACKEND_ENV_FILE="$(backup_absolute_file "$RESTIC_BACKEND_ENV_FILE")"
    local backend_mode
    backend_mode="$(stat -c '%a' "$RESTIC_BACKEND_ENV_FILE")"
    if (( (8#$backend_mode & 077) != 0 )); then
      backup_error "RESTIC_BACKEND_ENV_FILE must not be group/world accessible (current mode $backend_mode)"
    fi
    RESTIC_BACKEND_ENV_ARGS=(--env-file "$RESTIC_BACKEND_ENV_FILE")
  fi

  mkdir -p "$BACKUP_STATUS_DIR"
  # Status documents contain only operational timestamps/results (no dump or
  # credentials) and are mounted read-only into the unprivileged API process.
  chmod 755 "$BACKUP_STATUS_DIR"
  COMPOSE=(docker compose -f "$BACKUP_COMPOSE_FILE" --env-file "$BACKUP_ENV_FILE")
  RESTIC_MOUNTS=()
}

restic_run() {
  docker run --rm \
    --user "$(id -u):$(id -g)" \
    -e HOME=/tmp \
    -e RESTIC_REPOSITORY="$RESTIC_DOCKER_REPOSITORY" \
    -e RESTIC_PASSWORD_FILE=/run/secrets/restic-password \
    -v "$RESTIC_PASSWORD_FILE:/run/secrets/restic-password:ro" \
    "${RESTIC_BACKEND_ENV_ARGS[@]}" \
    "${RESTIC_LOCAL_MOUNT[@]}" \
    "${RESTIC_MOUNTS[@]}" \
    "$RESTIC_IMAGE" "$@"
}

backup_acquire_lock() {
  local lock_name="$1"
  backup_require_command flock
  exec 9>"$BACKUP_STATUS_DIR/$lock_name.lock"
  flock -n 9 || backup_error "$lock_name is already running"
}

backup_write_status() {
  local destination="$1"
  local payload="$2"
  local temporary
  temporary="$(mktemp "$BACKUP_STATUS_DIR/.status.XXXXXX")"
  chmod 644 "$temporary"
  printf '%s\n' "$payload" > "$temporary"
  mv "$temporary" "$destination"
}
