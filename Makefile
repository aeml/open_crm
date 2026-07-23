SHELL := /bin/bash

API_DIR := apps/api
WEB_DIR := apps/web
ENV_FILE ?= .env
DEV_ENV := ./scripts/run-with-env.sh --if-present "$(ENV_FILE)" --
DEV_COMPOSE_PROJECT ?= open-crm-dev
DEV_COMPOSE := docker compose --project-name "$(DEV_COMPOSE_PROJECT)" --file docker-compose.yml

.PHONY: db-up db-down db-migrate db-seed api-dev web-dev check-dev-environment check-licenses test-api test-web test-backup-restore test-deploy-recovery test-monitoring test

db-up:
	$(DEV_COMPOSE) up -d postgres

db-down:
	$(DEV_COMPOSE) down

db-migrate:
	$(DEV_ENV) go -C $(API_DIR) run ./cmd/migrate

db-seed:
	$(DEV_ENV) go -C $(API_DIR) run ./cmd/seed

api-dev:
	$(DEV_ENV) go -C $(API_DIR) run ./cmd/open_crm_api

web-dev:
	$(DEV_ENV) npm --prefix $(WEB_DIR) run dev

check-dev-environment:
	./scripts/test-dev-environment.sh

check-licenses:
	node scripts/check-third-party-notices.mjs --check

test-api:
	cd $(API_DIR) && go test ./...

test-web:
	cd $(WEB_DIR) && npm test

test-backup-restore:
	scripts/test-backup-restore.sh

test-deploy-recovery:
	scripts/test-deploy-recovery.sh

test-monitoring:
	docker run --rm -v "$(CURDIR)/ops/monitoring:/monitoring:ro" --entrypoint /bin/promtool prom/prometheus:v3.12.0@sha256:69f5241418838263316593f7274a304b095c40bcf22e57272865da91bd60a8ac check rules /monitoring/prometheus-alerts.yml

test: test-api test-web
