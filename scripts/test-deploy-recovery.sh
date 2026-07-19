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

"$REPO_ROOT/scripts/remote-deploy.sh" "$REPO_ROOT" "$good_release"
grep -qx "$good_release" "$test_root/state/current-release"
grep -q '"status":"succeeded"' "$test_root/state/last-deploy.json"

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
