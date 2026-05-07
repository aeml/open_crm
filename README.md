# Open CRM

[![Frontend Deploy](https://github.com/aeml/open_crm/actions/workflows/frontend-pages.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/frontend-pages.yml)
[![Backend Deploy](https://github.com/aeml/open_crm/actions/workflows/backend-deploy.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/backend-deploy.yml)
[![CI](https://github.com/aeml/open_crm/actions/workflows/ci.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/ci.yml)

> Built by [Robert Mendola](https://mendola.tech)

Open CRM is a full-stack CRM and business operations platform built to support the everyday workflow of a small team: managing customers, companies, deals, tasks, notes, team users, and operational visibility in one system. It is designed to look and behave like normal business software rather than a demo app, with a clear product surface, explicit backend APIs, durable data storage, and deployment automation.

Live links:
- App: https://crm.mendola.tech
- API: https://crmserver.mendola.tech
- Repo: https://github.com/aeml/open_crm

Preview:

![Open CRM dashboard screenshot](docs/media/dashboard-summary.png)
![Open CRM record detail workflow screenshot](docs/media/record-detail-workflow.png)

## Overview

Open CRM is a production-oriented modular monolith made up of:
- a React frontend in `apps/web`
- a Go API in `apps/api`
- a PostgreSQL database
- Docker-based local and deployment workflows
- GitHub Actions CI/CD for testing and rollout

The application covers core CRM workflows: authentication, organization bootstrap, user management, contacts, companies, deals, tasks, notes, dashboard summaries, saved views, imports, exports, audit history, and notifications.

## Why This Project Matters

This repo is meant to demonstrate the kind of engineering used in practical business software:
- full-stack delivery across frontend, backend, database, and deployment
- authenticated CRUD workflows backed by real persistence and validation
- REST APIs with explicit route handling and tested failure paths
- Dockerized deployment and operational runbooks instead of hand-wavy infrastructure claims
- CI/CD quality gates for tests, linting, builds, formatting, and dependency hygiene
- a scalable architecture choice for a CRM product: a modular monolith that stays easy to debug and evolve

## Features

- Authentication with server-side sessions, secure cookie handling, setup-password onboarding, and rate-limited auth flows
- Multi-user organization support with role-aware settings, user management, profile updates, and preferences
- CRM records for contacts, companies, and deals
- Task and note workflows attached to CRM records
- Dashboard summaries for operational follow-up
- Saved views and filtering for list workflows
- CSV import preview for contacts and companies
- CSV export for contacts, companies, deals, and tasks
- Audit events for admin-facing lifecycle tracking
- Notifications and unread-count workflows
- Responsive React UI with route-based navigation and automated frontend tests

## Architecture

The backend is a Go `net/http` application with explicit route registration, module-level services, plain SQL via `pgx/v5`, and PostgreSQL-backed session storage. The frontend is a React SPA built with Vite and React Router. Deployment uses Docker Compose for the API and database, while GitHub Actions handles CI and release workflows.

```mermaid
graph LR
    Browser[Browser] --> Frontend[React + Vite SPA]
    Frontend --> API[Go REST API]
    API --> Auth[Authentication + Session Layer]
    API --> Modules[CRM Modules\nContacts, Companies, Deals, Tasks, Notes, Dashboard]
    Auth --> DB[(PostgreSQL 16)]
    Modules --> DB
    CI[GitHub Actions CI/CD] --> Frontend
    CI --> API
```

Engineering notes:
- Backend routes include auth, users, contacts, companies, deals, tasks, notes, saved views, imports, exports, notifications, audit events, `/healthz`, and `/readyz`.
- Authentication uses Argon2id password hashing and an `HttpOnly` same-site session cookie backed by database session records.
- The API applies CSRF protection for state-changing requests, structured request logging, request IDs, security headers, and readiness checks.
- The codebase documents major architectural decisions in `docs/adr/`.

## Tech Stack

| Layer | Technology |
|---|---|
| Frontend | React 18, Vite, React Router, JavaScript, CSS |
| Backend | Go 1.23, `net/http`, `ServeMux`, `pgx/v5` |
| Database | PostgreSQL 16 |
| Auth | Argon2id password hashing, server-side sessions, cookie authentication |
| Local infra | Docker, Docker Compose, Make |
| Deployment | Docker Compose, GitHub Actions, GitHub Pages, SSH-based backend rollout |
| Testing | Vitest, Testing Library, Go `testing`, `httptest` |

## Local Development

Prerequisites:
- Go 1.23+
- Node.js 18.x and npm 9 or 10
- Docker with Compose plugin

The frontend runtime is pinned in `.nvmrc` and `.node-version`.

Quick start:

```bash
cp .env.example .env
make db-up
make db-migrate
make api-dev
make web-dev
```

Useful repo commands:

```bash
make db-up
make db-down
make db-migrate
make db-seed
make api-dev
make web-dev
make test
```

Manual verification commands used by CI:

```bash
cd apps/api && gofmt -l . && go vet ./... && go test ./...
cd apps/web && npm test && npm run lint && npm run build
```

Local environment defaults come from `.env.example`:

```env
DATABASE_URL=postgres://open_crm:open_crm@localhost:5432/open_crm?sslmode=disable
API_PORT=8080
WEB_PORT=5173
API_BASE_URL=http://localhost:8080
WEB_BASE_URL=http://localhost:5173
```

## Deployment Notes

Frontend:
- `.github/workflows/frontend-pages.yml`
- Runs `npm test` and `npm run build:pages` from `apps/web`
- Deploys the built frontend to GitHub Pages

Backend:
- `.github/workflows/backend-deploy.yml`
- Syncs the repo to a remote host over SSH
- Writes `.env.production` from the `DEPLOY_ENV` secret
- Runs `scripts/remote-deploy.sh`, which builds the API image, starts PostgreSQL, runs migrations, and brings the API up with `docker-compose.deploy.yml`

Operational details already documented in the repo:
- health and readiness endpoints: `/healthz`, `/readyz`
- deploy recovery commands: `docs/operations-runbook.md`
- backup and restore procedures for PostgreSQL: `docs/operations-runbook.md`

## Testing

Current automated checks in `.github/workflows/ci.yml`:
- backend `go mod tidy` verification
- `gofmt -l .`
- `go vet ./...`
- backend `go test ./...`
- frontend `npm audit --audit-level=high`
- frontend `npm test`
- frontend `npm run lint`
- frontend `npm run build`

The backend CI job also runs against a disposable PostgreSQL service so migration and database-integrity behavior is exercised with a real database.

## Project Status

Open CRM is an actively developed portfolio project with a substantial implemented feature set across frontend, backend, data model, and deployment workflows.

Current state reflected in the repo:
- completed CRM foundation for contacts, companies, deals, tasks, notes, dashboard workflows, imports, exports, saved views, audit events, and notifications
- documented architectural decisions in `docs/adr/`
- documented operations and recovery steps in `docs/operations-runbook.md`
- version roadmap maintained in `docs/professionalization-roadmap.md`

This repo is intended to show the ability to build and operate business software end to end: React frontend, Go backend, PostgreSQL persistence, authentication, REST APIs, Dockerized deployment, and CI/CD.
