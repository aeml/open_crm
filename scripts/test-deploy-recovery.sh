#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/open-crm-deploy-test.XXXXXX")"
project_name="opencrm_deploy_test_$$"
image_repository="open-crm-deploy-test-$$"
good_release="test-good-$$"
next_release="test-next-$$"
failed_release="test-failed-$$"

cleanup() {
  OPEN_CRM_RELEASE_ID="$good_release" \
  OPEN_CRM_API_IMAGE_REPOSITORY="$image_repository" \
  OPEN_CRM_ENV_FILE="$test_root/test.env" \
  COMPOSE_PROJECT_NAME="$project_name" \
    docker compose -f "$REPO_ROOT/docker-compose.deploy.yml" --env-file "$test_root/test.env" \
    down -v --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$image_repository:$good_release" "$image_repository:$next_release" "$image_repository:$failed_release" >/dev/null 2>&1 || true
  rm -rf -- "$test_root"
}
trap cleanup EXIT

mkdir -p "$test_root/status" "$test_root/state"
cat > "$test_root/test.env" <<EOF
POSTGRES_DB=open_crm_deploy_test
POSTGRES_USER=open_crm
POSTGRES_PASSWORD=open_crm
ALLOWED_ORIGINS=http://localhost:5173
API_PORT=0
GO_ENV=production
EMAIL_FROM_NAME=Open CRM
OPEN_CRM_ENV_FILE=$test_root/test.env
BACKUP_STATUS_DIR=$test_root/status
EOF
chmod 600 "$test_root/test.env"

export COMPOSE_PROJECT_NAME="$project_name"
export DEPLOY_ENV_FILE="$test_root/test.env"
export DEPLOY_COMPOSE_FILE="$REPO_ROOT/docker-compose.deploy.yml"
export DEPLOY_STATE_DIR="$test_root/state"
export OPEN_CRM_API_IMAGE_REPOSITORY="$image_repository"
export OPEN_CRM_DEPLOY_STABILITY_SECONDS=2

compose_good_release() {
  OPEN_CRM_RELEASE_ID="$good_release" \
  OPEN_CRM_API_IMAGE_REPOSITORY="$image_repository" \
  OPEN_CRM_ENV_FILE="$test_root/test.env" \
    docker compose -f "$REPO_ROOT/docker-compose.deploy.yml" --env-file "$test_root/test.env" "$@"
}

if OPEN_CRM_DEPLOY_STABILITY_SECONDS=121 \
  "$REPO_ROOT/scripts/remote-deploy.sh" "$REPO_ROOT" "$good_release"; then
  echo "out-of-range deployment stabilization unexpectedly succeeded" >&2
  exit 1
fi

"$REPO_ROOT/scripts/remote-deploy.sh" "$REPO_ROOT" "$good_release"
grep -qx "$good_release" "$test_root/state/current-release"
grep -q '"status":"succeeded"' "$test_root/state/last-deploy.json"

published_address="$(compose_good_release port api 8080 | head -n 1)"
published_port="${published_address##*:}"
[[ "$published_port" =~ ^[0-9]+$ ]] || { echo "deployed API port is missing" >&2; exit 1; }
container_id="$(compose_good_release ps -q api)"
[[ -n "$container_id" ]] || { echo "deployed API container is missing" >&2; exit 1; }
restart_count_before="$(docker inspect --format '{{.RestartCount}}' "$container_id")"

# Reproduce the production daemon-restore ordering where the API starts before
# PostgreSQL is reachable. The process must never expose a health-only partial
# application; it exits until Docker can restart it with a usable database.
compose_good_release stop postgres
compose_good_release restart api
restart_deadline=$((SECONDS + 15))
restart_count_during="$restart_count_before"
while (( SECONDS < restart_deadline )); do
  published_address="$(compose_good_release port api 8080 2>/dev/null | head -n 1 || true)"
  published_port="${published_address##*:}"
  if [[ "$published_port" =~ ^[0-9]+$ ]] && \
    curl --fail --silent --show-error --max-time 1 \
      "http://127.0.0.1:${published_port}/healthz" >/dev/null 2>&1; then
    echo "API served health while PostgreSQL was unavailable during startup" >&2
    exit 1
  fi
  restart_count_during="$(docker inspect --format '{{.RestartCount}}' "$container_id")"
  if (( restart_count_during > restart_count_before )); then
    break
  fi
  sleep 1
done
if (( restart_count_during <= restart_count_before )); then
  echo "API did not exit and enter restart recovery while PostgreSQL was unavailable" >&2
  exit 1
fi

compose_good_release start postgres
recovery_deadline=$((SECONDS + 120))
recovered=false
while (( SECONDS < recovery_deadline )); do
  published_address="$(compose_good_release port api 8080 2>/dev/null | head -n 1 || true)"
  published_port="${published_address##*:}"
  container_health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
  observed_release=""
  if [[ "$published_port" =~ ^[0-9]+$ ]]; then
    observed_release="$(curl --fail --silent --show-error --max-time 2 \
      --dump-header - --output /dev/null "http://127.0.0.1:${published_port}/readyz" 2>/dev/null \
      | tr -d '\r' | sed -n 's/^[Xx]-[Oo]pen-[Cc][Rr][Mm]-[Rr]elease: *//p' | head -n 1 || true)"
  fi
  if [[ "$container_health" == "healthy" && "$observed_release" == "$good_release" ]]; then
    recovered=true
    break
  fi
  sleep 2
done
if [[ "$recovered" != "true" ]]; then
  compose_good_release logs --tail=100 api postgres >&2 || true
  echo "API did not recover exact-release readiness after PostgreSQL returned" >&2
  exit 1
fi
echo "database_startup_recovery_succeeded release=$good_release restart_count=$restart_count_during"

"$REPO_ROOT/scripts/remote-deploy.sh" "$REPO_ROOT" "$next_release"
grep -qx "$next_release" "$test_root/state/current-release"
grep -qx "$good_release" "$test_root/state/previous-release"

"$REPO_ROOT/scripts/rollback-release.sh" "$REPO_ROOT" "$good_release"
grep -qx "$good_release" "$test_root/state/current-release"
grep -qx "$next_release" "$test_root/state/previous-release"
grep -q '"status":"manually_rolled_back"' "$test_root/state/last-deploy.json"

if OPEN_CRM_DEPLOY_TEST_FORCE_HEALTH_FAILURE=true \
  "$REPO_ROOT/scripts/remote-deploy.sh" "$REPO_ROOT" "$failed_release"; then
  echo "forced unhealthy deployment unexpectedly succeeded" >&2
  exit 1
fi

grep -qx "$good_release" "$test_root/state/current-release"
grep -q '"status":"rolled_back"' "$test_root/state/last-deploy.json"
container_id="$(OPEN_CRM_RELEASE_ID="$good_release" docker compose \
  -f "$REPO_ROOT/docker-compose.deploy.yml" --env-file "$test_root/test.env" ps -q api)"
[[ -n "$container_id" ]] || { echo "rolled-back API container is missing" >&2; exit 1; }
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container_id" \
  | grep -qx "OPEN_CRM_RELEASE_ID=$good_release"
docker exec "$container_id" test -r /app/LICENSE
docker exec "$container_id" grep -Fqx '# Third-Party Notices' /app/THIRD_PARTY_NOTICES.md

if OPEN_CRM_ROLLBACK_TEST_FORCE_HEALTH_FAILURE=true \
  "$REPO_ROOT/scripts/rollback-release.sh" "$REPO_ROOT" "$next_release"; then
  echo "forced unhealthy manual rollback unexpectedly succeeded" >&2
  exit 1
fi
grep -qx "$good_release" "$test_root/state/current-release"
grep -q '"status":"manual_rollback_recovered"' "$test_root/state/last-deploy.json"

sed -i 's/"rollbackSafe":true/"rollbackSafe":false/' "$test_root/state/releases/$good_release/manifest.json"
if "$REPO_ROOT/scripts/rollback-release.sh" "$REPO_ROOT" "$next_release"; then
  echo "manual rollback across a contract release unexpectedly succeeded" >&2
  exit 1
fi
grep -qx "$good_release" "$test_root/state/current-release"

python3 -m json.tool "$test_root/state/last-deploy.json" >/dev/null
python3 -m json.tool "$test_root/state/releases/$good_release/manifest.json" >/dev/null
python3 -m json.tool "$test_root/state/releases/$next_release/manifest.json" >/dev/null

echo "deploy_recovery_acceptance_succeeded restored_release=$good_release"
