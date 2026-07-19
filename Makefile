SHELL := /bin/bash

API_DIR := apps/api
WEB_DIR := apps/web

.PHONY: db-up db-down db-migrate db-seed api-dev web-dev test-api test-web test-backup-restore test-deploy-recovery test-monitoring test

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-migrate:
	cd $(API_DIR) && go run ./cmd/migrate

db-seed:
	cd $(API_DIR) && go run ./cmd/seed

api-dev:
	cd $(API_DIR) && go run ./cmd/open_crm_api

web-dev:
	cd $(WEB_DIR) && npm run dev

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
