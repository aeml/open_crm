#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=scripts/lib/env-file.sh
source "$SCRIPT_DIR/lib/env-file.sh"

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

open_crm_load_env_keys "$DEPLOY_ENV_FILE" \
  OPEN_CRM_API_IMAGE_REPOSITORY \
  BACKUP_STATUS_DIR \
  ALLOW_CONTRACT_MIGRATIONS \
  OPEN_CRM_DEPLOY_STABILITY_SECONDS \
  OPEN_CRM_DEPLOY_RETAIN_IMAGES
export OPEN_CRM_RELEASE_ID="$requested_release"
export OPEN_CRM_ENV_FILE="$DEPLOY_ENV_FILE"
OPEN_CRM_API_IMAGE_REPOSITORY="${OPEN_CRM_API_IMAGE_REPOSITORY:-open-crm-api}"
export OPEN_CRM_API_IMAGE_REPOSITORY
deploy_stability_seconds="${OPEN_CRM_DEPLOY_STABILITY_SECONDS:-45}"
[[ "$deploy_stability_seconds" =~ ^([0-9]|[1-9][0-9]|1[01][0-9]|120)$ ]] || \
  deploy_error "OPEN_CRM_DEPLOY_STABILITY_SECONDS must be an integer from 0 through 120"
retain_release_images="${OPEN_CRM_DEPLOY_RETAIN_IMAGES:-5}"
if [[ ! "$retain_release_images" =~ ^[0-9]+$ ]] || \
  (( retain_release_images < 2 || retain_release_images > 50 )); then
  deploy_error "OPEN_CRM_DEPLOY_RETAIN_IMAGES must be an integer from 2 through 50"
fi
readiness_attempts="$(( 90 + (deploy_stability_seconds + 1) / 2 ))"

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

prune_release_images() {
  local current_release="$1"
  local previous_release="$2"
  local completed_at retention_status image release embedded_release created
  local candidate_line protected_release
  local eligible=0 kept=0 pruned=0 failures=0
  local -a candidates=()
  declare -A retained=()

  # Current and previous are the only supported application rollback pair and
  # are never eligible for cleanup, even when manual rollback makes them older
  # than the configured history window.
  for protected_release in "$current_release" "$previous_release"; do
    [[ -n "$protected_release" ]] || continue
    if [[ -z "${retained[$protected_release]+present}" ]]; then
      retained["$protected_release"]=true
      ((kept += 1))
    fi
  done

  # Limit cleanup to syntactically valid commit-style tags in the configured
  # repository whose immutable build label exactly names that tag. Foreign,
  # operator-managed, legacy, and malformed tags are ignored rather than
  # guessed at or force-removed.
  mapfile -t candidates < <(
    while IFS=$'\t' read -r image_repository release; do
      [[ "$image_repository" == "$OPEN_CRM_API_IMAGE_REPOSITORY" ]] || continue
      valid_release_id "$release" || continue
      image="$OPEN_CRM_API_IMAGE_REPOSITORY:$release"
      embedded_release="$(docker image inspect \
        --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' \
        "$image" 2>/dev/null || true)"
      [[ "$embedded_release" == "$release" ]] || continue
      created="$(docker image inspect --format '{{.Created}}' "$image" 2>/dev/null || true)"
      [[ -n "$created" ]] || continue
      printf '%s\t%s\n' "$created" "$release"
    done < <(docker image ls \
      --filter "reference=$OPEN_CRM_API_IMAGE_REPOSITORY:*" \
      --format '{{.Repository}}\t{{.Tag}}') \
      | sort -r -k1,1 -k2,2
  )
  eligible="${#candidates[@]}"

  for candidate_line in "${candidates[@]}"; do
    release="${candidate_line#*$'\t'}"
    if [[ -n "${retained[$release]+present}" ]]; then
      continue
    fi
    if (( kept < retain_release_images )); then
      retained["$release"]=true
      ((kept += 1))
      continue
    fi
    image="$OPEN_CRM_API_IMAGE_REPOSITORY:$release"
    echo "deploy_image_prune_selected release=$release image=$image"
    if docker image rm "$image" >/dev/null 2>&1; then
      ((pruned += 1))
      echo "deploy_image_pruned release=$release image=$image"
    else
      ((failures += 1))
      echo "deploy_image_prune_failed release=$release image=$image" >&2
    fi
  done

  retention_status="succeeded"
  if (( failures > 0 )); then
    retention_status="partial"
  fi
  completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  atomic_write "$DEPLOY_STATE_DIR/last-image-retention.json" \
    "$(printf '{\"status\":\"%s\",\"releaseId\":\"%s\",\"retainImages\":%d,\"eligibleImages\":%d,\"keptImages\":%d,\"prunedImages\":%d,\"failedImages\":%d,\"completedAt\":\"%s\"}' \
      "$retention_status" "$current_release" "$retain_release_images" "$eligible" "$kept" "$pruned" "$failures" "$completed_at")"
  echo "deploy_image_retention status=$retention_status release=$current_release retain_images=$retain_release_images eligible_images=$eligible kept_images=$kept pruned_images=$pruned failed_images=$failures"
}

wait_for_release() {
  local expected_release="$1"
  local allow_forced_failure="${2:-true}"
  if [[ "$allow_forced_failure" == "true" && "${OPEN_CRM_DEPLOY_TEST_FORCE_HEALTH_FAILURE:-false}" == "true" ]]; then
    return 1
  fi
  local healthy_since=""
  local healthy_instance=""
  for attempt in $(seq 1 "$readiness_attempts"); do
    local container_id health container_instance observed_release published_address published_port now stable_for
    container_id="$("${COMPOSE[@]}" ps -q api 2>/dev/null || true)"
    health=""
    if [[ -n "$container_id" ]]; then
      health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
      container_instance="$(docker inspect --format '{{.RestartCount}}:{{.State.StartedAt}}' "$container_id" 2>/dev/null || true)"
    fi
    published_address="$("${COMPOSE[@]}" port api 8080 2>/dev/null | head -n 1 || true)"
    published_port="${published_address##*:}"
    observed_release=""
    if [[ "$published_port" =~ ^[0-9]+$ ]]; then
      observed_release="$(curl --fail --silent --show-error --max-time 3 \
        --dump-header - --output /dev/null "http://127.0.0.1:${published_port}/readyz" 2>/dev/null \
        | tr -d '\r' | sed -n 's/^[Xx]-[Oo]pen-[Cc][Rr][Mm]-[Rr]elease: *//p' | head -n 1 || true)"
    fi
    now="$(date +%s)"
    stable_for=0
    if [[ "$health" == "healthy" && "$observed_release" == "$expected_release" ]]; then
      container_instance="$container_id:$container_instance"
      if [[ -z "$healthy_since" || "$healthy_instance" != "$container_instance" ]]; then
        healthy_since="$now"
        healthy_instance="$container_instance"
      fi
      stable_for="$(( now - healthy_since ))"
      if (( stable_for >= deploy_stability_seconds )); then
        echo "deploy_readiness_stable release=$expected_release stability_seconds=$stable_for"
        return 0
      fi
    else
      healthy_since=""
      healthy_instance=""
    fi
    if [[ "$attempt" -eq "$readiness_attempts" ]]; then
      echo "deploy_readiness_failed expected_release=$expected_release container_health=${health:-missing} observed_release=${observed_release:-missing} required_stability_seconds=$deploy_stability_seconds observed_stability_seconds=$stable_for" >&2
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

current_release=""
if [[ -f "$DEPLOY_STATE_DIR/current-release" ]]; then
  current_release="$(tr -d '[:space:]' < "$DEPLOY_STATE_DIR/current-release")"
  valid_release_id "$current_release" || deploy_error "stored current release ID is invalid"
fi

current_container="$("${COMPOSE[@]}" ps -q api 2>/dev/null || true)"
if [[ -n "$current_container" ]]; then
  current_image_id="$(docker inspect --format '{{.Image}}' "$current_container")"
  if [[ -z "$current_release" ]]; then
    current_release="legacy-$(date -u +%Y%m%d%H%M%S)"
  fi
  if ! docker image inspect "$OPEN_CRM_API_IMAGE_REPOSITORY:$current_release" >/dev/null 2>&1; then
    docker tag "$current_image_id" "$OPEN_CRM_API_IMAGE_REPOSITORY:$current_release"
  fi
fi

previous_release="$current_release"
if [[ -n "$current_release" && "$requested_release" == "$current_release" && \
  -f "$DEPLOY_STATE_DIR/previous-release" ]]; then
  recorded_previous_release="$(tr -d '[:space:]' < "$DEPLOY_STATE_DIR/previous-release")"
  valid_release_id "$recorded_previous_release" || \
    deploy_error "stored previous release ID is invalid"
  if [[ "$recorded_previous_release" != "$current_release" ]]; then
    previous_release="$recorded_previous_release"
    echo "deploy_preserving_rollback_target current_release=$current_release previous_release=$previous_release"
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

# When a healthy database already exists, prove that the newly uploaded
# credentials and migration image can reach it before Compose is allowed to
# recreate PostgreSQL for an environment or image change. Besides applying the
# same transactional expand migrations slightly earlier, --no-deps is the
# critical safety boundary: a stale/rotated secret fails while the current
# database and API containers are still untouched.
migrations_applied=false
existing_postgres="$("${COMPOSE[@]}" ps -q postgres 2>/dev/null || true)"
if [[ -n "$existing_postgres" ]] &&
  [[ "$(docker inspect --format '{{.State.Running}}' "$existing_postgres" 2>/dev/null || true)" == "true" ]]; then
  echo "deploy_database_preflight release=$requested_release"
  if ! "${COMPOSE[@]}" run --rm --no-deps migrate; then
    deploy_error "database credential or migration preflight failed before PostgreSQL restart; current containers were left unchanged"
  fi
  migrations_applied=true
fi
"${COMPOSE[@]}" up -d postgres
if [[ "$migrations_applied" != "true" ]]; then
  "${COMPOSE[@]}" run --rm migrate
fi
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
prune_release_images "$requested_release" "$previous_release"

"${COMPOSE[@]}" ps
echo "deploy_succeeded release=$requested_release previous_release=${previous_release:-none} image_id=$image_id"
