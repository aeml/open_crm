SHELL := /bin/bash

API_DIR := apps/api
WEB_DIR := apps/web

.PHONY: db-up db-down api-dev web-dev test-api test-web test

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

api-dev:
	cd $(API_DIR) && go run ./cmd/open_crm_api

web-dev:
	cd $(WEB_DIR) && npm run dev

test-api:
	cd $(API_DIR) && go test ./...

test-web:
	cd $(WEB_DIR) && npm test

test: test-api test-web
