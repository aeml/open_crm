#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/env-file.sh
source "$SCRIPT_DIR/lib/env-file.sh"

DEPLOY_PATH="${1:-$HOME/open_crm}"
requested_target="${2:-}"
DEPLOY_ENV_FILE="${DEPLOY_ENV_FILE:-$DEPLOY_PATH/.env.production}"
DEPLOY_COMPOSE_FILE="${DEPLOY_COMPOSE_FILE:-$DEPLOY_PATH/docker-compose.deploy.yml}"
DEPLOY_STATE_DIR="${DEPLOY_STATE_DIR:-$DEPLOY_PATH/var/deploy}"

rollback_error() {
  echo "rollback_error: $*" >&2
  exit 1
}

valid_release_id() {
  [[ "$1" =~ ^[a-z0-9][a-z0-9._-]{6,63}$ ]]
}

[[ -d "$DEPLOY_PATH" ]] || rollback_error "deployment path does not exist: $DEPLOY_PATH"
cd "$DEPLOY_PATH"
[[ -f "$DEPLOY_ENV_FILE" ]] || rollback_error "missing environment file: $DEPLOY_ENV_FILE"
[[ -f "$DEPLOY_COMPOSE_FILE" ]] || rollback_error "missing compose file: $DEPLOY_COMPOSE_FILE"
for command_name in curl docker flock; do
  command -v "$command_name" >/dev/null 2>&1 || rollback_error "$command_name is required"
done

mkdir -p "$DEPLOY_STATE_DIR"
exec 8>"$DEPLOY_STATE_DIR/deploy.lock"
flock -n 8 || rollback_error "another deployment or rollback is already running"

[[ -f "$DEPLOY_STATE_DIR/current-release" ]] || rollback_error "current release state is missing"
[[ -f "$DEPLOY_STATE_DIR/previous-release" ]] || rollback_error "previous release state is missing"
current_release="$(tr -d '[:space:]' < "$DEPLOY_STATE_DIR/current-release")"
previous_release="$(tr -d '[:space:]' < "$DEPLOY_STATE_DIR/previous-release")"
target_release="${requested_target:-$previous_release}"
valid_release_id "$current_release" || rollback_error "stored current release ID is invalid"
valid_release_id "$previous_release" || rollback_error "stored previous release ID is invalid"
valid_release_id "$target_release" || rollback_error "target release ID is invalid"
[[ "$target_release" == "$previous_release" ]] || rollback_error "only the recorded previous release can be selected: $previous_release"
[[ "$target_release" != "$current_release" ]] || rollback_error "target release is already current"

current_manifest="$DEPLOY_STATE_DIR/releases/$current_release/manifest.json"
target_manifest="$DEPLOY_STATE_DIR/releases/$target_release/manifest.json"
[[ -f "$current_manifest" ]] || rollback_error "current release manifest is missing: $current_manifest"
[[ -f "$target_manifest" ]] || rollback_error "target release manifest is missing: $target_manifest"
grep -Fq '"rollbackSafe":true' "$current_manifest" || \
  rollback_error "current release applied contract migrations; deploy a forward fix or restore the database instead of rolling the app back"

open_crm_load_env_keys "$DEPLOY_ENV_FILE" OPEN_CRM_API_IMAGE_REPOSITORY
OPEN_CRM_API_IMAGE_REPOSITORY="${OPEN_CRM_API_IMAGE_REPOSITORY:-open-crm-api}"
export OPEN_CRM_API_IMAGE_REPOSITORY
export OPEN_CRM_RELEASE_ID="$target_release"
export OPEN_CRM_ENV_FILE="$DEPLOY_ENV_FILE"
docker image inspect "$OPEN_CRM_API_IMAGE_REPOSITORY:$target_release" >/dev/null 2>&1 || \
  rollback_error "target image is unavailable: $OPEN_CRM_API_IMAGE_REPOSITORY:$target_release"

COMPOSE=(docker compose -f "$DEPLOY_COMPOSE_FILE" --env-file "$DEPLOY_ENV_FILE")
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
started_epoch="$(date +%s)"

atomic_write() {
  local destination="$1"
  local contents="$2"
  local temporary
  temporary="$(mktemp "$DEPLOY_STATE_DIR/.rollback-state.XXXXXX")"
  chmod 644 "$temporary"
  printf '%s\n' "$contents" > "$temporary"
  mv "$temporary" "$destination"
}

wait_for_release() {
  local expected_release="$1"
  local allow_forced_failure="${2:-true}"
  if [[ "$allow_forced_failure" == "true" && "${OPEN_CRM_ROLLBACK_TEST_FORCE_HEALTH_FAILURE:-false}" == "true" ]]; then
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
      echo "rollback_readiness_failed expected_release=$expected_release container_health=${health:-missing} observed_release=${observed_release:-missing}" >&2
      return 1
    fi
    sleep 2
  done
}

"${COMPOSE[@]}" up -d api --no-build --force-recreate
if ! wait_for_release "$target_release" true; then
  recovery_status="manual_rollback_failed"
  export OPEN_CRM_RELEASE_ID="$current_release"
  if docker image inspect "$OPEN_CRM_API_IMAGE_REPOSITORY:$current_release" >/dev/null 2>&1; then
    "${COMPOSE[@]}" up -d api --no-build --force-recreate
    if wait_for_release "$current_release" false; then
      recovery_status="manual_rollback_recovered"
    fi
  fi
  atomic_write "$DEPLOY_STATE_DIR/last-deploy.json" \
    "$(printf '{\"status\":\"%s\",\"phase\":\"readiness\",\"releaseId\":\"%s\",\"previousReleaseId\":\"%s\",\"startedAt\":\"%s\",\"completedAt\":\"%s\",\"durationSeconds\":%d}' \
      "$recovery_status" "$target_release" "$current_release" "$started_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(( $(date +%s) - started_epoch ))")"
  rollback_error "target release did not become ready; recovery status: $recovery_status"
fi

atomic_write "$DEPLOY_STATE_DIR/current-release" "$target_release"
atomic_write "$DEPLOY_STATE_DIR/previous-release" "$current_release"
atomic_write "$DEPLOY_STATE_DIR/last-deploy.json" \
  "$(printf '{\"status\":\"manually_rolled_back\",\"phase\":\"complete\",\"releaseId\":\"%s\",\"previousReleaseId\":\"%s\",\"startedAt\":\"%s\",\"completedAt\":\"%s\",\"durationSeconds\":%d}' \
    "$target_release" "$current_release" "$started_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(( $(date +%s) - started_epoch ))")"

"${COMPOSE[@]}" ps
echo "rollback_succeeded restored_release=$target_release replaced_release=$current_release"
