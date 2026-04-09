#!/usr/bin/env bash
set -euo pipefail

DEPLOY_PATH="${1:-$HOME/open_crm}"
cd "$DEPLOY_PATH"

if [[ ! -f .env.production ]]; then
  echo "Missing $DEPLOY_PATH/.env.production" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required on the remote host" >&2
  exit 1
fi

docker compose -f docker-compose.deploy.yml --env-file .env.production up -d --build --remove-orphans
docker compose -f docker-compose.deploy.yml --env-file .env.production ps
