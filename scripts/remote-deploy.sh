#!/usr/bin/env bash
set -euo pipefail

DEPLOY_PATH="${1:-$HOME/open_crm}"
requested_release="${2:-${OPEN_CRM_RELEASE_ID:-}}"
DEPLOY_ENV_FILE="${DEPLOY_ENV_FILE:-$DEPLOY_PATH/.env.production}"
DEPLOY_COMPOSE_FILE="${DEPLOY_COMPOSE_FILE:-$DEPLOY_PATH/docker-compose.deploy.yml}"
DEPLOY_STATE_DIR="${DEPLOY_STATE_DIR:-$DEPLOY_PATH/var/deploy}"

deploy_error() {
  echo "deploy_error: $*" >&2
  exit 1
}

valid_release_id() {
  [[ "$1" =~ ^[a-z0-9][a-z0-9._-]{6,63}$ ]]
}

[[ -d "$DEPLOY_PATH" ]] || deploy_error "deployment path does not exist: $DEPLOY_PATH"
cd "$DEPLOY_PATH"
[[ -f "$DEPLOY_ENV_FILE" ]] || deploy_error "missing environment file: $DEPLOY_ENV_FILE"
[[ -f "$DEPLOY_COMPOSE_FILE" ]] || deploy_error "missing compose file: $DEPLOY_COMPOSE_FILE"
valid_release_id "$requested_release" || deploy_error "release ID must be 7-64 lowercase letters, digits, dots, underscores, or hyphens"
for command_name in curl docker flock install; do
  command -v "$command_name" >/dev/null 2>&1 || deploy_error "$command_name is required"
done

set -a
# shellcheck disable=SC1090
source "$DEPLOY_ENV_FILE"
set +a
export OPEN_CRM_RELEASE_ID="$requested_release"
export OPEN_CRM_ENV_FILE="$DEPLOY_ENV_FILE"
OPEN_CRM_API_IMAGE_REPOSITORY="${OPEN_CRM_API_IMAGE_REPOSITORY:-open-crm-api}"
export OPEN_CRM_API_IMAGE_REPOSITORY

mkdir -p "$DEPLOY_STATE_DIR" "$DEPLOY_STATE_DIR/releases"
chmod 755 "$DEPLOY_STATE_DIR" "$DEPLOY_STATE_DIR/releases"
exec 8>"$DEPLOY_STATE_DIR/deploy.lock"
flock -n 8 || deploy_error "another deployment is already running"

COMPOSE=(docker compose -f "$DEPLOY_COMPOSE_FILE" --env-file "$DEPLOY_ENV_FILE")
started_epoch="$(date +%s)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

atomic_write() {
  local destination="$1"
  local contents="$2"
  local temporary
  temporary="$(mktemp "$DEPLOY_STATE_DIR/.deploy-state.XXXXXX")"
  chmod 644 "$temporary"
  printf '%s\n' "$contents" > "$temporary"
  mv "$temporary" "$destination"
}

write_deploy_status() {
  local status="$1"
  local phase="$2"
  local release="$3"
  local previous="$4"
  local completed_at duration
  completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  duration="$(( $(date +%s) - started_epoch ))"
  atomic_write "$DEPLOY_STATE_DIR/last-deploy.json" \
    "$(printf '{\"status\":\"%s\",\"phase\":\"%s\",\"releaseId\":\"%s\",\"previousReleaseId\":\"%s\",\"startedAt\":\"%s\",\"completedAt\":\"%s\",\"durationSeconds\":%d}' \
      "$status" "$phase" "$release" "$previous" "$started_at" "$completed_at" "$duration")"
}

wait_for_release() {
  local expected_release="$1"
  local allow_forced_failure="${2:-true}"
  if [[ "$allow_forced_failure" == "true" && "${OPEN_CRM_DEPLOY_TEST_FORCE_HEALTH_FAILURE:-false}" == "true" ]]; then
    return 1
  fi
  for attempt in $(seq 1 30); do
    local container_id health observed_release published_address published_port
    container_id="$("${COMPOSE[@]}" ps -q api 2>/dev/null || true)"
    health=""
    if [[ -n "$container_id" ]]; then
      health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
    fi
    published_address="$("${COMPOSE[@]}" port api 8080 2>/dev/null | head -n 1 || true)"
    published_port="${published_address##*:}"
    observed_release=""
    if [[ "$published_port" =~ ^[0-9]+$ ]]; then
      observed_release="$(curl --fail --silent --show-error --max-time 3 \
        --dump-header - --output /dev/null "http://127.0.0.1:${published_port}/readyz" 2>/dev/null \
        | tr -d '\r' | sed -n 's/^[Xx]-[Oo]pen-[Cc][Rr][Mm]-[Rr]elease: *//p' | head -n 1 || true)"
    fi
    if [[ "$health" == "healthy" && "$observed_release" == "$expected_release" ]]; then
      return 0
    fi
    if [[ "$attempt" -eq 30 ]]; then
      echo "deploy_readiness_failed expected_release=$expected_release container_health=${health:-missing} observed_release=${observed_release:-missing}" >&2
      return 1
    fi
    sleep 2
  done
}

# The unprivileged API reads only non-secret backup status evidence from this
# directory. Backup data and repository credentials are never mounted here.
backup_status_dir="${BACKUP_STATUS_DIR:-$DEPLOY_PATH/var/backup-status}"
if [[ "$backup_status_dir" != /* ]]; then
  backup_status_dir="$DEPLOY_PATH/$backup_status_dir"
fi
install -d -m 755 "$backup_status_dir"

previous_release=""
if [[ -f "$DEPLOY_STATE_DIR/current-release" ]]; then
  previous_release="$(tr -d '[:space:]' < "$DEPLOY_STATE_DIR/current-release")"
  valid_release_id "$previous_release" || deploy_error "stored current release ID is invalid"
fi

current_container="$("${COMPOSE[@]}" ps -q api 2>/dev/null || true)"
if [[ -n "$current_container" ]]; then
  current_image_id="$(docker inspect --format '{{.Image}}' "$current_container")"
  if [[ -z "$previous_release" ]]; then
    previous_release="legacy-$(date -u +%Y%m%d%H%M%S)"
  fi
  if ! docker image inspect "$OPEN_CRM_API_IMAGE_REPOSITORY:$previous_release" >/dev/null 2>&1; then
    docker tag "$current_image_id" "$OPEN_CRM_API_IMAGE_REPOSITORY:$previous_release"
  fi
fi

rollback_allowed=true
if [[ "${ALLOW_CONTRACT_MIGRATIONS:-false}" == "true" ]]; then
  rollback_allowed=false
fi

"${COMPOSE[@]}" pull postgres || true
release_image="$OPEN_CRM_API_IMAGE_REPOSITORY:$requested_release"
if docker image inspect "$release_image" >/dev/null 2>&1; then
  embedded_release="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$release_image")"
  [[ "$embedded_release" == "$requested_release" ]] || \
    deploy_error "existing release image has mismatched embedded identity: $release_image"
  echo "deploy_reusing_release_image release=$requested_release image=$release_image"
else
  "${COMPOSE[@]}" build api
fi
"${COMPOSE[@]}" up -d postgres
"${COMPOSE[@]}" run --rm migrate
"${COMPOSE[@]}" up -d api --remove-orphans --force-recreate

if ! wait_for_release "$requested_release" true; then
  write_deploy_status "failed" "readiness" "$requested_release" "$previous_release"
  if [[ "$rollback_allowed" == "true" && -n "$previous_release" ]] && \
    docker image inspect "$OPEN_CRM_API_IMAGE_REPOSITORY:$previous_release" >/dev/null 2>&1; then
    export OPEN_CRM_RELEASE_ID="$previous_release"
    "${COMPOSE[@]}" up -d api --no-build --force-recreate
    if wait_for_release "$previous_release" false; then
      write_deploy_status "rolled_back" "readiness" "$requested_release" "$previous_release"
      echo "deploy_rolled_back failed_release=$requested_release restored_release=$previous_release" >&2
    else
      write_deploy_status "rollback_failed" "readiness" "$requested_release" "$previous_release"
      echo "deploy_rollback_failed failed_release=$requested_release attempted_release=$previous_release" >&2
    fi
  else
    echo "deploy_rollback_unavailable failed_release=$requested_release previous_release=${previous_release:-none} rollback_allowed=$rollback_allowed" >&2
  fi
  exit 1
fi

release_dir="$DEPLOY_STATE_DIR/releases/$requested_release"
install -d -m 755 "$release_dir"
install -m 644 "$DEPLOY_COMPOSE_FILE" "$release_dir/docker-compose.deploy.yml"
image_id="$(docker image inspect --format '{{.Id}}' "$release_image")"
atomic_write "$release_dir/manifest.json" \
  "$(printf '{\"releaseId\":\"%s\",\"image\":\"%s:%s\",\"imageId\":\"%s\",\"rollbackSafe\":%s,\"deployedAt\":\"%s\"}' \
    "$requested_release" "$OPEN_CRM_API_IMAGE_REPOSITORY" "$requested_release" "$image_id" "$rollback_allowed" "$(date -u +%Y-%m-%dT%H:%M:%SZ)")"
atomic_write "$DEPLOY_STATE_DIR/previous-release" "$previous_release"
atomic_write "$DEPLOY_STATE_DIR/current-release" "$requested_release"
write_deploy_status "succeeded" "complete" "$requested_release" "$previous_release"

"${COMPOSE[@]}" ps
echo "deploy_succeeded release=$requested_release previous_release=${previous_release:-none} image_id=$image_id"
