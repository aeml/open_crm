# Open CRM

[![Frontend Deploy](https://github.com/aeml/open_crm/actions/workflows/frontend-pages.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/frontend-pages.yml)
[![Backend Deploy](https://github.com/aeml/open_crm/actions/workflows/backend-deploy.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/backend-deploy.yml)
[![CI](https://github.com/aeml/open_crm/actions/workflows/ci.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/ci.yml)

> Built by [Robert Mendola](https://mendola.tech)

Open CRM is a production-capable CRM MVP built as a boring, explicit full-stack app: one Go API, one React web app, one Postgres database, and just enough product surface to be useful without turning into enterprise sludge.

## Live links
- App: https://crm.mendola.tech
- API: https://crmserver.mendola.tech
- Repo: https://github.com/aeml/open_crm

## Preview
![Open CRM dashboard screenshot](docs/media/dashboard-summary.png)
![Open CRM record detail workflow screenshot](docs/media/record-detail-workflow.png)

> README screenshots use disposable demo data captured from the live app so the product surface is visible without exposing real customer information.

## Why this project exists
- Ship a real CRM workflow surface without hiding behind fake architecture
- Prove that a modular monolith can stay clean, fast to debug, and production-ready
- Cover the day-to-day operator loop: contacts, companies, deals, tasks, notes, activity, and dashboard visibility
- Keep the stack small enough that runtime behavior is obvious when something breaks

## What ships today
- Workspace bootstrap and owner sign-in flow
- Organization users and role-aware settings
- Contacts, companies, and deals with searchable list + detail workflows
- Notes and tasks attached directly to contacts, companies, and deals
- Activity history for write operations
- Dashboard summary with live counts and recent activity
- Business-profile adaptation for different CRM operating modes
- Production deploys for both frontend and backend with documented recovery paths

## Technical highlights
- Go 1.23 API using `net/http`, `ServeMux`, `pgx/v5`, explicit SQL, and Postgres-backed sessions
- React 18 + Vite + React Router frontend with plain CSS and small reusable UI primitives
- PostgreSQL 16 as the source of truth for auth, CRM records, tasks, notes, and dashboard data
- Thin fetch-based API clients instead of a heavy frontend state framework
- Tracked SQL migrations with database-level constraints for core roles, statuses, entity types, monetary values, stage uniqueness, and contact-company links
- Vitest + Testing Library on the frontend, Go `testing` + `httptest` on the backend, and a Postgres-backed migration integrity test in CI
- CI gates for Go formatting, `go vet`, backend tests, frontend tests, frontend lint, and frontend production build
- GitHub Actions deployment split cleanly between static frontend hosting and SSH-based backend rollout

## What it demonstrates
- Full-stack product execution instead of toy scaffolding
- Clear runtime behavior over abstraction theater
- Boring deployment primitives that are easy to reason about
- Shipping useful operator workflows without dependency sprawl

## Architecture at a glance

```mermaid
graph LR
    Browser[User Browser] --> Web[React + Vite Web App]
    Web --> API[Go API]
    API --> Auth[Session + Auth Layer]
    API --> Modules[Contacts / Companies / Deals / Tasks / Notes / Dashboard]
    Modules --> DB[(PostgreSQL)]
```

## Stack

| Layer | Technology |
|-------|------------|
| Frontend | React 18, Vite, React Router, plain CSS |
| Backend | Go 1.23, stdlib `net/http`, `pgx/v5` |
| Auth | Argon2id password hashing, HttpOnly opaque session cookie, Postgres-backed sessions |
| Database | PostgreSQL 16 via Docker Compose |
| Testing | Vitest, Testing Library, Go `testing`, `httptest` |
| Deploy | GitHub Pages for frontend, GitHub Actions + SSH + Docker Compose for backend |

## Local development

Prerequisites:
- Go 1.23+
- Node.js 18.x and npm 9 or 10
- Docker with Compose plugin

Node is pinned with `.nvmrc` and `.node-version`. Use `nvm use`, `fnm use`, Volta, or your preferred version manager before installing frontend dependencies.

Quick start:

```bash
cp .env.example .env
make db-up
make db-migrate
make api-dev
make web-dev
```

Useful commands:

```bash
make db-up       # start Postgres
make db-down     # stop the compose stack
make db-migrate  # run migrations
make db-seed     # seed local data
make api-dev     # run the Go API
make web-dev     # run the Vite frontend
make test        # backend + frontend tests
```

Full release-candidate verification uses the same quality gates as CI:

```bash
cd apps/api && gofmt -l . && go vet ./... && go test ./...
cd apps/web && npm test && npm run lint && npm run build
```

When Postgres is available, the migration integrity test can be run locally by setting `OPEN_CRM_TEST_DATABASE_URL` before `go test ./...`. CI provides this automatically with a disposable PostgreSQL service.

## Repo layout

```text
open_crm/
├── apps/
│   ├── api/     # Go API, migrations, seed/migrate commands
│   └── web/     # React frontend
├── scripts/     # deploy helpers
├── .github/workflows/
│   ├── frontend-pages.yml
│   ├── ci.yml
│   └── backend-deploy.yml
├── docker-compose.deploy.yml
├── Makefile
└── mvp.md
```

## Deployment

Every push to `main` that changes application code runs CI before or alongside deployment. Backend CI includes a disposable Postgres service so migrations are applied against a real database and key constraints/indexes are verified.

Frontend deploy:
- `.github/workflows/frontend-pages.yml`
- Runs frontend tests, builds `apps/web`, and deploys to GitHub Pages
- Injects `VITE_API_BASE_URL=https://crmserver.mendola.tech`
- Copies `index.html` to `404.html` so client-side routing still works on Pages

Backend deploy:
- `.github/workflows/backend-deploy.yml`
- Syncs the repo to `aeml@ssh.mendola.tech:~/open_crm`
- Writes `.env.production` from the `DEPLOY_ENV` GitHub secret
- Runs `scripts/remote-deploy.sh` on the remote host
- Rebuilds the API, runs migrations, and preserves the existing Postgres volume
- Operational recovery, health checks, and database backup/restore are documented in `docs/operations-runbook.md`

Production safety baseline:
- SQL migrations are tracked in `schema_migrations` and reruns skip already-applied files.
- API runtime has request timeouts and graceful shutdown for deploy restarts.
- State-changing cookie-auth requests are protected by same-site CSRF checks.
- Auth/bootstrap endpoints are rate limited.
- API responses include `X-Request-Id`; production request logs are structured JSON.
- `/healthz` checks process health and `/readyz` checks dependencies.
- Backup, restore, migration recovery, and health-check procedures live in `docs/operations-runbook.md`.

Required GitHub secrets:
- `SSH_PRIVATE_KEY`
- `DEPLOY_ENV`

Example `DEPLOY_ENV`:

```env
POSTGRES_DB=open_crm
POSTGRES_USER=open_crm
POSTGRES_PASSWORD=***
API_PORT=18089
ALLOWED_ORIGINS=https://crm.mendola.tech
GO_ENV=production
```

## Product scope

This repo intentionally stays focused on the core CRM loop:
- organizations and users
- contacts and companies
- deals and pipeline tracking
- notes, tasks, and activity history
- dashboard visibility and basic filtering/search

Explicit non-goals for MVP:
- marketing automation
- email sync
- calendar sync
- workflow-engine nonsense
- microservices
- public API/platform ambitions before the core product earns them

## Status

This is a professional release-candidate foundation for a clean, operator-focused CRM. The core infrastructure baseline is complete: quality gates, reproducible tooling, migration safety, runtime hardening, security controls, operational runbooks, and database integrity are in place.

The next work should come from real usage friction and product feedback, not invented architecture projects.
