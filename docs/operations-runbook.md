# Operations Runbook

This runbook covers the production Docker Compose deployment used by `scripts/remote-deploy.sh`.

## Request Tracing

- Every API response includes `X-Request-Id`.
- API request logs include `request_id`, `method`, `path`, `status`, `duration_ms`, `remote_addr`, and response `bytes`.
- In production (`GO_ENV=production`), API logs are JSON so request IDs can be searched directly in log tooling.

## Deploy

From the remote host:

```sh
cd ~/open_crm
scripts/remote-deploy.sh
```

The deploy script builds the API image, starts Postgres, runs migrations once, and then restarts the API service.

## Deploy Recovery

Check service state:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production ps
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 api
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 migrate
```

Restart the API without rerunning migrations:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production up -d api
```

Rerun migrations after fixing a migration/dependency issue:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production run --rm migrate
docker compose -f docker-compose.deploy.yml --env-file .env.production up -d api
```

## Backup Postgres

Create a timestamped logical backup on the remote host:

```sh
mkdir -p backups
source .env.production
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres pg_dump -U "${POSTGRES_USER:-open_crm}" -d "${POSTGRES_DB:-open_crm}" --format=custom > "backups/open_crm_$(date +%Y%m%d_%H%M%S).dump"
```

Copy the backup off-host after creation. Treat backups as sensitive because they contain customer CRM data and password/session metadata.

## Restore Postgres

Restores replace current database contents. Take a fresh backup before restoring unless the database is already known to be disposable.

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production stop api
source .env.production
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres dropdb -U "${POSTGRES_USER:-open_crm}" --if-exists "${POSTGRES_DB:-open_crm}"
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres createdb -U "${POSTGRES_USER:-open_crm}" "${POSTGRES_DB:-open_crm}"
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres pg_restore -U "${POSTGRES_USER:-open_crm}" -d "${POSTGRES_DB:-open_crm}" --clean --if-exists < backups/open_crm_YYYYMMDD_HHMMSS.dump
docker compose -f docker-compose.deploy.yml --env-file .env.production run --rm migrate
docker compose -f docker-compose.deploy.yml --env-file .env.production up -d api
```

## Health Checks

- `GET /healthz` confirms the API process is serving HTTP.
- `GET /readyz` confirms required dependencies are reachable.
- The production API container healthcheck uses `/healthz` so Docker can restart unhealthy API containers.
