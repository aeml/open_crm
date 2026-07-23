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
- [`docs/list-endpoint-inventory.md`](docs/list-endpoint-inventory.md) — list cardinality, pagination, ordering, overflow, and scale decisions
- [`docs/project-convergence-goal.md`](docs/project-convergence-goal.md) — execution order and definition of done
- [`docs/sales-workflow.md`](docs/sales-workflow.md) — supported pilot sales path, semantics, boundaries, and feedback record

## Overview

Open CRM is a production-oriented modular monolith made up of:
- a React frontend in `apps/web`
- a Go API in `apps/api`
- a PostgreSQL database
- Docker-based local and deployment workflows
- GitHub Actions CI/CD for testing and rollout

The production-capable core covers verified, idempotent workspace signup, authentication, user roles and lifecycle, contacts, companies, deals, tasks, notes, record followers, teammate mentions, bounded activity history with visible continuation, admin-configurable pipelines with stable stages, explainable probability-weighted forecasting, snapshot-backed sales activity reporting, exact cohort-based pipeline conversion and velocity, an exact client-period activity report, tenant-safe saved table and grouped bar reports, a bounded shared grouped-bar report dashboard, scheduled saved-report CSV delivery, transactional won-deal client handoff and account context, traceable contact/client touchpoint history, stale follow-up queues, explainable derived client health, recurring client review/renewal tasks, bounded transactional deal follow-up playbooks, reversible admin review of captured spam with safe delayed-work cancellation, durable mapped CSV imports with progress and rollback, reversible bulk maintenance, reviewed contact/client duplicate merge, bounded revision-safe typed contact/client custom fields, explicit archived-record recovery, live explainable data-quality queues, bounded revision-safe personal saved views, bounded operational CSV exports, durable full-workspace portability exports, audit history, and organization-scoped access. The repository also contains broad post-MVP foundations for billing, mailbox sync, sequences, calling/SMS/calendar, quoting, lead generation, general-purpose workflows, and richer analytics. Those foundations have different maturity levels and are not all complete product outcomes; the capability matrix is authoritative. Incomplete custom line/funnel/pie/KPI, personal-dashboard, and external-sharing options are hidden from production navigation, while dedicated audience/scoring, booking, and marketing management routes are also excluded from production bundles.

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
- Contacts, companies, deals, tasks, notes, bounded activity/notes continuation, ownership, filters, pagination, and exact-revision personal saved views with a 100-view per-user/entity ceiling and legacy-safe complete loading; company detail keeps a 50-person first page with exact totals, explicit name/email/relationship search and continuation, safe existing-contact link/primary/unlink controls, atomic sole-person replacement for individual clients, and generic edits that cannot replace unseen relationships
- Admin-managed pipelines with bounded stage creation, renaming, outcome classification, configurable open-stage probability, exact reordering, default selection, and stable stage identities for existing deals
- Explainable period forecasts with unweighted, won, stage-weighted, owner, unassigned, and stage-assumption rollups plus close-date filter parity across deal lists, saved views, and CSV exports
- Snapshot-backed sales activity reports with bounded UTC date/teammate filters, exact deal and follow-up counts, outcome win rate, event-based stage movement, fixed won/lost reason breakdowns with close-note context, honest history coverage, and deal drill-down
- An exact pipeline cohort report with a fixed entry stage, creation-owner and cohort-date filters, a separate as-of date, current outcome counts, per-stage reach/current/exit math, forward-or-won exits, median elapsed-day velocity, explicit maturity/coverage semantics, a five-second deadline, and a 500-deal performance gate
- Saved contact, company, deal, and task table reports plus grouped bar reports with typed filters, one category and a count/sum/average, tenant and archived-row isolation, bounded pagination/query time, an exact accessible data table, and owner/admin audited formula-safe CSV downloads with an explicit 10,000-row ceiling. Writers can publish up to six active grouped bars to one revisioned shared workspace dashboard; all members see the same five-second, 12-groups-per-widget snapshot. Owners/admins can also schedule one daily or weekly UTC CSV email per saved report to up to ten active workspace members, with exact artifacts, durable per-recipient evidence, explicit ambiguous-send recovery, seven-day artifact retention, metrics, alerts, portable evidence, and real-PostgreSQL/Chromium acceptance. Custom line/funnel/pie/KPI, personal dashboards, external sharing, and PDF scheduling remain incomplete
- Transactional won-deal handoff that requires an account relationship, promotes the explicit organization—or a company-less primary contact—into the existing client model, records activity/audit evidence, and links to a one-page rollup of won deals, open account tasks, recent notes, and key people
- Viewer-aware contact/client touchpoint summaries and stale follow-up queues with explicit source, creation-fallback, linked-person, and privacy semantics
- Derived Healthy, Watch, and Needs attention client signals with exact stale-follow-up and open-task reasons, reusable scoped customer segments, organization/individual and health filters, company-linked-person rollup, and no fabricated issue claim
- Adaptive service profiles that present the mature pipeline record as a Job and linked delivery tasks as Service/Site Tasks without advertising a separate project module
- Task-backed client review and renewal schedules with assigned ownership, one-time or recurring cadence, durable reminders, late-period skip semantics, dashboard visibility, and recovery-safe client/task guards
- Tenant-scoped deal line items and totals, immutable versioned quote PDFs with effective-dated workspace-base-currency disclosure, explicit active/expired/replaced lifecycle and immutable reissue, durable connected-mailbox delivery with uncertain-send recovery, expiring customer access, approximate view/download evidence, explicit non-acceptance receipt, an optional quote-bound typed-name consent ceremony with idempotent recipient decisions, staff void controls, retained audit certificates, and deliberate certificate-bound conversion into the selected won stage, close review, automation, and client handoff
- Admin-managed automations that atomically create an ordered 1–5-task deal playbook with independent 0–365-day due offsets, optionally pause for one owner/admin/current-owner decision, or append one bounded same-transaction in-app teammate notification; lead forms can instead snapshot and durably schedule one assigned follow-up task. Versioned contracts keep unreviewed definitions inert. Every supported action retains tenant-bound lifecycle/result evidence, and exact causal parent, ancestor re-entry, and depth guards protect future trigger-capable actions
- Exact overdue and rolling-24-hour task surfaces with preference-aware assignment events and durable, replay-safe in-app reminders
- Record following, explicit teammate mentions, relevant notification links, and followed/team activity digests
- Searchable, independently paged email-template and snippet catalogs with exact revisions, transactional lifecycle audit, serialized 100-definition boundaries, complete legacy-safe composer selection, dynamic tenant custom-field merge tokens, exact preview, and private test-to-self delivery
- Dry-run CSV mapping followed by an idempotent PostgreSQL-backed import job with visible progress, bounded retries, seven-day recovery source, row error downloads, and safe rollback; bounded bulk archive/reassignment plus contact/client status and task-completion changes with safe rollback; field-resolved contact/client duplicate merge; and admin-defined typed contact/client fields carried through forms, filters, saved views, imports, exports, and duplicate review
- Searchable contact/client/deal/task archive history with role-aware restore, dependency safeguards, permanent merge-source protection, retained relationships, and transactional activity/audit evidence
- Live data-quality queues for missing ownership/contact details, stale or incomplete deals, unscheduled tasks, and business-profile-specific client/account gaps, with explainable counts and direct cleanup links
- Tenant-scoped CSV exports for all four core record types, with custom columns, exact visible-filter parity, a tested 10,000-row synchronous ceiling, explicit refusal instead of silent truncation, and a separate durable 50,000-row/50 MiB path with progress, replay, checksums, and seven-day downloads
- Owner/admin full-workspace ZIP exports generated by the durable job queue from one repeatable-read snapshot, with explicit dataset coverage, secret/private-mail redaction, integrity checksum, seven-day expiry, request/ready/download audit, and availability during hosted suspension
- Append-only workspace-lifetime audit events with admin review, bounded CSV and complete portable export; role and access changes revalidate the acting owner/admin inside the mutation transaction, commit current state and history together, and leave no unaudited access change after an audit failure or stale session
- Responsive React UI with route-level tests and consistent loading/error/permission states

In convergence:

- Lead forms, hosted pages, and website widgets map standard or active typed contact custom fields, require complete required-field coverage before activation, and bind every public challenge/submission to the exact form revision. Form, landing-page, and website-widget management now uses exact 50/default and 100/maximum status pages with stable continuation; their transactionally authorized writes require exact revisions and value-free audit. Mapped definition changes invalidate outstanding challenges; typed contact values, consent, a value-free mapping snapshot, activity, and submission commit together. The local PostgreSQL and Chromium journey covers typed capture, review, recovery, and assignment, while edge/WAF reputation, conversion analytics, custom domains/SEO, automatic scoring/routing, and pilot validation keep the overall lead-generation surface at foundation maturity
- Billing retains a fake provider for self-hosted/development plan switching, but only a runtime configured with Stripe is managed: self-hosted sessions report `unmanaged`, usage is shown without hosted ceilings, and stale trial/subscription fields cannot restrict private writes, workers, or public lead capture. Stripe mode has backend-hosted Checkout and customer-portal sessions, raw-body signed/idempotent webhooks, authoritative subscription/trial/dunning/cancellation state, an invoice ledger, six-hour provider reconciliation through the durable job queue, admin replay, provider metrics, centralized tenant-mutation suspension, attempt-preserving worker deferral, and a suspension-safe portable workspace export. Plan & Billing gives owners/admins suspension-safe, newest-first invoice history with amounts, provider retry evidence, and sanitized HTTPS invoice/PDF links; Stripe remains authoritative for retry timing. The page and an exempt once-daily durable worker reconcile the same explainable period snapshot of active seats/records, sent outbound email, successful automation/background jobs, and estimated tenant PostgreSQL row bytes; they use an exact Stripe period when available and a UTC month otherwise, retry bounded serialization conflicts, and clearly avoid unapproved new quotas. The hosted catalog currently exposes only the subscription and seat/contact/deal capacity contract it enforces: unapproved feature arrays are empty, unfinished API/SSO/automation/reporting outcomes are not advertised as plan benefits, and Stripe Checkout—not a hard-coded UI hint—confirms the actual recurring price. Those three capacities use durable tenant-scoped claims that are consumed atomically with direct creates, user reactivation, imports, public lead capture, and single/bulk archive recovery; concurrent requests see either the claim or committed record, and abandoned claims expire. Managed authentication returns a server-derived access snapshot; a persistent read-only/unavailable banner and shared role-aware controls remove normal mutation actions while reads, exports, self-account maintenance, job recovery, and billing recovery remain available. Managed public lead writes fail closed without exposing tenant billing state. CI connects the production adapter and app routes to a credential-free Stripe HTTP contract sandbox and real PostgreSQL for Checkout, signed-event, reconciliation, tenant invoice visibility, dunning, recovery, cancellation, and quota-concurrency acceptance. It remains a foundation until an approved credentialed Stripe test-mode deployment smoke, explicit portal/proration/resubscription and feature-tier policy, any approved message/automation/job/storage quotas, dunning timing, and tenant deletion/retention policy are complete
- System identity email has a real Postmark adapter plus an authenticated, attempt-bound, idempotent bounce/complaint callback with complaint suppression, admin delivery guidance, secret-safe retention, and aggregate alerts; approved live-sandbox evidence is still pending. User mailbox email has real SMTP/IMAP plus Gmail and Microsoft Graph inbound and outbound OAuth adapters, encrypted scope-aware tokens, serialized refresh, provider metrics, and exact credential-free HTTP contract tests; approved live-provider evidence is still pending. Contact/company/deal composers render active tenant custom fields, show the exact server-merged recipient/subject/body, fail visibly on unknown tokens, and can send a durable private test only to the acting user's revalidated sign-in address with tracking, suppression, unsubscribe, and customer timeline effects forced off. Customer sends persist an exact actor-scoped intent before the provider, claim once, recheck current recipient/sender/suppression, atomically finalize CRM evidence, and never auto-retry ambiguity; interrupted customer or test sends remain visible for explicit Sent-folder recovery, deactivation quiesces them, aggregate alerts cover recovery, and portable exports remove internal correlation. One-to-one open/click collection is off by default, requires a per-send sender authorization acknowledgement, expires after 90 days, hides then scrubs observations through an observable bounded lifecycle pass, and leaves old links usable without further collection; the signals remain approximate. Customer sends carry a bounded opaque RFC `Message-ID`; Gmail receipts preserve provider message/thread IDs; IMAP, Gmail raw MIME, and selected Microsoft internet headers retain reply references; and a sequence exits only for an exact header match or one unambiguous provider-thread match within the same tenant/contact/mailbox/time boundary. Customer-mail bounce/complaint feedback and approved provider deliverability evidence remain incomplete
- Email sequences are production-capable locally: searchable/status-filtered 50-row definition pages, a serialized 100-active ceiling, exact-revision transactional authoring/approval/audit, complete drift-detecting active-only selection, contact enrollment, durable delivery, suppression, conservative reply/feedback correlation, provisional hosted send caps, outcome analytics, bounded tenant-safe enrollment drill-down, and ambiguity recovery are implemented. Fresh-PostgreSQL CI proves 1,001-definition paging, tenant/role/revision/audit/final-slot behavior and drives the visible row-51/search/draft-to-finished journey through the real SMTP adapter with a local provider sandbox, captured MIME assertions, individual enrollment reconciliation, active-option exclusion, and WCAG scanning. Approved Gmail/Microsoft/SMTP evidence, downstream DKIM/deliverability behavior, the 100-active operating limit, and pilot validation remain before calling the outcome provider-validated.
- Calling, SMS, and calendar workflows retain development-only fake providers; their contact actions and booking management are hidden from the production journey and bundle until real providers and compliance controls exist
- Quote preparation now includes admin-managed revisioned terms/delivery templates, optional template- or workspace-required independent approval by a different active owner/admin, exact PDF-digest decision evidence, delivery gating, notifications, portable export, aggregate alerts, and the real-PostgreSQL browser path through approval and signing. Template administration has searchable active/archived 50-row pages with exact totals and a concurrency-safe 100-active ceiling; deal preparation loads only the complete active set and preserves access to legacy overflow. Accounting policy, jurisdiction/consent review, approved live-mailbox evidence, and pilot validation remain. The native immutable quote signing, certificate, and deliberate signed-close handoff paths are still labeled foundation until those external outcomes pass. Workflow execution now has bounded production outcomes: transactional 1–5-task deal-event playbooks with an optional retained approval or final teammate notification, one conditional lead-form task with a durable whole-day creation schedule, and one exact transactional deal-owner assignment on creation, stage change, or direct owner change. Owner assignment preserves ordinary notifications, emits a causally linked owner-change event, records changed or explicit no-op evidence, and is protected by ancestor, depth, and causal-tree guards. General field mutation, provider actions, arbitrary scheduling, and branching remain hidden foundations. Fixed pipeline cohort/velocity reporting, saved table/grouped-bar reports, one shared six-widget grouped-bar dashboard, and bounded scheduled CSV delivery now span Reports, Dashboard, and the durable worker; custom line/funnel/pie/KPI charts, personal dashboards, external sharing, and PDF scheduling remain incomplete. Audience/scoring, calling, SMS, calendar/booking-link, marketing-email, and nurture-campaign management foundations remain development-only until their promised end-to-end jobs exist

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
- Backend routes span the core CRM plus billing, email, communication, quoting, lead-generation, workflow-definition, saved-report execution, and richer report-definition foundations.
- Authentication uses Argon2id password hashing and an `HttpOnly` same-site session cookie backed by database session records.
- The API applies CSRF protection for state-changing requests, structured request logging, request IDs, security headers, and readiness checks.
- Mailbox sync, email sequences, and meeting/task reminders run through a durable PostgreSQL-backed job system with idempotent enqueueing, leased claims, retries, dead letters, and operator recovery. Sequence definition writes revalidate the active actor transactionally, bind reviewed mutations to an exact revision, and cap new activations at 100 per workspace; enrollment and provider attempts additionally require an admin-approved exact content revision, while a writer safety pause defers queued work without spending attempts. Managed Stripe runtimes enforce shared per-tenant and per-sender sequence-send safety budgets before a provider claim; these are configurable operating caps rather than plan entitlements, and fake/self-hosted billing remains unrestricted. Sequence outcomes distinguish provider acceptance, exact header/provider-thread-qualified replies, completed cadences, suppression exits, and uncertain sends without claiming inbox delivery or bounce/complaint coverage.
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

The hosted-billing acceptance requires a separate empty database. It starts a
local Stripe-shaped HTTP sandbox, but drives the production Stripe adapter and
the real browser/API boundaries; no provider credential or payment is used:

```bash
OPEN_CRM_E2E_DATABASE_URL='postgres://open_crm:open_crm@127.0.0.1:5432/open_crm_e2e_hosted?sslmode=disable' \
  OPEN_CRM_E2E_BILLING_PROVIDER=stripe npm run test:e2e:hosted
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
- Runs `scripts/remote-deploy.sh` with the commit SHA, which builds an immutable API image, enforces expand/contract migration policy, and—when PostgreSQL already exists—proves the new database credential and migrations with `--no-deps` before Compose may recreate PostgreSQL or the API
- Requires readiness-based container health and the exact release identity to remain stable locally, automatically restores the previous image after failed readiness when schema-compatible, preserves that rollback pointer on an exact-SHA redeploy, and then verifies the public `/healthz` and `/readyz` release header
- After acceptance, keeps five exact revision-labeled API images by default while always protecting the current/previous pair; mismatched, malformed, legacy, and operator-managed tags are never guessed at or force-removed

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
- Chromium pilot journey against a disposable PostgreSQL database, including idempotent workspace bootstrap, mandatory owner-email verification and trial start, invitation rotation with old-link rejection, explicit revocation, reactivation, final activation, later user deactivation/reactivation and promotion to an independent admin approver, user-controlled active-sign-in revocation, multi-device password recovery/session invalidation, required typed custom-field administration, dynamic mapped import through the durable worker and safe rollback, client/contact creation and reviewed core/custom-field duplicate merge, admin stage/probability configuration with existing-deal continuity and forecast verification, deal/task work, revisioned quote-template configuration, workspace approval policy, blocked pre-approval delivery, independent exact-PDF approval, immutable quote delivery through a real SMTP sandbox, public consent signing, matching certificate evidence, accessible staff-controlled signed-quote conversion, won close review and transactional client handoff/account summary, exact pipeline-cohort conversion/velocity reconciliation with WCAG and foreign-ID rejection, client-health triage, recurring client review tasks, exact client-period activity/source reconciliation with WCAG and foreign-collection exclusion, reversible bulk client changes, teammate mention and followed-digest navigation, session persistence, and cross-tenant denial
- isolated Stripe-mode Chromium journey against a second disposable PostgreSQL database, covering verified trial start, server-created Checkout and Portal destinations, webhook-only activation, exact duplicate signed events, past-due grace, unpaid suspension and direct-write denial, payment recovery, scheduled/final cancellation, suspension-safe invoice visibility and portable export, plus WCAG scans at trial, suspension, and cancellation
- encrypted Restic snapshot, retention/integrity check, extraction, isolated PostgreSQL restore, forward migration, and plaintext-leak acceptance
- immutable release, expand-migration, pre-restart database-credential drift refusal, bounded rollback-safe image retention, exact-SHA rollback-pointer preservation, manual rollback, failed-readiness recovery, and database-unavailable startup recovery acceptance
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
