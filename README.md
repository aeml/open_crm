# Open CRM

[![Frontend Deploy](https://github.com/aeml/open_crm/actions/workflows/frontend-pages.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/frontend-pages.yml)
[![Backend Deploy](https://github.com/aeml/open_crm/actions/workflows/backend-deploy.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/backend-deploy.yml)
[![CI](https://github.com/aeml/open_crm/actions/workflows/ci.yml/badge.svg)](https://github.com/aeml/open_crm/actions/workflows/ci.yml)

> Built by [Robert Mendola](https://mendola.tech)

Open CRM is a full-stack, self-hostable revenue-and-client operations CRM for small B2B service teams. It is designed around a coherent lead-to-client workflow—ownership, communication, pipeline, quoting, and handoff—without requiring enterprise CRM administration or an oversized dependency stack.

Live links:
- App: https://crm.mendola.tech
- API: https://crmserver.mendola.tech
- Repo: https://github.com/aeml/open_crm

Product direction and current maturity are documented in:

- [`docs/product-vision.md`](docs/product-vision.md) — target customer, critical journey, and convergence scope
- [`docs/capability-matrix.md`](docs/capability-matrix.md) — canonical evidence-backed capability status
- [`docs/security-surface-inventory.md`](docs/security-surface-inventory.md) — route/background-operation controls and known evidence gaps
- [`docs/project-convergence-goal.md`](docs/project-convergence-goal.md) — execution order and definition of done
- [`docs/sales-workflow.md`](docs/sales-workflow.md) — supported pilot sales path, semantics, boundaries, and feedback record

## Overview

Open CRM is a production-oriented modular monolith made up of:
- a React frontend in `apps/web`
- a Go API in `apps/api`
- a PostgreSQL database
- Docker-based local and deployment workflows
- GitHub Actions CI/CD for testing and rollout

The production-capable core covers verified, idempotent workspace signup, authentication, user roles and lifecycle, contacts, companies, deals, tasks, notes, record followers, teammate mentions, focused activity, admin-configurable pipelines with stable stages, explainable probability-weighted forecasting, snapshot-backed sales activity reporting, transactional won-deal client handoff and account context, traceable contact/client touchpoint history, stale follow-up queues, explainable derived client health, recurring client review/renewal tasks, bounded deal follow-up task rules, mapped CSV imports with rollback, reversible bulk maintenance, reviewed contact/client duplicate merge, bounded typed contact/client custom fields, explicit archived-record recovery, live explainable data-quality queues, saved views, bounded operational CSV exports, durable full-workspace portability exports, audit history, and organization-scoped access. The repository also contains broad post-MVP foundations for billing, mailbox sync, sequences, calling/SMS/calendar, quoting, lead generation, general-purpose workflows, and custom reporting. Those foundations have different maturity levels and are not all complete product outcomes; the capability matrix is authoritative, and incomplete booking/marketing management foundations are hidden from production navigation.

## Why This Project Matters

This repo is meant to demonstrate the kind of engineering used in practical business software:
- full-stack delivery across frontend, backend, database, and deployment
- authenticated CRUD workflows backed by real persistence and validation
- REST APIs with explicit route handling and tested failure paths
- Dockerized deployment and operational runbooks instead of hand-wavy infrastructure claims
- CI/CD quality gates for tests, linting, builds, formatting, and dependency hygiene
- a scalable architecture choice for a CRM product: a modular monolith that stays easy to debug and evolve

## Current Product Surface

Production-capable core:

- Verified self-serve workspace provisioning with stable retry keys, one-time 24-hour email links, no pre-verification session, a verification-started 14-day trial, safe resend recovery, and bounded public auth/signup flows
- Server-side session authentication with a user-visible active-sign-in list, protected individual/all-other revocation, one-time password setup, and self-service one-hour password recovery that atomically invalidates every session—even against a concurrent old-password login—alongside CSRF/origin protection and shared PostgreSQL-backed public/auth abuse budgets that coordinate across restarts and replicas without retaining raw client addresses
- Organization-scoped owner/admin/member/viewer roles and tenant isolation
- Contacts, companies, deals, tasks, notes, activity, ownership, filters, pagination, and saved views
- Admin-managed pipelines with bounded stage creation, renaming, outcome classification, configurable open-stage probability, exact reordering, default selection, and stable stage identities for existing deals
- Explainable period forecasts with unweighted, won, stage-weighted, owner, unassigned, and stage-assumption rollups plus close-date filter parity across deal lists, saved views, and CSV exports
- Snapshot-backed sales activity reports with bounded UTC date/teammate filters, exact deal and follow-up counts, outcome win rate, event-based stage movement, fixed won/lost reason breakdowns with close-note context, honest history coverage, and deal drill-down
- Transactional won-deal handoff that requires an account relationship, promotes the explicit organization—or a company-less primary contact—into the existing client model, records activity/audit evidence, and links to a one-page rollup of won deals, open account tasks, recent notes, and key people
- Viewer-aware contact/client touchpoint summaries and stale follow-up queues with explicit source, creation-fallback, linked-person, and privacy semantics
- Derived Healthy, Watch, and Needs attention client signals with exact stale-follow-up and open-task reasons, reusable scoped customer segments, organization/individual and health filters, company-linked-person rollup, and no fabricated issue claim
- Adaptive service profiles that present the mature pipeline record as a Job and linked delivery tasks as Service/Site Tasks without advertising a separate project module
- Task-backed client review and renewal schedules with assigned ownership, one-time or recurring cadence, durable reminders, late-period skip semantics, dashboard visibility, and recovery-safe client/task guards
- Tenant-scoped deal line items and totals, immutable versioned quote PDFs with recipient/terms/validity/digest evidence, and explicitly manual proposal status tracking that does not claim delivery or legal e-signature
- Admin-managed deal rules that create one assigned, auditable, idempotent follow-up task on deal creation, a real stage change, or archive
- Exact overdue and rolling-24-hour task surfaces with preference-aware assignment events and durable, replay-safe in-app reminders
- Record following, explicit teammate mentions, relevant notification links, and followed/team activity digests
- Dry-run CSV mapping, idempotent tracked imports, row error downloads, bounded bulk archive/reassignment plus contact/client status and task-completion changes with safe rollback, field-resolved contact/client duplicate merge, and admin-defined typed contact/client fields carried through forms, filters, saved views, imports, exports, and duplicate review
- Searchable contact/client/deal/task archive history with role-aware restore, dependency safeguards, permanent merge-source protection, retained relationships, and transactional activity/audit evidence
- Live data-quality queues for missing ownership/contact details, stale or incomplete deals, unscheduled tasks, and business-profile-specific client/account gaps, with explainable counts and direct cleanup links
- Tenant-scoped CSV exports for all four core record types, with custom columns, visible-filter parity, a tested 10,000-row synchronous ceiling, and explicit refusal instead of silent truncation
- Owner/admin full-workspace ZIP exports generated by the durable job queue from one repeatable-read snapshot, with explicit dataset coverage, secret/private-mail redaction, integrity checksum, seven-day expiry, request/ready/download audit, and availability during hosted suspension
- Audit events, notification center, and documented archive-with-history-retention semantics
- Responsive React UI with route-level tests and consistent loading/error/permission states

In convergence:

- Billing retains a fake provider for self-hosted/development plan switching, but only a runtime configured with Stripe is managed: self-hosted sessions report `unmanaged`, usage is shown without hosted ceilings, and stale trial/subscription fields cannot restrict private writes, workers, or public lead capture. Stripe mode has backend-hosted Checkout and customer-portal sessions, raw-body signed/idempotent webhooks, authoritative subscription/trial/dunning/cancellation state, an invoice ledger, six-hour provider reconciliation through the durable job queue, admin replay, provider metrics, centralized tenant-mutation suspension, attempt-preserving worker deferral, and a suspension-safe portable workspace export. Plan & Billing gives owners/admins suspension-safe, newest-first invoice history with amounts, provider retry evidence, and sanitized HTTPS invoice/PDF links; Stripe remains authoritative for retry timing. The page and an exempt once-daily durable worker reconcile the same explainable period snapshot of active seats/records, sent outbound email, successful automation/background jobs, and estimated tenant PostgreSQL row bytes; they use an exact Stripe period when available and a UTC month otherwise, retry bounded serialization conflicts, and clearly avoid unapproved new quotas. The hosted catalog currently exposes only the subscription and seat/contact/deal capacity contract it enforces: unapproved feature arrays are empty, unfinished API/SSO/automation/reporting outcomes are not advertised as plan benefits, and Stripe Checkout—not a hard-coded UI hint—confirms the actual recurring price. Those three capacities use durable tenant-scoped claims that are consumed atomically with direct creates, user reactivation, imports, public lead capture, and single/bulk archive recovery; concurrent requests see either the claim or committed record, and abandoned claims expire. Managed authentication returns a server-derived access snapshot; a persistent read-only/unavailable banner and shared role-aware controls remove normal mutation actions while reads, exports, self-account maintenance, job recovery, and billing recovery remain available. Managed public lead writes fail closed without exposing tenant billing state. CI connects the production adapter and app routes to a credential-free Stripe HTTP contract sandbox and real PostgreSQL for Checkout, signed-event, reconciliation, tenant invoice visibility, dunning, recovery, cancellation, and quota-concurrency acceptance. It remains a foundation until an approved credentialed Stripe test-mode deployment smoke, explicit portal/proration/resubscription and feature-tier policy, any approved message/automation/job/storage quotas, dunning timing, and tenant deletion/retention policy are complete
- System identity email has a real Postmark adapter plus an authenticated, attempt-bound, idempotent bounce/complaint callback with complaint suppression, admin delivery guidance, secret-safe retention, and aggregate alerts; approved live-sandbox evidence is still pending. User mailbox email has real SMTP/IMAP plus Gmail and Microsoft Graph inbound and outbound OAuth adapters, encrypted scope-aware tokens, serialized refresh, provider metrics, and exact credential-free HTTP contract tests; approved live-provider evidence is still pending. One-to-one open/click collection is off by default, requires a per-send sender authorization acknowledgement, expires after 90 days, hides then scrubs observations through an observable bounded lifecycle pass, and leaves old links usable without further collection; the signals remain approximate. Customer sends carry a bounded opaque RFC `Message-ID`; Gmail receipts preserve provider message/thread IDs; IMAP, Gmail raw MIME, and selected Microsoft internet headers retain reply references; and a sequence exits only for an exact header match or one unambiguous provider-thread match within the same tenant/contact/mailbox/time boundary. Customer-mail bounce/complaint feedback and approved provider deliverability evidence remain incomplete
- Calling, SMS, and calendar workflows retain development-only fake providers; their contact actions and booking management are hidden from the production journey and bundle until real providers and compliance controls exist
- Quote delivery/legal signature, marketing, general-purpose workflow, and report-builder foundations still need their real runtime or provider outcome; immutable finalized quote versions, current-data draft PDFs, manual proposal tracking, and the narrower deal task automation surface are executable. Calling, SMS, calendar/booking-link, marketing-email, and nurture-campaign management foundations remain development-only until their promised end-to-end jobs exist

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
- Backend routes span the core CRM plus billing, email, communication, quoting, lead-generation, workflow-definition, and report-definition foundations.
- Authentication uses Argon2id password hashing and an `HttpOnly` same-site session cookie backed by database session records.
- The API applies CSRF protection for state-changing requests, structured request logging, request IDs, security headers, and readiness checks.
- Mailbox sync, email sequences, and meeting/task reminders run through a durable PostgreSQL-backed job system with idempotent enqueueing, leased claims, retries, dead letters, and operator recovery. Sequence enrollment and provider attempts additionally require an admin-approved exact content revision; a writer safety pause defers queued work without spending attempts. Managed Stripe runtimes enforce shared per-tenant and per-sender sequence-send safety budgets before a provider claim; these are configurable operating caps rather than plan entitlements, and fake/self-hosted billing remains unrestricted. Sequence outcomes distinguish provider acceptance, exact header/provider-thread-qualified replies, completed cadences, suppression exits, and uncertain sends without claiming inbox delivery or bounce/complaint coverage.
- The codebase documents major architectural decisions in `docs/adr/`.

## Tech Stack

| Layer | Technology |
|---|---|
| Frontend | React 18, Vite, React Router, JavaScript, CSS |
| Backend | Go 1.26, `net/http`, `ServeMux`, `pgx/v5` |
| Database | PostgreSQL 16 |
| Auth | Argon2id password hashing, server-side sessions, cookie authentication |
| Local infra | Docker, Docker Compose, Make |
| Deployment | Docker Compose, GitHub Actions, GitHub Pages, SSH-based backend rollout |
| Testing | Vitest, Testing Library, Go `testing`, `httptest` |

## Local Development

Prerequisites:
- Go 1.26.5
- Node.js 24.x and its bundled npm
- Docker with Compose plugin

The frontend runtime is pinned in `.nvmrc` and `.node-version`.
Supported runtime lines, audit gates, and the minimal-dependency exception
process are defined in [`docs/dependency-policy.md`](docs/dependency-policy.md).

Quick start:

```bash
cp example.env .env
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
make test-backup-restore
make test-monitoring
```

The browser journey intentionally requires a disposable PostgreSQL database; it will not silently use the normal development database:

```bash
cd apps/web
OPEN_CRM_E2E_DATABASE_URL='postgres://open_crm:open_crm@127.0.0.1:5432/open_crm_e2e?sslmode=disable' npm run test:e2e
```

Manual verification commands used by CI:

```bash
cd apps/api && gofmt -l . && go vet ./... && go test ./...
cd apps/web && npm test && npm run lint && npm run build:checked
```

Local environment defaults come from `example.env`:

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
- Called by `.github/workflows/ci.yml` only after backend, frontend, real-PostgreSQL browser, and encrypted backup/restore jobs pass on `main`
- Rebuilds the Pages artifact, publishes an exact commit marker, deploys it, and verifies the HTTPS URL reports that commit

Backend:
- `.github/workflows/backend-deploy.yml`
- Called by `.github/workflows/ci.yml` only after backend, frontend, real-PostgreSQL browser, and encrypted backup/restore jobs pass on `main`
- Syncs the repo to a remote host over SSH
- Writes `.env.production` from the `DEPLOY_ENV` secret
- Runs `scripts/remote-deploy.sh` with the commit SHA, which builds an immutable API image, starts PostgreSQL, enforces expand/contract migration policy, runs migrations, and brings the API up with `docker-compose.deploy.yml`
- Requires readiness-based container health and the exact release identity to remain stable locally, automatically restores the previous image after failed readiness when schema-compatible, and then verifies the public `/healthz` and `/readyz` release header

Operational details already documented in the repo:
- health and readiness endpoints: `/healthz`, `/readyz`
- immutable deploy state, guarded manual rollback, and automatic recovery: `docs/operations-runbook.md`
- encrypted off-host backup, scheduling, restore-drill, and deliberate recovery procedures: `docs/operations-runbook.md`
- protected metrics, initial SLOs, alert rules, and incident triage: `docs/operations-runbook.md`

## Testing

Current automated checks in `.github/workflows/ci.yml`:
- backend `go mod tidy` verification
- `gofmt -l .`
- `go vet ./...`
- pinned `gosec` static-security analysis with rule-specific, explained suppressions only
- pinned `govulncheck` reachable-vulnerability scan
- exact shipped-Go and installed-npm license-policy validation plus a generated,
  freshness-checked [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)
- backend `go test ./...`
- full frontend dependency audit at high severity
- frontend `npm test`
- frontend `npm run lint`
- frontend `npm run build:checked` with entry/lazy/total/CSS raw+gzip budgets
- Chromium axe-core WCAG A/AA scans over critical public and authenticated surfaces, including password recovery, with structured per-surface failure evidence and a keyboard skip-link check
- Chromium pilot journey against a disposable PostgreSQL database, including idempotent workspace bootstrap, mandatory owner-email verification and trial start, invitation rotation with old-link rejection, explicit revocation, reactivation, final activation, later member deactivation/reactivation, user-controlled active-sign-in revocation, multi-device password recovery/session invalidation, required typed custom-field administration, dynamic mapped import and safe rollback, client/contact creation and reviewed core/custom-field duplicate merge, admin stage/probability configuration with existing-deal continuity and forecast verification, deal/task work, won close review and transactional client handoff/account summary, client-health triage, recurring client review tasks, reversible bulk client changes, teammate mention and followed-digest navigation, session persistence, and cross-tenant denial
- encrypted Restic snapshot, retention/integrity check, extraction, isolated PostgreSQL restore, forward migration, and plaintext-leak acceptance
- immutable release, expand-migration, manual rollback, failed-readiness recovery, and database-unavailable startup recovery acceptance
- protected bounded-cardinality operational metrics plus promtool-validated request/database/job/provider/public-abuse/backup alert rules
- representative multi-tenant PostgreSQL query-plan, concurrent-read latency, and bounded database-failure budgets

The backend and browser jobs each use a disposable PostgreSQL 16 service. The
backup job creates and removes its own disposable Compose stack and encrypted
repository. Browser failures retain a screenshot, video, trace, and HTML report
as a short-lived CI artifact.

## Licensing

Open CRM is distributed under the [`MIT license`](LICENSE). The generated
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) inventories the Go and npm
components shipped in supported application artifacts and preserves their
required license/notice texts. The API image contains both files, and the
hosted frontend publishes the third-party notice alongside the SPA.

## Project Status

Open CRM is in a convergence phase: the repository has substantial feature breadth, but the priority is now to complete and harden the critical lead-to-client journey rather than add categories.

Status rules:

- [`docs/capability-matrix.md`](docs/capability-matrix.md) is the canonical current-state view.
- [`docs/professionalization-roadmap.md`](docs/professionalization-roadmap.md) is the historical roadmap and implementation log.
- A schema, management UI, fake provider, or stored definition is labeled as a foundation until the user outcome, failure recovery, operations, and acceptance tests are complete.
- The convergence release targets a trustworthy pilot workflow and optional managed SaaS; AI, help desk, marketplace/custom objects, native mobile, real-time, and enterprise breadth are deferred.
