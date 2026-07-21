# Open CRM Roadmap

> **Status note (2026-07-19):** This file is the historical version plan and implementation log. The canonical current maturity of exposed capabilities is maintained in [`capability-matrix.md`](capability-matrix.md), using stricter outcome-based states. A “foundation complete” note below does not mean the full product outcome or version exit criteria are complete. Current execution order and completion rules live in [`project-convergence-goal.md`](project-convergence-goal.md).

This roadmap has two parts.

**Part I (`0.1.1` → `0.10.0`)** moves the project from MVP-complete to a professional-grade, production-ready CRM foundation: a small, explicit CRM built from a Go API, React web app, and Postgres. Each version is a shippable slice that improves safety, reliability, maintainability, and operator trust without introducing unnecessary platform complexity. This part is largely complete or well-specified and should be finished first — it is the load-bearing foundation everything else sits on.

**Part II (`1.0.0` → `2.0.0`)** is a deliberate strategic expansion: turn Open CRM into a **full-featured, multi-tenant SaaS product that competes with HubSpot, Pipedrive, Zoho, Salesforce, Close, and Copper.** This part intentionally reverses several "non-goals" declared in `mvp.md` (email sync, calendar sync, marketing automation, workflow engine, mobile, real-time). Competing in the CRM SaaS market requires those capabilities plus the platform machinery to sell, meter, and operate software as a service. See `## Strategic Direction Change` and `## Competitive Gap Analysis` below before starting Part II work.

The "minimal dependencies, every dependency earns its existence" principle from `mvp.md` still holds in Part II — but the bar shifts from "do we need a library" to "do we need a capability to be competitive," and several capabilities (background workers, an email/comms layer, billing, AI providers) now clearly clear that bar.

After `0.3.0`, the baseline infrastructure work is complete. Part I versions are driven by product usefulness, operator workflows, and measured reliability. Part II versions are driven by competitive parity and SaaS go-to-market requirements.

## Strategic Direction Change

`mvp.md` deliberately scoped Open CRM as a small, self-hosted, single-tenant-feeling CRM and listed these as explicit non-goals: marketing automation, email sync, calendar sync, custom workflow engine, mobile app, event bus/queue, microservices, public API versioning guarantees, multi-region, and real-time collaboration.

Part II reclassifies these from "non-goals" to "competitive requirements," with the following stance:

- **Still a modular monolith first.** We add background workers and an outbound integration layer, but we do not jump to microservices. We scale the monolith and extract only when a capability (email ingestion, AI processing) genuinely needs an independent runtime.
- **Multi-tenant SaaS, not just multi-org.** Self-serve signup, billing, plan-gated features, usage metering, and tenant isolation at scale become first-class.
- **Buy vs. build for commodity infrastructure.** Email/SMS delivery (e.g. Postmark/SendGrid/Twilio), payments (Stripe), and AI inference (hosted LLM APIs) are bought, not built. CRM workflow value is built.
- **Open-core friendly.** Where practical, keep the core CRM open and gate advanced/enterprise capabilities behind plan tiers so the project can sustain a SaaS business and a self-hosted community edition.

## Competitive Gap Analysis

What Open CRM has today (through `0.4.x`) vs. what table-stakes CRM SaaS products ship. "Built" = exists now; "Part I" = covered by the existing `0.5`–`0.10` plan; "Part II" = new competitive work below.

| Capability area | Competitors (HubSpot/Pipedrive/Zoho/Salesforce) | Open CRM status |
| --- | --- | --- |
| Contacts, companies, deals, tasks, notes, activity | Yes | Built |
| Multi-user, roles, ownership, assignment | Yes | Built |
| Saved views, filters, import/export CSV | Yes | Built |
| Audit trail, notifications (in-app) | Yes | Built |
| Bulk actions, duplicate merge, custom fields | Yes | Part I (`0.5.x`) |
| Configurable pipelines, forecasting, win/loss | Yes | Part I (`0.6.x`) |
| Customer/account post-sale views | Yes | Part I (`0.7.x`) |
| Public API, API tokens, webhooks | Yes | Part I (`0.8.x`) |
| Background jobs, scale, pagination, backups | Yes | Part I (`0.9.x`) |
| **Self-serve signup + billing + plan tiers** | Yes | **Part II (`1.0`)** |
| **SSO / SAML / SCIM, granular RBAC, i18n, multi-currency** | Yes | **Part II (`1.0`, `2.0`)** |
| **2-way email sync, tracking, templates, sequences** | Yes | **Part II (`1.1`)** |
| **Telephony, SMS, call recording, meeting scheduler** | Yes | **Part II (`1.2`)** |
| **Product catalog, quotes/CPQ, e-signature, quotas** | Yes | **Part II (`1.3`)** |
| **Forms, landing pages, campaigns, lead scoring** | Yes | **Part II (`1.4`)** |
| **Visual workflow/automation engine** | Yes | **Part II (`1.5`)** |
| **Custom report builder + dashboards** | Yes | **Part II (`1.6`)** |
| **AI: drafting, summarization, scoring, copilot, enrichment** | Yes (now a key battleground) | **Part II (`1.7`)** |
| **Help desk / tickets / customer portal / knowledge base** | Yes | **Part II (`1.8`)** |
| **Integration marketplace + custom objects** | Yes | **Part II (`1.9`)** |
| **Mobile apps + real-time collaboration** | Yes | **Part II (`1.10`)** |
| **Enterprise: SSO/SCIM, sandboxes, data residency, audit/compliance** | Yes | **Part II (`2.0`)** |

## Progress

### Part I — Professional Foundation

- `0.1.1` Migration Safety: complete.
- `0.1.2` HTTP Runtime Hardening: complete.
- `0.1.3` CI Quality Gates: complete.
- `0.1.4` Tooling Reproducibility: complete.
- `0.1.5` Frontend API Client Consolidation: complete.
- `0.1.6` Request Validation And Body Limits: complete.
- `0.1.7` Error Semantics: complete.
- `0.1.8` Security Baseline: complete.
- `0.1.9` User Lifecycle: complete.
- `0.2.0` Observability And Operations: complete.
- `0.2.1` Backend Maintainability: complete.
- `0.2.2` Frontend Maintainability: in progress (reopened for convergence hotspot work).
- `0.2.3` Database Integrity: complete.
- `0.3.0` Professional Release Candidate: complete.
- `0.3.1` First-Use Product Polish: complete.
- `0.3.2` Saved Views And Filters: complete.
- `0.3.3` Import Foundation: complete.
- `0.3.4` Activity Timeline Improvements: complete.
- `0.3.5` Dashboard Decision Support: complete.
- `0.3.6` Admin Audit Trail: complete.
- `0.3.7` Data Export: complete.
- `0.3.7a` Architecture Decision Records Seeding: complete.
- `0.3.7b` Responsive And Mobile Pass: complete.
- `0.3.7c` Error Boundaries And Session UX: complete.
- `0.3.8` Accessibility And Keyboard Pass: complete.
- `0.3.8a` Tenant Isolation Hardening: complete.
- `0.3.8b` Dependency Hygiene: complete.
- `0.3.9` Release Readiness Review: complete.
- `0.4.0` Multi-User Team CRM: complete.
- `0.4.1` User Profile And Preferences: complete.
- `0.4.2` Team Assignment Views: complete.
- `0.4.3` Role Permissions Pass: complete.
- `0.4.4` Notification Preferences: complete.
- `0.4.5` Mention And Follow Model: complete.
- `0.4.6` Team Activity Digest: complete.
- `0.4.7` Admin User Lifecycle Hardening: complete.
- `0.4.8` Team Usage Reporting: planned.
- `0.4.9` Team Release Review: planned.
- `0.5.0` CRM Data Operations: complete.
- `0.5.1` Bulk Actions: complete.
- `0.5.2` Duplicate Management: complete.
- `0.5.3` Import Mapping UI: complete.
- `0.5.4` Import Validation And Rollback: complete.
- `0.5.5` Custom Fields Foundation: complete.
- `0.5.6` Custom Field Filtering: complete.
- `0.5.7` Data Quality Reports: complete.
- `0.5.8` Archive And Retention Controls: complete.
- `0.5.9` Data Operations Review: complete.
- `0.6.0` Sales Workflow Maturity: in progress.
- `0.6.1` Pipeline Configuration: complete.
- `0.6.2` Deal Probability And Forecasting: complete.
- `0.6.3` Task Automation Rules: complete.
- `0.6.4` Reminder Workflow: complete.
- `0.6.5` Sales Activity Reporting: complete.
- `0.6.6` Contact Touchpoint Tracking: complete.
- `0.6.7` Quote Or Proposal Placeholder Flow: complete.
- `0.6.8` Win Loss Review: complete.
- `0.6.9` Sales Workflow Review: in progress (technical review complete; approved pilot usage evidence pending).
- `0.7.0` Customer Operations: in progress (technical outcome complete; approved pilot usage evidence pending).
- `0.7.1` Post-Sale Account View: complete.
- `0.7.2` Client Health Signals: complete.
- `0.7.3` Renewal And Follow-Up Tasks: complete.
- `0.7.4` Service Or Job Tracking: complete.
- `0.7.5` Account Notes And Internal Handoff: complete.
- `0.7.6` Customer Segment Views: complete.
- `0.7.7` Customer Activity Reports: planned (deferred to Phase 4 reporting convergence).
- `0.7.8` Customer Data Review: complete.
- `0.7.9` Customer Operations Review: in progress (technical review complete; approved pilot usage evidence pending).
- `0.8.0` Integrations Foundation: planned.
- `0.8.1` Public API Shape: planned.
- `0.8.2` API Token Management: planned.
- `0.8.3` Webhook Delivery Model: planned.
- `0.8.4` Email Link Capture: planned.
- `0.8.5` Calendar Link Planning: planned.
- `0.8.6` Integration Event Log: planned.
- `0.8.7` Import API Endpoint: planned.
- `0.8.8` Integration Security Review: planned.
- `0.8.9` Integration Release Review: planned.
- `0.9.0` Scale And Reliability: in progress.
- `0.9.1` Query Performance Review: in progress (core tenant query plans and representative budgets are CI-gated; dashboard/report/import/provider review remains).
- `0.9.2` Pagination And Large Dataset Hardening: in progress (core paginated reads and the bounded 10,000-row export plus explicit overflow refusal are tested; full endpoint/pagination-boundary review remains).
- `0.9.3` Background Job Runner: complete.
- `0.9.4` Async Import And Export Jobs: planned.
- `0.9.5` Backup Automation: complete (production repository credentials and timer activation remain an operator deployment step).
- `0.9.6` Restore Drill Automation: complete (real off-host validation remains required for pilot evidence).
- `0.9.7` Monitoring And Alerting Hooks: in progress (implementation complete; production scrape/destination validation pending).
- `0.9.8` Load And Failure Testing: in progress (read/write/query/database-failure/export/import/provider/bundle budgets complete; production-like host evidence remains).
- `0.9.9` Reliability Release Review: in progress (immutable deploy recovery and migration compatibility complete; load/failure review remains).
- `0.10.0` Production Beta: in progress (license inventory/notice gate complete; approved pilot readiness evidence remains).

### Part II — Competitive SaaS Platform

- `1.0.0` Multi-Tenant SaaS Platform (signup, billing, plan gating, SSO): in progress.
- `1.1.0` Email And Communications (2-way sync, tracking, templates, sequences): in progress.
- `1.2.0` Telephony, SMS, And Meeting Scheduling: in progress (development foundations retained; fake-only contact and booking actions are hidden and bundle-guarded in production until real provider/compliance outcomes exist).
- `1.3.0` Sales Acceleration And CPQ (catalog, quotes, e-sign, quotas): in progress.
- `1.4.0` Marketing And Lead Generation (forms, pages, campaigns, scoring): in progress.
- `1.5.0` Workflow Automation Engine (visual builder): in progress.
- `1.6.0` Reporting And Analytics (custom report builder, dashboards): in progress.
- `1.7.0` AI And Intelligence (copilot, drafting, summarization, scoring, enrichment): planned.
- `1.8.0` Service, Support, And Customer Portal (tickets, SLAs, knowledge base): planned.
- `1.9.0` Ecosystem And Extensibility (integration marketplace, custom objects): planned.
- `1.10.0` Mobile And Real-Time Collaboration: planned.
- `2.0.0` Enterprise And General Availability: planned.

# Part I — Professional Foundation

## Version 0.1.1 - Migration Safety

Status: complete.

Goal: make database deploys safer and repeatable.

- Add a `schema_migrations` table.
- Record applied migration names and timestamps.
- Skip already-applied migrations on future deploys.
- Keep migration execution explicit and easy to debug.
- Update migration tests to cover tracked execution.

Exit criteria:

- Running migrations twice does not reapply completed migrations.
- Backend tests pass with `go test ./...`.

## Version 0.1.2 - HTTP Runtime Hardening

Status: complete.

Goal: make the API process safer under production traffic and deploy restarts.

- Add `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, and `IdleTimeout` to the API server.
- Add graceful shutdown on `SIGINT` and `SIGTERM`.
- Add tests or small seams around server configuration where practical.
- Document production runtime behavior.

Exit criteria:

- API shuts down cleanly during deploys.
- Server has bounded request lifecycle defaults.

## Version 0.1.3 - CI Quality Gates

Status: complete.

Goal: separate quality validation from deployment.

- Add a general CI workflow for pushes and pull requests.
- Run `go test ./...` and `go vet ./...`.
- Run `npm ci`, `npm test`, and `npm run build` for the frontend.
- Add a formatting check for Go files.
- Keep deploy workflows focused on deployment.

Exit criteria:

- Pull requests fail before deploy if backend or frontend quality gates fail.
- CI uses the same Node version documented for local development.

## Version 0.1.4 - Tooling Reproducibility

Status: complete.

Goal: make local setup predictable for future contributors and deployment debugging.

- Add `.nvmrc` and optional `.node-version` for the supported Node runtime (currently Node 24).
- Pin supported Go and container base-image lines in CI and production builds.
- Add package manager metadata where useful.
- Update README local development instructions.
- Confirm frontend tests and builds run from a clean checkout.

Exit criteria:

- A new contributor can install and test with the documented runtime versions.
- Local frontend verification no longer depends on an accidental global Node/npm version.

Current convergence evidence: local/frontend automation uses Node 24. CI,
`go.mod`, and container builds pin Go 1.26.5 after a reachable-vulnerability
scan identified the fixed standard-library patch requirement; the production
image uses a supported Alpine 3.24 patch image. Container bases are
digest-pinned and tracked by Dependabot. The upgraded runtime passed vet, every
serialized real-PostgreSQL package, Chromium acceptance, encrypted
backup/restore, and immutable deploy-recovery drills.

## Version 0.1.5 - Frontend API Client Consolidation

Status: complete.

Goal: reduce duplicated fetch behavior and make errors consistent.

- Create one shared frontend API request helper.
- Centralize `credentials: 'include'`, JSON parsing, error message extraction, and 204 handling.
- Preserve feature-specific helpers in `src/lib/*`, but make them call the shared client.
- Add tests for error parsing and empty responses.

Exit criteria:

- Feature API modules no longer duplicate low-level fetch boilerplate.
- Auth/session errors can be handled consistently in one place later.

## Version 0.1.6 - Request Validation And Body Limits

Status: complete.

Goal: make backend request handling more robust.

- Add bounded JSON body reads with `http.MaxBytesReader`.
- Add a shared JSON decode helper.
- Consider `DisallowUnknownFields` for API write payloads after checking current frontend behavior.
- Standardize bad-request responses.

Exit criteria:

- Oversized request bodies are rejected predictably.
- JSON decode behavior is consistent across handlers.

## Version 0.1.7 - Error Semantics

Status: complete.

Goal: make API behavior more professional and predictable.

- Introduce module-level `ErrNotFound` values where needed.
- Map not-found errors to `404` instead of generic `500` responses.
- Standardize validation and conflict errors across modules.
- Add handler tests for not-found paths.

Exit criteria:

- Missing resources return consistent `404` responses.
- Client behavior can rely on stable error codes.

## Version 0.1.8 - Security Baseline

Status: complete.

Goal: address the highest-value security gaps for cookie-auth CRM usage.

- Add CSRF protection for state-changing requests or document a deliberate alternative.
- Add login/bootstrap rate limiting.
- Remove unused `SESSION_COOKIE_SECRET` or wire it to a real purpose.
- Add basic security headers at the API or documented edge layer.
- Review session token storage and session cleanup.

Exit criteria:

- Cookie-auth write requests have CSRF mitigation.
- Auth endpoints have abuse protection.
- Configuration no longer contains unused security settings.

Current convergence evidence: CI now pins `gosec` v2.28.0 and rejects every
unsuppressed backend finding. The initial 175-file scan replaced predictable
`math/rand` request IDs with cryptographic identifiers, bounds accepted Argon2
parameters and decoded lengths before hashing, confines backup evidence reads
to an `os.Root`, reads generated export archives back through their already
bounded temporary file handle, and handles import close outcomes explicitly.
Remaining false positives have rule-specific inline explanations for public
OAuth/template metadata, SQL column names, bounded multipart parsing,
environment-derived secure cookies, the explicit local seed credential, and
the validated HTTP(S) click-tracking redirect; an executable repository scan
enforces the dependency policy's prohibition on bare or unexplained
suppressions.

## Version 0.1.9 - User Lifecycle

Status: complete.

Goal: replace temporary-password behavior with a safer onboarding workflow.

- Replace hardcoded temporary passwords with invite or password setup tokens.
- Add token expiry and one-time-use semantics.
- Add user-facing setup/reset flow in the frontend.
- Add tests for invite creation and consumption.

Exit criteria:

- New users are not created with a shared known password.
- Admin-created users can securely activate accounts.

Current convergence evidence: invitations use digest-only, one-time setup tokens
with a seven-day expiry and production-hidden raw links. The admin Users surface
shows pending, expired, accepted, and revoked states and supports token-rotating
resend plus explicit, safely repeatable revocation. Revocation runs through the
same access-removal transaction as member deactivation, including session
invalidation, work reassignment, effect quiescence, and audit evidence; a
reactivated invitation still requires a newly delivered link. Handler/UI tests,
freshly migrated PostgreSQL acceptance, and the clean browser journey cover
cross-tenant denial, expiry, old-link rejection, revocation, reactivation, and
final activation. Migration `085_postmark_delivery_feedback.sql` and the
authenticated Postmark callback now bind bounce/complaint events to separate
attempt keys, preserve a secret-safe idempotent ledger, surface admin recovery,
and suppress future system mail after complaints. Approved live-Postmark
delivery and feedback remain external validation work.

## Version 0.2.0 - Observability And Operations

Status: complete.

Goal: make production behavior easier to understand and support.

- Add structured request logging with method, path, status, duration, and request ID.
- Add API container health checks in deploy compose.
- Document backup and restore procedures for Postgres.
- Add a deploy/runbook document for common operational tasks.

Exit criteria:

- Production issues can be traced by request ID.
- Operators have documented backup, restore, and deploy recovery steps.

## Version 0.2.1 - Backend Maintainability

Status: complete.

Goal: keep the modular monolith explicit while reducing oversized files.

- Split `internal/app/app.go` by handler area.
- Keep route registration centralized and easy to scan.
- Move repeated response and decode helpers into focused shared helpers or small platform/web utilities.
- Avoid new framework dependencies unless they clearly remove more complexity than they add.

Exit criteria:

- Handler files are easier to review in isolation.
- No behavior changes beyond tested refactors.

Current convergence evidence: `app.go` is now 380 lines and uses the default
500-line CI ceiling. All 213 explicit registrations live in focused 172-line
platform, 246-line foundation, and 267-line core-CRM files, called centrally by
`NewServer`; package-wide inventory and hosted-write-policy scans preserve the
complete route set after the split. HTTP rate limiting, proxy-aware client
identity, CSRF/CORS, and security/release headers live in a focused 285-line
policy file. Service contracts and dependency composition live in a focused
460-line file. Shared request decoding, response shaping, audit helpers, and
session-cookie behavior now live in a 250-line helper file, while invitation
lifecycle delivery lives in a focused handler, leaving
`support_handlers.go` at 455 lines. Every production file in `internal/app` is
therefore under the default 500-line CI ceiling, with existing behavior tests
preserved.

## Version 0.2.2 - Frontend Maintainability

Status: in progress (reopened for convergence hotspot work).

Goal: make route components easier to evolve without changing the visual language.

- Extract large route sections into feature-local components where it reduces complexity.
- Add consistent loading, empty, error, and retry states.
- Add request cancellation with `AbortController` through the shared API client.
- Add minimal ESLint configuration.

Exit criteria:

- Large routes are easier to modify safely.
- Search and detail loading no longer leave unnecessary in-flight requests.

Current convergence evidence: route-level loading and bundle budgets are
CI-gated. Tested list, editor, view-model, communications, insights, shared
work, touchpoint, production outreach, lead-score, and 168-line create/detail
workspace extraction plus bulk/custom-field integration and shared record
selection/work plus a 134-line contact-detail orchestrator leave `contacts.jsx`
at 449 lines, down from 2,038 and below the default 500-line ceiling. Tested 68-line selection and
142-line work hooks abort obsolete loads, distinguish repeated A-to-B-to-A visits,
serialize contact mutations, validate returned record/work identities, and keep
late saves, notes, and tasks off the active contact. The 229-line outreach hook clears record-scoped email and
sequence state on contact changes and rejects late responses from prior
selection epochs; a tested 59-line lead-score hook additionally rejects duplicate
in-flight evaluations, mismatched contact identities, and late responses after
leave-and-return navigation. Development-only call, SMS, and meeting orchestration remains
in a 456-line focused module excluded from production builds. Shared record-work cards,
touchpoints/account/health context, company editor/view helpers, and focused
142-line directory plus 82-line linked-people presentation, a 155-line create/detail
workspace, and tested 70-line company-people plus 176-line directory-state hooks leave `companies.jsx`
at 458 lines, down from 1,364 and below the default 500-line ceiling. The directory hook owns
bootstrap data, filters, loading, and request identity so stale bootstrap/search results cannot replace
the latest list. A 178-line
company-detail orchestrator owns direct-route, directory-selection, related-deal,
work, and seeded-create state. Company selection now shares the same visit-identity,
pending-mutation, work, and response-validation contract as contacts and deals, while
the focused people hook calls one transactional linked-person endpoint and rejects
late responses after leave-and-return navigation. Deal view,
shared work, quote, signature, and bulk-action components leave `deals.jsx` at
460 lines, down from 1,365 and below the default 500-line ceiling, with directory, shared-form, editor, and detail-workspace
presentation isolated in 157-, 74-, 87-, and 107-line modules, directory/bootstrap state in a tested 231-line hook, and route/detail loading in a 164-line orchestrator. The directory hook owns filters, URL/history synchronization, loading, options, and request identity without repeating its full bootstrap after each filter change. The shared selection and work hooks
abort obsolete loads, distinguish A-to-B-to-A visits, serialize
snapshot-changing mutations, expose one pending state so incompatible commercial
controls wait for durable completion, suppress duplicate work actions, validate
returned identities, and keep notes, tasks,
activities, quote lines, and proposal tracking on the active deal; the guarded
187-line commercial hook shares that contract. The generic follower control
also rejects responses for an earlier record. Task filtering,
sorting, labels, due-date view logic, a shared 98-line create/update form,
207-line directory, and 64-line create/detail workspace plus tested 88-line
quick-action and 128-line detail-state hooks leave `tasks.jsx` at 496 lines,
down from 1,093 and below the default 500-line ceiling. Quick and full-form mutations
validate response identity and cannot replace a newer selection; full-form saves also
suppress duplicate submission and cannot navigate after route unmount. Tighter source ratchets preserve every reduction while
holding every production route to the default 500-line ceiling with no explicit exceptions.

## Version 0.2.3 - Database Integrity

Status: complete.

Goal: move important invariants closer to the data.

- Add constraints for roles, statuses, entity types, and positive monetary values.
- Add unique indexes where duplicates are not valid.
- Add integration-style migration verification against Postgres.
- Review indexes for common list/search patterns.

Exit criteria:

- Invalid core states are rejected by the database, not only by application code.
- Migration tests verify real schema outcomes.

Current convergence evidence: in addition to schema constraints, cross-record
writes are being reviewed as vertical transactions. Organization-client person
creation now reserves hosted contact capacity and commits the normalized
non-client contact, typed custom values, primary-safe link, company timestamp,
and contact/company activities together. A disposable-PostgreSQL test forces
the link insert to fail and proves no orphan contact or activity remains; the
same suite covers wrong-tenant and individual-client rejection, and the browser
journey exercises the endpoint through linked-person creation and duplicate
review.

## Version 0.3.0 - Professional Release Candidate

Status: complete.

Goal: complete the professional-grade baseline and prepare for real usage feedback.

- Re-run security and reliability review.
- Verify full local bootstrap from clean checkout.
- Verify CI, deploy, migration, backup, and restore workflows.
- Update README to reflect current production posture and roadmap status.

Exit criteria:

- The project is ready to present as a professional, production-conscious CRM foundation.
- Remaining work is product-driven rather than infrastructure cleanup.

Completion notes:

- CI verifies Go formatting, `go vet`, backend tests, frontend tests, frontend lint, and frontend build.
- CI runs the migration suite against disposable PostgreSQL and verifies core constraints/indexes.
- Deploy workflows have passed after the frontend, backend, and database-integrity hardening slices.
- README now reflects the current production posture, local verification commands, operational runbook, and release-candidate status.

## Version 0.3.1 - First-Use Product Polish

Status: complete.

Goal: reduce first-session confusion for a new operator evaluating the CRM.

- Add helpful empty states for dashboard, contacts, companies, deals, and tasks.
- Add lightweight guidance for the first three useful setup actions.
- Make seeded/demo data boundaries clear in local development.
- Review copy for business-profile labels and onboarding paths.

Exit criteria:

- A new user can understand what to create first without reading docs.
- Empty accounts feel intentional rather than broken.

Completion notes:

- Dashboard now shows a first-use setup guide when the workspace has no CRM activity.
- Contacts, clients, deals/jobs, and tasks distinguish empty accounts from filtered empty results.
- Empty states provide direct next actions or reset actions where useful.

## Version 0.3.2 - Saved Views And Filters

Status: complete.

Goal: let operators quickly return to the slices of CRM data they use repeatedly.

- Add saved filter/view definitions for contacts, companies, deals, and tasks.
- Support per-user default views where it is useful.
- Keep the URL query state shareable and compatible with saved views.
- Add tests around view creation, update, delete, and application.

Exit criteria:

- Users can save and reuse common filtered lists.
- Existing search/filter URLs keep working.

Completion notes:

- Added organization/user-scoped saved views for contacts, clients, deals/jobs, and tasks.
- Added save, load, apply, update, default, and delete controls without replacing shareable URL filters.
- Added backend route tests and frontend coverage for applying a saved pipeline view.

## Version 0.3.3 - Import Foundation

Status: complete.

Goal: make it possible to bring real CRM data into the system safely.

- Define CSV import formats for contacts and companies.
- Add backend parsing and validation without committing rows on invalid files.
- Add a preview endpoint for row-level errors and warnings.
- Document the first supported import templates.

Exit criteria:

- Users can validate contact/company CSV files before import.
- Bad files produce actionable row-level feedback.

Completion notes:

- Added authenticated CSV import preview support for contacts and companies.
- Contact template columns: `first_name,last_name,email,phone,address_line1,address_line2,city,state,postal_code,country,job_title,status,is_client`.
- Company template columns: `name,client_type,address_line1,address_line2,city,state,postal_code,country,industry,phone,website,status`.
- Preview validates required columns and row-level requirements without committing imported rows.

## Version 0.3.4 - Activity Timeline Improvements

Status: complete.

Goal: make record history easier to scan and trust.

- Group activity by date on record detail pages.
- Add clearer activity summaries for create, update, archive, stage change, note, and task events.
- Add filters for activity type where records become noisy.
- Ensure activity entries include enough context for audits without exposing secrets.

Exit criteria:

- A user can understand what changed on a record without reading raw data.
- Activity timelines remain usable as record history grows.

Completion notes:

- Added a shared activity timeline for contacts, clients, deals/jobs, and tasks.
- Activity entries are grouped by date with clearer type labels and time-only row metadata.
- Records with multiple activity types expose an activity-type filter to reduce noisy histories.

## Version 0.3.5 - Dashboard Decision Support

Status: completed in `0.3.5`.

Goal: make the dashboard guide action instead of only showing counts.

- Add overdue task and upcoming task sections.
- Add stalled deal indicators based on last activity or stage age.
- Add recently touched contacts/companies.
- Keep all dashboard queries indexed and covered by tests.

Exit criteria:

- The dashboard tells an operator what needs attention today.
- Dashboard performance remains predictable on larger demo datasets.

Completion notes:

- Added a task focus card that separates overdue, due-today, and upcoming task decisions.
- Added a pipeline signal card that highlights clear, untouched, stale, or recently touched pipeline states from the existing dashboard activity feed.
- Added recently touched contacts/clients from recent activity without introducing new dashboard queries.
- Covered the new dashboard decision-support UI with router tests.

## Version 0.3.6 - Admin Audit Trail

Status: completed in `0.3.6`.

Goal: make administrative changes visible and reviewable.

- Record user invites, role changes, password setup events, and profile changes.
- Add an admin-only audit view.
- Avoid storing sensitive token values or password material.
- Add retention guidance for audit records.

Exit criteria:

- Admins can review high-impact administrative events.
- Audit data avoids sensitive secret leakage.

Completion notes:

- Added organization-scoped audit events with indexed reads by organization, event type, and creation time.
- Recorded user invites, role changes, password setup completions, and organization profile changes without storing setup tokens or password material.
- Invitation resend and revocation record secret-free lifecycle audit events; repeated revocation is idempotent and does not duplicate evidence.
- Added an admin-only audit API and Settings audit view with event filtering and retention guidance.
- Covered audit event access, metadata sanitization, role updates, and audit UI behavior with tests.

## Version 0.3.7 - Data Export

Status: completed in `0.3.7`.

Goal: give operators confidence that their CRM data is portable.

- Add CSV export for contacts, companies, deals, and tasks.
- Respect organization scoping and archived filters.
- Include predictable column headers and stable date formats.
- Document export behavior and limitations.

Exit criteria:

- Core CRM lists can be exported from the UI.
- Exports are scoped to the current organization and easy to open in spreadsheets.

Completion notes:

- Added organization-scoped CSV export endpoints for contacts, clients, deals, and tasks.
- Matched existing list filters, excluded archived records, and added task export support for due-view and assignee filters used in the UI.
- Added stable CSV headers, UTC task timestamps, spreadsheet-friendly UTF-8 output, and browser download actions on core list pages.
- Covered export routing, CSV generation, filter construction, and frontend export URL behavior with tests.

## Version 0.3.7a - Architecture Decision Records Seeding

Status: complete.

Goal: capture the sticky architectural decisions that already shape the codebase so future contributors understand the boundaries.

- Create `docs/adr/` with a small numbered template.
- Record: server-side sessions over JWT, stdlib `net/http` over framework, plain SQL over ORM, JavaScript over TypeScript on the frontend, modular monolith over services.
- Add a short `docs/ui-guidelines.md` capturing existing visual primitives, spacing scale, and copy stance.
- Link ADRs from README and `mvp.md` where relevant.

Exit criteria:

- New contributors can understand the why of the stack without inferring it from code.
- Future deviations from these decisions go through an ADR update, not a silent commit.

Completion notes:

- Added `docs/adr/` with five ADRs: stdlib HTTP, server-side sessions, plain SQL, JavaScript over TypeScript, modular monolith.
- Added `docs/ui-guidelines.md` covering tokens, spacing scale, components, layout primitives, form patterns, table patterns, copy tone, and CSS organization.
- Updated README repo layout to reference `docs/adr/` and `docs/ui-guidelines.md`.

## Version 0.3.7b - Responsive And Mobile Pass

Status: complete.

Goal: make core CRM workflows usable on smaller viewports without committing to a separate mobile app.

- Audit list, detail, settings, and dashboard views at common breakpoints.
- Fix overflow, table reflow, and side-nav behavior on narrow widths.
- Verify touch target sizing for primary actions.
- Add a small set of layout tokens or utilities only if they reduce duplication.

Exit criteria:

- Operators can use the CRM from a phone or tablet for read-heavy workflows.
- Layout regressions are visible in tests where practical.

## Version 0.3.7c - Error Boundaries And Session UX

Status: complete.

Goal: make unexpected client failures and session expiry feel intentional instead of broken.

- Add a top-level React error boundary with a recoverable fallback UI.
- Detect 401 responses centrally and route through a clear "session ended" path.
- Add inline reconnect/retry behavior for transient API errors where it does not hide real problems.
- Avoid silent reload loops and double-submit edge cases on auth failure.

Exit criteria:

- A client crash shows a recoverable surface, not a blank page.
- Expired sessions return users to login with context preserved where safe.

## Version 0.3.8 - Accessibility And Keyboard Pass

Status: complete.

Goal: make core workflows usable without relying on pointer-only interactions.

- Review focus states, labels, landmarks, and button semantics.
- Add keyboard paths for list/detail workflows and forms.
- Fix high-impact contrast or screen-reader issues.
- Add targeted accessibility tests where practical.

Exit criteria:

- Core navigation and record workflows are keyboard usable.
- Form controls and status messages are understandable to assistive technology.

Completion notes:

- Promoted `<h2>` page headings to `<h1 id="page-heading">` with `aria-labelledby` on `<main>`; demoted site header `<h1>` to `<p class="org-name">`.
- Added skip link and `id="main-content"` on `<main>`; added `role="alert"` on auth error paragraphs.
- Darkened `--text-muted` to `#546477` for WCAG AA contrast.
- Converted export buttons to `<a href>` for semantic keyboard access; added `type="search"` on search inputs.
- Added `usePageTitle` hook; all 11 routes set `document.title`.
- `<Card>` changed from `<section>` to `<div>` to eliminate unnamed landmark noise.
- Added a CI-gated axe-core Chromium journey against disposable PostgreSQL. It
  rejects automated WCAG A/AA violations across ten public and authenticated
  critical surfaces, attaches structured per-surface findings, and verifies
  the keyboard skip-link entry point. The first run caught and fixed the active
  status chip's insufficient 4.39:1 contrast by using the existing strong
  accent token.

## Version 0.3.8a - Tenant Isolation Hardening

Status: complete.

Goal: lock down the highest-value invariant in a multi-tenant CRM before the team-CRM milestone widens the write surface.

- Add an integration test suite that walks every authenticated route with a foreign-org actor.
- Verify cross-org reads return `404`, not `403`, so record existence is not leaked.
- Verify cross-org writes, updates, archives, exports, and saved-view operations all reject.
- Add a small helper or convention for organization scoping in repositories so future modules cannot forget it.
- Document the isolation contract in an ADR.

Exit criteria:

- Every existing module has a tested cross-org negative path.
- Adding a new module requires extending the isolation suite, not opting out of it.

Completion notes:

- Added `apps/api/internal/app/cross_org_test.go` with 11 negative-path tests covering PATCH/DELETE for contacts, companies, deals (incl. stage), tasks, and saved views.
- All foreign-org operations return `404` (no existence leakage via `403`).
- Uses existing `fakeXService` stubs and `authenticatedXServer` helpers; no new test infrastructure required.

## Version 0.3.8b - Dependency Hygiene

Status: complete.

Goal: keep dependencies small, current, and auditable as the project ages.

- Enable Dependabot or Renovate for Go modules, npm, Docker, and GitHub Actions.
- Add `go mod tidy` and `npm audit --omit=dev` checks to CI as advisory or required gates.
- Document the dependency budget rules from `mvp.md` in a short policy doc.
- Track third-party version skew in the roadmap rather than chasing every minor bump.

Exit criteria:

- Security-relevant updates surface as PRs without manual checking.
- The dependency surface stays explicit and small.

Completion notes:

- Added `.github/dependabot.yml` — weekly PRs for `gomod`, `npm`, Docker, and monthly `github-actions` updates.
- Added `go mod tidy`, pinned `govulncheck`, and `npm audit --audit-level=high` gates to CI.
- Promoted `golang.org/x/crypto` from indirect to direct in `go.mod`.
- CI path triggers extended to include `.github/dependabot.yml`.
- Added `docs/dependency-policy.md` with supported runtime baselines, blocking
  audit levels, the module-only advisory exception process, and update cadence.

## Version 0.3.9 - Release Readiness Review

Status: complete.

Goal: close out the `0.3.x` polish cycle before moving into team CRM work.

- Re-run local and CI verification.
- Smoke test live frontend/API after deploy.
- Review open product friction from first-use testing.
- Update README and roadmap with the next milestone focus.

Exit criteria:

- `0.3.x` is stable enough to use for real pilot feedback.
- The next milestone starts from product evidence rather than assumptions.

Completion notes:

- All Go packages pass; 57/57 frontend tests pass locally.
- `0.3.8` through `0.3.8b` completion notes captured; progress table updated.
- Next milestone: `0.4.0` Multi-User Team CRM — ownership, assignment, and admin lifecycle.
- Remaining `0.3.x` product friction (role-aware UI, bulk actions, import mapping) deferred to their planned slices.

## Version 0.4.0 - Multi-User Team CRM

Status: complete.

Goal: move from single-operator usage toward small-team CRM workflows.

- Improve team member visibility across owners, assignees, and recent activity.
- Make ownership and assignment transitions clear in contacts, companies, deals, and tasks.
- Strengthen admin workflows for adding, disabling, and reviewing users.
- Add team-focused acceptance tests for shared records.

Exit criteria:

- A small team can use Open CRM without losing track of record ownership.
- Admins can manage team access confidently.

Completion notes:

- Added `owner_user_id` and `owner_user_name` to contacts, companies, and deals list responses via LEFT JOIN on users.
- Added `assigned_to_user_id` server-side filter for tasks.
- Added `owner_user_id` server-side filter for contacts and companies.
- Added owner/assignee chips in contacts, companies, and deals list rows.
- Added owner filter dropdown with URL persistence (`?owner=`) on contacts and companies pages.
- `GET /api/users` relaxed from admin-only to any org member so filter dropdowns can populate.
- Users list shows "Pending setup" hint when `setupPending` is true.

## Version 0.4.1 - User Profile And Preferences

Status: complete.

Goal: let each user control basic identity and working preferences.

- Add profile editing for first name, last name, and display identity.
- Add default landing view and basic list preference storage.
- Add timezone preference planning if date handling requires it.
- Keep organization-level settings separate from user preferences.

Exit criteria:

- Users can keep their own profile information current.
- Preferences improve workflow without creating broad settings complexity.

Completion notes:

- Added migration 012: `preferences JSONB NOT NULL DEFAULT '{}'` column on `users` table.
- Added `PATCH /api/me/profile` to update first name and last name; records `user.profile_updated` audit event.
- Added `GET /api/me/preferences` and `PATCH /api/me/preferences` backed by the JSONB preferences column.
- Valid `defaultLandingView` values: `""`, `"/dashboard"`, `"/companies"`, `"/deals"`, `"/tasks"`.
- Added `settings/profile` route with personal profile form and landing view preference form.
- Added "My Profile" link to sidebar navigation.
- After login, if no specific return path is set, the user is redirected to their preferred landing view.
- `refreshSession()` called after profile save so the header name updates immediately.
- 65/65 frontend tests pass; Go compiles clean.

## Version 0.4.2 - Team Assignment Views

Status: complete.

Goal: make assigned work and owned records easier to review.

- Add "mine", "unassigned", and teammate filters for tasks and deals.
- Add owner/assignee chips in list views.
- Add admin/team views for workload review.
- Preserve URL-driven filter state.

Exit criteria:

- Users can quickly find their work and unowned work.
- Team leads can see assignment gaps.

Completion notes:

- Added `UnassignedOnly bool` to all four `ListQuery` structs (tasks, deals, contacts, companies); SQL filter builders emit `AND <col> IS NULL` when set.
- API handlers parse `?unassigned=true` and `?assignedToUserId=` / `?ownerUserId=` parameters.
- Frontend libs (`tasks.js`, `deals.js`, `contacts.js`, `companies.js`) pass `unassigned=true` or owner/assignee user ID to the server.
- All four list routes updated: server-side "Mine" button (sets `currentUserId`), "Unassigned" button, and teammate dropdown; assignee/owner state persisted in URL.
- `matchesAssignee` also applied client-side for immediate UI feedback while server reload settles, consistent with entity-type filter pattern.
- 65/65 frontend tests pass; Go compiles clean.

## Version 0.4.3 - Role Permissions Pass

Status: complete.

Goal: align UI affordances and API enforcement for owner, admin, member, and viewer roles.

- Review all write endpoints for role expectations.
- Hide or disable UI actions that the current role cannot perform.
- Add tests for forbidden role paths.
- Document the role model in README or operations docs.

Exit criteria:

- Role behavior is consistent across UI and API.
- Viewers cannot mutate CRM or admin state.

## Version 0.4.4 - Notification Preferences

Status: complete.

Goal: prepare for useful notifications without adding noisy channels too early.

- Add in-app notification preference storage.
- Define notification-worthy events for assignments, mentions, and due tasks.
- Add a simple notification center or placeholder inbox.
- Avoid email delivery until event quality is proven.

Exit criteria:

- Notification settings exist before external delivery channels are introduced.
- Events can be generated and reviewed in-app.

Completion evidence (2026-07-20): task and deal assignment preferences are
persisted before delivery. Deal create/update, bulk reassignment and rollback,
and user-deactivation reassignment now write the recipient event in the same
transaction as the owner change. A monotonic per-deal owner generation makes
unchanged saves and transaction retries quiet without suppressing a later
assign-away/back event; inactive, foreign, self, and opted-out recipients do not
receive one. Disposable-PostgreSQL acceptance covers direct, bulk, lifecycle,
rollback, preference, and failed-sink behavior, while the notification-center
test and clean-browser pilot journey prove the assignment filter and deal deep
link. Startup/hourly multi-instance-safe retention now keeps acknowledged items
for 90 days and unread items for 365 days in bounded batches. Protected
aggregate metrics and validated alerts expose backlog age, reviewed event mix,
per-recipient concentration, cleanup failures, and deletion counts without
tenant or user labels; unknown event values collapse into `other`.
Disposable-PostgreSQL acceptance covers both retention boundaries,
idempotency, bounded concurrent cleanup, 24-hour event aggregation, and the
privacy-safe fallback. In-app delivery remains deliberately below
production-capable until pilot noise and event selection are validated.

## Version 0.4.5 - Mention And Follow Model

Status: complete.

Goal: let team members intentionally pull others into record context.

- Add record followers or watchers for core entities.
- Add simple `@mention` parsing in notes if it fits the UI.
- Generate notification events for mentions/follows.
- Keep permissions scoped to organization membership.

Exit criteria:

- Users can subscribe to relevant record updates.
- Mentions/follows produce reviewable in-app events.

Completion notes:

- Added organization-scoped, idempotent followers for contacts, companies, and deals with active-member and record-existence validation before serializable writes.
- Notes resolve explicit `@email` tokens only against active teammates and transactionally persist deduplicated mentions, actor/recipient following, activity, and per-user idempotent notification events.
- Added a shared record UI for loading/following/unfollowing and inserting unambiguous teammate mentions, plus notification filters and record deep links.
- Verified viewer self-follow, forbidden/cross-tenant/disabled paths, duplicate operations, and notification behavior in handler, real-PostgreSQL, UI, and browser acceptance.

## Version 0.4.6 - Team Activity Digest

Status: complete.

Goal: help teams understand what changed recently without opening every record.

- Add team-wide recent activity views with filters.
- Add "my followed records" activity filtering.
- Add date and actor filters.
- Keep activity queries indexed.

Exit criteria:

- Users can review relevant team changes from one place.
- Activity remains useful without becoming an unfiltered firehose.

Completion notes:

- Added a tenant-scoped activity digest with explicit followed-record or whole-team scope, 1/7/30-day windows, optional actor filtering, aggregate counts, stable newest-first ordering, and a 50-item ceiling.
- Added the digest to the notification center with scope/window/teammate controls, actor/record context, and record deep links.
- Real-PostgreSQL tests prove followed/unrelated/foreign activity boundaries and browser acceptance proves a mentioned teammate can review the digest and return to the source record.

## Version 0.4.7 - Admin User Lifecycle Hardening

Status: complete.

Goal: make user access changes safer over time.

- Add disable/reactivate user flows.
- Decide how reassignment works for disabled users.
- Invalidate sessions for disabled users.
- Add audit entries for lifecycle events.

Exit criteria:

- Admins can remove access without deleting historical ownership context.
- Disabled users cannot keep active sessions.

Completion notes:

- Added an organization-membership lifecycle state with active/disabled visibility in the Users settings page.
- Deactivation runs as one serializable transaction: it protects the current actor and last active owner, validates an optional same-tenant active replacement, reassigns active contacts, companies, deals, tasks, shared-inbox work, lead routing, and future meetings, ends sessions, removes record subscriptions, stops mailbox sync/reminders/jobs, and records an audit event. The complete transaction retries a bounded four times when concurrent session touches cause PostgreSQL serialization/deadlock conflicts, so access removal does not surface a transient `500` or partially apply.
- Preserved archived ownership and creator/authorship history; reactivation restores login access without silently re-enabling mailbox sync.
- Normal assignment pickers hide disabled users, while deal, task, shared-inbox, lead-routing, and booking-link services reject crafted disabled-user assignments.
- Added handler permission/error tests, a freshly migrated disposable-PostgreSQL lifecycle test that deterministically forces concurrent session-touch serialization recovery and covers cross-tenant/forbidden paths, and invited-user activation/deactivation/reactivation coverage in the real-PostgreSQL browser journey.

## Version 0.4.8 - Team Usage Reporting

Status: in progress (technical outcome complete; approved pilot usage evidence pending).

Goal: give admins basic visibility into whether the CRM is being used.

- Add reports for active users, records created, tasks completed, and notes added.
- Keep reporting lightweight and operationally useful.
- Avoid surveillance-style metrics that do not help the CRM workflow.
- Add date range filters.

Exit criteria:

- Admins can see adoption and usage trends.
- Reports are simple enough to trust and explain.

## Version 0.4.9 - Team Release Review

Status: complete.

Goal: close the team workflow milestone before data operations work.

- Re-run role, permissions, and user lifecycle tests.
- Re-run the tenant isolation suite against the new write surfaces.
- Smoke test multi-user workflows in a seeded environment.
- Update documentation for team operation.
- Identify remaining team workflow gaps from usage.

Exit criteria:

- Small-team usage is stable enough for pilot customers.
- Remaining team work is clearly prioritized.

## Version 0.5.0 - CRM Data Operations

Status: complete.

Goal: make data maintenance practical for real CRM usage.

- Support bulk workflows for common list maintenance.
- Improve import/export beyond the initial foundation.
- Add duplicate and data quality tools.
- Keep every destructive operation reversible or explicitly confirmed.

Exit criteria:

- Operators can maintain CRM data without one-record-at-a-time friction.
- Data operations are safe enough for production use.

Completion evidence (2026-07-19): versions 0.5.1–0.5.9 now form one tested
operator workflow: bounded mapped imports with checkpoint resume and safe
rollback, reversible bulk maintenance, reviewed permanent duplicate merge,
typed contact/client custom fields through forms and data interchange, live
quality queues, explicit archive recovery, and complete core-record CSV exports
within their stated synchronous boundary. Each sub-version below records its
role, tenant, transaction, idempotency, recovery, observability, and acceptance
evidence; the integrated release review closes the combined milestone.

## Version 0.5.1 - Bulk Actions

Status: complete.

Goal: reduce repetitive record maintenance.

- Add multi-select list interactions.
- Support bulk archive, owner assignment, status changes, and task completion where appropriate.
- Add confirmation flows for destructive bulk actions.
- Add backend safeguards for organization scoping and row counts.

Exit criteria:

- Common bulk edits can be completed safely from list views.
- Accidental destructive actions are hard to trigger.

Completion evidence (2026-07-19): contacts, organization and individual clients, deals, and tasks now expose keyboard-accessible multi-select controls for bounded owner/assignee, status/task-completion, and archive changes. The API normalizes and deduplicates at most 100 ids, rejects the entire serializable transaction when any id is missing or foreign, validates active same-organization assignees, and uses a stable request digest plus idempotency key. Persistent operation/row state contains only reversible fields; in-list history can undo an operation while exact `updated_at` checks leave later teammate edits untouched and report partial recovery. Every changed record gets activity history and each completion/rollback gets an aggregate audit event. Handler role/scope/conflict tests, a disposable-PostgreSQL idempotency/tenant/assignee/change-aware rollback acceptance test, UI failed-request retry and persistent recovery tests, and the real-PostgreSQL browser journey cover the slice.

## Version 0.5.2 - Duplicate Management

Status: complete.

Goal: help users identify and resolve duplicate contacts and companies.

- Add duplicate candidate detection views.
- Support manual merge or archive workflows.
- Preserve notes, tasks, activities, and links during merge decisions.
- Add tests for duplicate resolution edge cases.

Exit criteria:

- Users can resolve obvious duplicates without direct database work.
- Merges do not orphan related records.

Completion evidence (2026-07-19): an admin-only Data Quality view finds active contact pairs by exact normalized email, phone, or name and client pairs by exact normalized website, phone, or name; explains the match and current linked-work counts; and requires the operator to choose the survivor plus every selectable differing field. The serializable merge locks both tenant records, rejects a stale review, uses a request digest and organization-scoped idempotency key for safe retry, retains the safest client flags, archives rather than deletes the source, and collision-safely consolidates notes, tasks, activity, meetings, calls/SMS, email links, followers, notifications, contact/client links, deals, lead submissions, and sequence enrollments. Import, bulk-operation, and audit ledgers deliberately retain original record IDs for historical accuracy. Permanent-operation copy, explicit browser confirmation, tenant-scoped merge history, survivor activity, and aggregate audit make the decision visible. Handler role/scope/conflict tests, UI permission/retry tests, disposable-PostgreSQL contact/client relationship, cross-tenant, inactive-actor, idempotency, and stale-review acceptance, plus the clean-migration Chromium pilot journey cover the slice.

## Version 0.5.3 - Import Mapping UI

Status: complete.

Goal: make CSV import usable for real-world files with varied column names.

- Add upload and column mapping screens.
- Suggest mappings for common column names.
- Show preview rows before import.
- Persist no file contents longer than needed.

Exit criteria:

- Users can map CSV columns without editing files by hand.
- Imports remain transparent before data is written.

Completion notes:

- Added an admin Data Imports route for contacts and companies with common-header suggestions, explicit editable mappings, and a row-level dry run before writes.
- CSV files are bounded to 2 MiB/1,000 rows, processed in memory, and never persisted; only the mapping, source digest, counts, record ids, and privacy-safe issues remain.
- Added UI and handler acceptance for multipart mapping and forbidden roles.

## Version 0.5.4 - Import Validation And Rollback

Status: complete.

Goal: make imports safe after validation moves from preview to write.

- Write imports as tracked batches.
- Add row-level success/failure reporting.
- Support rollback for recently imported batches where feasible.
- Add operational guidance for failed imports.

Exit criteria:

- Import results are auditable.
- Bad imports can be corrected without manual SQL in common cases.

Completion notes:

- Added organization-scoped import batches and row outcomes with source-hash idempotency, serialized tenant execution, 50-row durable checkpoints, retry/resume, duplicate skips, progress/history, and downloadable row-error CSV.
- Imported contacts/companies receive normal owner/activity behavior. Completion and rollback are audited, and cross-tenant/disabled-actor paths are rejected.
- Rollback archives only records whose `updated_at` still matches the imported version; changed or already archived records remain active and are reported for manual review.
- Disposable-PostgreSQL acceptance covers idempotent replay, mismatched payload conflicts, errors without retained row values, tenant isolation, changed-record protection, and full rollback. The pilot load gate writes 1,000 mapped rows under 10 seconds, and Chromium covers import plus recovery.

## Version 0.5.5 - Custom Fields Foundation

Status: complete.

Goal: support lightweight organization-specific data without turning into a schema builder.

- Add custom field definitions for selected entities.
- Support a small initial set of field types.
- Store values with validation and organization scoping.
- Keep core fields first-class and explicit.

Exit criteria:

- Organizations can capture a few business-specific fields.
- Custom fields do not compromise core schema clarity.

Completion evidence (2026-07-19): migration 064 adds organization-scoped
contact/company definitions and JSONB values without changing explicit core
columns. Admins can create, edit, and archive at most 25 active text, number,
date, boolean, or single-select definitions per record type; stable keys and
types are immutable, options and payloads are bounded, required fields are
validated atomically on create/edit/import, and archived definitions retain
historic values while leaving normal product paths. Typed forms and list values
serve contacts, organization clients, and linked-person creation. Definition
changes are audited, disabled actors and foreign tenants are rejected, and
removing a select option still in use returns a conflict. Handler/unit/UI tests,
a real-PostgreSQL vertical acceptance suite, and the Chromium pilot journey
cover the foundation.

## Version 0.5.6 - Custom Field Filtering

Status: complete.

Goal: make custom fields useful in day-to-day list workflows.

- Add list display options for custom fields.
- Add filtering for supported custom field types.
- Include custom fields in saved views where appropriate.
- Review query plans before broad usage.

Exit criteria:

- Custom field data can be used to find records.
- Filtering remains performant on realistic datasets.

Completion evidence (2026-07-19): active fields marked for list display render
on contact/client rows; type-appropriate equality, text containment, numeric
range, and date range filters compile to tenant-scoped SQL over a GIN-backed
JSONB object. Filter state round-trips through URLs, saved views, and contact or
organization-client exports. CSV preview/write dynamically maps active custom
columns with the same validation, and duplicate review can explicitly select
differing custom values while preserving unselected target values, including
false and zero. The disposable-PostgreSQL suite proves filters, imports,
exports, duplicate merge, archive retention, and tenant isolation together; the
clean-schema browser journey proves administration, required values, import
mapping, list display, and selected merge in the pilot workflow.

## Version 0.5.7 - Data Quality Reports

Status: complete.

Goal: surface incomplete or suspicious CRM data before it causes workflow issues.

- Add reports for missing owners, missing contact details, stale deals, and incomplete records.
- Make reports actionable with links to filtered records.
- Support business-profile-specific quality rules.
- Add tests for report counts and filters.

Exit criteria:

- Operators can find data cleanup work without manual searches.
- Reports produce explainable counts.

Completion evidence (2026-07-19): every active member, including viewers, can
open live data-quality queues in **Reports**. Five fixed, tenant-scoped queries
count missing ownership across contacts/clients/open deals/open tasks, contacts
without email or phone, open deals stale beyond a selectable 14/30/60/90-day
window, open deals missing client/contact/value/expected close, and open tasks
without a due date. One allowlisted rule adapts to the workspace profile:
services flags customer clients without a linked person, construction services
flags customers without a location, and product sales flags organization
accounts without an industry. Every queue explains its criterion and each
result's reason, excludes archived rows, returns at most 25 records with the
exact total, and links directly to the affected record. The endpoint accepts a
bounded 7–365-day stale window and performs no mutations. Handler tests cover
viewer access, organization propagation, thresholds, and safe errors;
disposable-PostgreSQL acceptance proves exact counts, all business profiles,
archive exclusion, and foreign-tenant absence; component and clean-schema
Chromium tests cover threshold refresh and report-to-deal cleanup navigation.

## Version 0.5.8 - Archive And Retention Controls

Status: complete.

Goal: make archived data behavior explicit.

- Add archived list views and restore actions where appropriate.
- Document what archive means for related notes, tasks, activities, and reports.
- Add retention planning for future hard-delete needs.
- Keep destructive deletion out unless required by real usage.

Exit criteria:

- Users can find and restore archived records.
- Data lifecycle behavior is documented and predictable.

Completion evidence (2026-07-19): a lazy-loaded **Settings > Archived Records**
view lets every active member search and filter tenant-scoped archived contacts,
clients, deals, and tasks, while only writers receive restore controls. Restore
runs as one serializable operation that locks the archived row, revalidates the
active actor and tenant, rejects hidden/foreign records, preserves record IDs and
related history, and writes per-record activity plus an audit event atomically.
Deals whose company or primary contact remains archived and tasks whose linked
record remains archived return an actionable conflict so the parent can be
restored first. Contact/client rows consumed by a permanent duplicate merge stay
visible as blocked history and cannot be revived. Core archive never hard-deletes
notes, tasks, activity, or relationship rows; independently active related work
remains active, while independently archived work is restored explicitly. Normal
lists, exports, and reports continue to exclude the archived core row until
restore. No automatic hard-delete or time-based purge exists; core archive rows
remain in the primary database and encrypted backups until the future tenant
offboarding retention/deletion workflow is explicitly implemented. Handler role
and safe-error tests, a disposable-PostgreSQL all-entity/tenant/dependency/merge-
history/audit acceptance test, member/viewer UI tests, and the clean-schema
Chromium task archive/restore journey cover the outcome.

## Version 0.5.9 - Data Operations Review

Status: complete.

Goal: close the data operations milestone safely.

- Test import, export, duplicate, bulk, custom field, and archive flows together.
- Re-run the tenant isolation suite against new bulk and custom-field write paths.
- Review data integrity constraints after new features.
- Update documentation for data operations.
- Identify scale risks before sales workflow expansion.

Exit criteria:

- Data operations are reliable enough for non-technical operators.
- New data features do not undermine schema integrity.

Completion evidence (2026-07-19): the clean 64-migration PostgreSQL Chromium
journey now performs custom-field administration, mapped import and rollback,
contact creation and reviewed custom/core-field merge, reversible bulk client
status, live quality-report navigation, task archive/restore, and authenticated
exports of contacts, clients, deals, and tasks in one flow. Export assertions
prove selected custom values survive, active data is included, and rolled-back
imports plus archived merge sources stay excluded. The complete real-PostgreSQL
suite reruns import, bulk, duplicate, custom-field, archive, collaboration,
tenant-denial, migration, and pilot-load tests; all frontend handler/component
tests run with the same slice.

The integrity review found migrations 061–064 retain organization-scoped
operation identities, composite parent/child foreign keys, bounded status/count
checks, typed JSON checks, stable custom-field keys, and tenant-first lookup
indexes. Polymorphic CRM record references intentionally cannot use one database
foreign key; serializable services therefore resolve every target inside the
session organization and the PostgreSQL negative-path suites prove foreign IDs
fail atomically. Historical actor IDs deliberately reference durable users
rather than deletable memberships. No integrity-changing migration was needed.

The review also caught and fixed a silent export-truncation defect: each export
now requests row 10,001, rejects overflow with `EXPORT_TOO_LARGE`, and states the
10,000-row synchronous ceiling beside its UI control. The measured pilot
boundaries remain 2 MiB/1,000 rows per synchronous import, 100 records per bulk
operation, 50 duplicate pairs per review, 25 active custom fields per supported
record type, 100 archive rows per request, and 25 examples per quality queue.
These are deliberate request/memory controls, not capacity claims. Larger
imports/exports require the planned durable job/offboarding package; duplicate
candidate review and JSONB quality/filter query plans should be remeasured on an
approved production-like pilot dataset before raising any boundary.

## Version 0.6.0 - Sales Workflow Maturity

Status: in progress.

Goal: make deal management useful beyond a basic pipeline list.

- Improve pipeline configuration and forecasting.
- Add sales activity and touchpoint tracking.
- Add reminders and lightweight automation.
- Keep workflow configurable without becoming a workflow engine.

Exit criteria:

- Sales operators can manage pipeline health and next actions in Open CRM.
- Sales features remain understandable and maintainable.

## Version 0.6.1 - Pipeline Configuration

Status: complete.

Goal: let organizations adapt deal stages to their sales process.

- Add stage create, rename, reorder, close/won/lost settings.
- Protect existing deals during stage changes.
- Add validation for stage uniqueness and ordering.
- Add tests for stage transitions.

Exit criteria:

- Admins can configure pipelines without database edits.
- Existing deals remain valid after stage configuration changes.

Completion evidence (2026-07-19): owners and admins can create, rename, and
choose the default among at most 10 tenant pipelines, then create, rename,
classify, and exactly reorder at most 20 unique stages per pipeline from the
dedicated Pipelines settings route. Serializable writes lock tenant
configuration, preserve stable stage IDs, and reject open/won/lost
reclassification while any active or archived deal uses a stage; safe renames
continue to update attached deals. Each change emits a transactional audit
event. Handler role/conflict tests, disposable-PostgreSQL uniqueness,
ordering, default, disabled-actor, cross-tenant, usage-protection, and audit
acceptance, focused UI tests, and the clean-schema Chromium journey cover stage
creation plus an attached deal surviving a stage rename. The 6.12 KiB lazy
admin route also removes inline creation from `deals.jsx`, reducing it from
1,052 to 1,016 lines.

## Version 0.6.2 - Deal Probability And Forecasting

Status: complete.

Goal: provide simple revenue forecasting without complex sales ops tooling.

- Add probability or confidence fields to deals or stages.
- Add weighted forecast totals.
- Add close-date range filters.
- Make forecast assumptions visible in the UI.

Exit criteria:

- Users can see unweighted and weighted pipeline values.
- Forecast numbers are easy to explain.

Completion evidence (2026-07-19): migration 065 adds bounded stage probability
percentages and backfills existing open stages from the previous documented
weighting, while won/lost stages remain fixed at 100/0. Admin pipeline settings
create and edit open-stage probabilities without changing stable stage IDs;
transactional configuration audit events retain the before/after assumption.
The dashboard accepts an explicit, validated period of at most one year and
shows unweighted, won, probability-weighted, per-owner (including unassigned),
and per-stage totals in the organization base currency, with missing exchange
rates exposed instead of silently guessed. Open deals without an expected close
date are consistently included; won deals without one use their last update
date. Deals list, saved-view URL state, and CSV export share close-date range
filters. Handler/unit tests, a disposable-PostgreSQL tenant-isolation acceptance
test, focused UI tests, and the clean-schema Chromium journey prove that changing
a used stage from 65% changes the explainable forecast without detaching its deal.

## Version 0.6.3 - Task Automation Rules

Status: complete.

Goal: remove repetitive follow-up setup for predictable CRM events.

- Add simple rules for creating tasks on deal creation, stage change, or archive.
- Keep rule conditions intentionally limited.
- Add per-organization rule settings.
- Add tests to prevent duplicate task creation.

Exit criteria:

- Common follow-up tasks can be generated automatically.
- Automation is simple enough for operators to audit.

Completion evidence (2026-07-19): Settings > Automations now exposes only the
production-capable subset of the earlier workflow-definition foundation:
organization-scoped admin rules for deal creation, a real stage change
(optionally to one destination stage), or archive, with exactly one literal
follow-up task due in 0–365 whole days. The task is assigned to the active deal
owner and falls back to the teammate who caused the event. Deal create, stage
change, direct archive, and bulk archive execute the rule, task, activity, run
ledger, and audit event in the same transaction; stable activity or bulk-event
keys and the existing unique run constraint make retries no-ops. A repeated
same-stage update emits no event or task. Unsupported broad legacy definitions
are hidden from normal navigation and recorded as safely skipped if their
trigger matches rather than partially dispatching an action. Definition changes
are audited, recent runs expose outcomes, and disabling a rule prevents future
tasks without rewriting already-created work. Handler/UI tests, a
disposable-PostgreSQL suite covering all triggers, owner fallback, replay,
unsupported rules, bulk replay, audit, and cross-tenant isolation, plus the
clean-65-migration Chromium journey cover the vertical slice.

## Version 0.6.4 - Reminder Workflow

Status: complete.

Goal: help users avoid missed follow-ups.

- Add due-soon and overdue reminder surfaces.
- Add in-app notification events for assigned tasks.
- Prepare email reminders only if in-app reminders prove useful.
- Add digest-friendly reminder queries.

Exit criteria:

- Users can see what follow-ups are due without manual filtering.
- Reminder behavior is predictable and not noisy.

Completion evidence (2026-07-19): open-task queries and matching CSV exports
now share exact saved-time buckets for overdue, the next rolling 24 hours,
later work, and missing due dates. The Tasks view exposes the overdue/due-soon
counts before filtering. Assigned tasks create one preference-aware,
transactional notification, while a versioned PostgreSQL reminder ledger
transactionally enqueues due-soon and overdue jobs on the shared durable runner.
Delivery locks and revalidates the tenant, task generation, due time, open state,
active assignee, and reminder preference; stale, replayed, opted-out, completed,
archived, or reassigned work becomes a safe no-op. Direct edits, deal automation,
bulk operations and rollback, archive restore, and member deactivation/reassignment
all refresh or quiesce reminder generations in their existing transaction.
Operators retain normal dead-letter visibility and replay. Email reminders remain
deliberately absent until pilot evidence shows the in-app path is useful.
Handler/unit coverage plus disposable-PostgreSQL delivery, preference, replay,
tenant-isolation, due-query, task-completion, and user-lifecycle acceptance, focused
UI tests, and the clean-schema Chromium journey cover preference persistence and
the due-soon task surface.

## Version 0.6.5 - Sales Activity Reporting

Status: complete.

Goal: make sales effort and outcomes visible.

- Add reports for deals created, won, lost, moved stages, and notes/tasks by date range.
- Add owner filters.
- Add basic conversion rates by stage where data supports it.
- Avoid vanity metrics without clear operator value.

Exit criteria:

- Sales teams can review activity and outcomes by period.
- Reports match underlying record history.

Completion evidence (2026-07-19): migration 067 starts an explicit per-workspace
coverage clock and adds a tenant-indexed deal-stage event ledger. Deal creation
and real stage changes now transactionally save the event-time deal name, actor,
owner, pipeline, stage position, and open/won/lost outcome beside the existing
activity, so later renames or outcome changes cannot rewrite history. The live,
viewer-safe Reports surface accepts an inclusive UTC date window of at most 366
days plus an optional retained teammate. It counts deals created/moved/won/lost,
notes, tasks created/completed, outcome-based win rate, teammate contribution,
and event-based stage entries/exits; its forward-exit rate is explicitly
forward-or-won exits divided by all exits, not a cohort funnel. Deal metrics use
the saved owner while note/task metrics use the activity actor. Windows that
begin before ledger coverage are visibly partial, and recent events deep-link
to the deal. Handler tests, a disposable-PostgreSQL suite covering snapshot
immutability, forward/backward math, disabled owners, bounded filters, activity
linkage, and cross-tenant denial, focused UI tests, and the clean-67-migration
Chromium pilot journey cover the complete slice.

## Version 0.6.6 - Contact Touchpoint Tracking

Status: complete.

Goal: make follow-up history clearer for contacts and companies.

- Add last-contacted or last-touch indicators from notes/tasks/activity.
- Add stale contact views.
- Add touchpoint summaries to record detail pages.
- Keep automatic inference transparent.

Exit criteria:

- Users can find contacts or companies that need follow-up.
- Touchpoint dates are understandable and traceable.

Completion evidence (2026-07-19): a tenant-scoped, viewer-aware read model now
derives contact and client touches from notes, durable task-completion events,
completed calls, sent/received SMS, scheduled meetings, and sent/received email.
It deliberately excludes ordinary record changes, failed communications,
cancelled meetings, reminders, and future task due dates; a record with no touch
uses its creation time, so a newly created lead is not immediately stale.
Client history combines direct client work with work on currently linked people
and returns the exact source record. Email and meeting visibility is evaluated
for the current viewer, CRM-sent email notes are deduplicated against their
durable message row, and each response carries the inference rules. Contact and
Client details expose the latest/recent history, while Reports adds bounded
14/30/60/90-day queues with retained-owner filters, exact total counts, direct
record links, and source attribution. Handler and focused UI suites plus a
disposable-PostgreSQL acceptance test cover all six sources, creation fallback,
deduplication, failed/cancelled exclusions, private mailbox/calendar behavior,
disabled owners, client rollup, limits, and cross-tenant denial. The clean-67-
migration Chromium journey covers detail refresh, client attribution, report
loading, and a foreign-touchpoint `404`; no schema change was required.

## Version 0.6.7 - Quote Or Proposal Placeholder Flow

Status: complete.

Goal: leave room for proposal tracking without building a full quoting system prematurely.

- Use the existing line-item/current-PDF foundation for a narrow proposal flow.
- Track an external proposal's manual status without claiming in-product delivery.
- Document explicit non-goals for quoting and signature.
- Avoid payment, approval, versioning, and legal-signature complexity.

Exit criteria:

- Sales users can track whether a proposal exists.
- The feature does not pretend to be a full CPQ system.

Completion evidence (2026-07-19): the existing tenant-scoped catalog/custom
line-item flow snapshots catalog values when saved, calculates subtotal,
discount, tax, and total, and updates the deal value in the same transaction.
Authenticated users can download a deliberately current-data PDF; its footer
and the deal UI state that terms, approval, delivery, immutable versioning, and
legal signature remain future work. Writers can record a recipient and manually
move an externally delivered proposal through draft/sent/signed/declined/voided
states with timestamped deal activity, while the UI explicitly says that Open
CRM neither sends the proposal nor collects an e-signature. Handler/unit and
focused UI tests plus fresh disposable-PostgreSQL acceptance cover calculations,
catalog snapshots, normalization, invalid data, activity history, PDF contents,
and cross-tenant rejection. The clean-67-migration Chromium pilot journey saves
a priced proposal, tracks it as sent, downloads the PDF, and proves a second
tenant receives `404`. This completes the intentionally narrow Phase 2
placeholder; versioned quote delivery and real signing remain Phase 4.

## Version 0.6.8 - Win Loss Review

Status: complete.

Goal: capture useful outcome context when deals close.

- Add close reason fields for won/lost deals.
- Add optional notes on close.
- Add win/loss reporting.
- Keep close reason options configurable only if real usage needs it.

Exit criteria:

- Closed deals explain why they closed.
- Win/loss reporting has useful context.

Completion evidence (2026-07-19): pipeline-stage outcome is now the sole deal
outcome write path. Moving into a won or lost stage requires one fixed,
outcome-specific pilot reason and accepts bounded optional notes; the same
transaction derives deal status, records close time/actor, writes human-readable
activity, and snapshots the reason and notes into the durable stage-event
ledger. Reopening clears the live close context without rewriting historical
events. General edit and bulk-operation paths can no longer manufacture an
outcome outside a stage transition. The detail view explains the derived
outcome, the sales report groups exact won/lost reason counts with honest
pre-tracking coverage, recent events retain close notes, and deal CSV exports
carry the close context. Expand-safe migration 68 reconciles legacy status from
stage definitions and adds `NOT VALID` allowlist/length/actor constraints that
enforce new writes without scanning or rejecting explicitly uncaptured legacy
rows. Unit, handler, focused UI, disposable-PostgreSQL transition/reopen/
constraint/snapshot/report/tenant tests and the clean-schema Chromium journey
cover the vertical flow. Reason configuration remains deliberately absent until
pilot usage demonstrates that fixed choices are insufficient.

## Version 0.6.9 - Sales Workflow Review

Status: in progress (technical review complete; approved pilot usage evidence pending).

Goal: close the sales workflow milestone before expanding customer operations.

- Test pipeline configuration, automation, reminders, and reports together.
- Re-run the tenant isolation suite against automation and reporting paths.
- Review query plans for sales reports.
- Update docs for sales workflows.
- Identify product feedback from real sales usage.

Exit criteria:

- Sales workflows are coherent end-to-end.
- Remaining sales work is prioritized from usage data.

Technical completion evidence (2026-07-19): the clean-schema Chromium journey
already composes admin pipeline/probability setup, a bounded deal-task rule,
deal creation, exactly-once automated follow-up, due-task visibility, stage
rename/forecast continuity, sales activity totals and event history, required
won close review, and export reconciliation in one workflow. The real-PostgreSQL
automation, reminder, and sales-report suites separately prove transactional
idempotency, durable replay, retained-user semantics, and foreign-tenant denial.
Migration 69 adds lock- and statement-bounded partial covering indexes for the
exact sales activity set. Report SQL now eliminates unrelated audit actions and
applies teammate filters inside both aggregates; a mixed-history PostgreSQL
planner test requires the tenant/date and owner/date stage-event and activity
paths to use their reviewed organization-scoped indexes. The supported operator
flow, definitions, deliberate gaps, and structured pilot feedback record are in
[`sales-workflow.md`](sales-workflow.md).

External evidence blocker: no approved pilot team/session or anonymized usage
observations are available in the repository, so terminology, probability,
reminder-noise, automation, report, and close-reason product decisions cannot
honestly be prioritized from real usage yet. This milestone remains in progress
until that evidence is recorded. In accordance with the convergence brief,
that external dependency does not block the already-prioritized `0.7.1`
won-deal client-handoff slice.

## Version 0.7.0 - Customer Operations

Status: in progress (technical outcome complete; approved pilot usage evidence pending).

Goal: support the post-sale relationship after a deal becomes a customer account.

- Add account/customer views that connect won deals, companies, contacts, tasks, and history.
- Add customer health and renewal-oriented workflows.
- Support service/job tracking where business profile calls for it.
- Keep the CRM general enough for small service and sales teams.

Exit criteria:

- Users can manage customer relationships after the initial sale.
- Post-sale workflows build on existing CRM records instead of creating a separate product.

## Version 0.7.1 - Post-Sale Account View

Status: complete.

Goal: make customer accounts easier to review after conversion.

- Add account summary panels for companies or individual clients.
- Show related won deals, open tasks, recent notes, and key contacts.
- Add links from won deals to customer/account context.
- Preserve existing company/contact detail workflows.

Exit criteria:

- Users can understand account state from one page.
- Post-sale context does not duplicate core CRM records.

Completion evidence (2026-07-19): winning a deal now requires a live company or
primary-contact relationship and promotes the existing explicit company to
`customer` inside the stage transaction; only a win without a company promotes
its primary contact to `customer` plus individual-client, so an organization
sale does not create a duplicate person account. Create, stage transition, and
later-edit paths enforce the relationship invariant with actionable API/UI
feedback. Legacy won deals can still be repaired by adding an account through
the same rule, repeated edits are idempotent, and reopening or a later loss
never reverses the customer relationship. The changed account receives
`client.handoff` activity and audit metadata naming the source deal. Expand-safe
migration 70 reconciles legacy
wins without inventing historical actors, gives explicit companies the same
precedence, and adds tenant/company/contact account-list indexes with bounded
lock and statement waits. A won deal links directly to the company or individual
account. The existing company/contact detail page now provides a compact
read-only summary of related won deals, open tasks on that client record, recent
client notes, and key linked contacts, followed by the unchanged source-record
workflows rather than a second account data model. Focused component tests,
disposable-PostgreSQL transition/create/unlink/reopen/late-link/idempotency/
backfill/tenant assertions, handler validation, and the clean Chromium journey
through close, account link, customer status, won deal, task, note, and key
contact cover the outcome.

## Version 0.7.2 - Client Health Signals

Status: complete.

Goal: provide lightweight health indicators for customer relationships.

- Define manual health statuses or simple derived signals.
- Add stale follow-up, overdue task, and open issue indicators where available.
- Add health filters to customer views.
- Keep health rules transparent.

Exit criteria:

- Users can quickly identify accounts that may need attention.
- Health indicators are explainable and editable where appropriate.

Completion evidence (2026-07-19): the Clients view now derives three transparent
states rather than adding another mutable status. **Needs attention** means an
overdue open task or a viewer-visible follow-up older than the selected 14-, 30-,
60-, or 90-day threshold; **Watch** means current follow-up plus an open task due
within seven days; **Healthy** means neither. Exact reasons and open/overdue/
due-soon counts appear in the queue and record summary. Operators can filter by
organization or individual client, health state, threshold, and retained owner.
Company health rolls up direct account and currently linked-person work while an
individual client remains contact-scoped. Private email and meeting touches stay
viewer-specific, archived work is excluded, and no issue signal is invented
because the product has no issue record. Migration 71 adds lock- and
statement-bounded partial query indexes. Handler validation, focused component
tests, disposable-PostgreSQL owner/privacy/company/task/cross-tenant acceptance,
and the clean Chromium won-account journey cover the outcome. The derived state
is intentionally not editable; the underlying follow-up and task records are the
auditable source of truth. A persisted override remains a pilot-validation
decision, not an incomplete implementation.

## Version 0.7.3 - Renewal And Follow-Up Tasks

Status: complete.

Goal: support recurring customer follow-up without calendar complexity.

- Add renewal or review date fields where useful.
- Add task generation for upcoming renewals/reviews.
- Add dashboard sections for customer follow-ups.
- Avoid full subscription billing logic.

Exit criteria:

- Users can track future customer follow-up obligations.
- Renewal/review tasks appear in existing task workflows.

Completion evidence (2026-07-19): every active customer company or individual
client can own one explicit review or renewal schedule. Saving it creates or
reschedules an ordinary assigned task and its durable due-soon/overdue reminder
state in the same serializable transaction. Operators can choose one time or a
1-, 3-, 6-, or 12-month cadence. Completing a recurring task advances from the
original due time until exactly one future obligation remains, so downtime or a
late completion cannot create a burst; replaying completion is a no-op. The
client page shows the schedule and current task, while Dashboard separates
overdue, due-within-30-days, and later obligations with client/task drill-down.
Direct task due-time or assignee changes reconcile the schedule, but archive,
bulk mutation, client demotion/archive, and duplicate merge are blocked until
the schedule is deliberately cleared. Clearing archives only an open generated
task; one-time completion and reopen remain recoverable. Migration 72 adds
tenant/entity and task integrity constraints. Handler and focused UI tests plus
disposable-PostgreSQL lifecycle, tenant, reminder, audit, bulk/archive, replay,
dashboard, and clean Chromium lead-to-client recurrence acceptance cover the
outcome. This is follow-up metadata, not subscription billing or a legal renewal
event.

## Version 0.7.4 - Service Or Job Tracking

Status: complete.

Goal: support business profiles that need jobs/projects connected to clients.

- Define a minimal job/service record if usage validates it.
- Link jobs to clients, contacts, notes, tasks, and activity.
- Add status tracking without full project management complexity.
- Keep labels adaptive by business profile.

Exit criteria:

- Service businesses can track active work against clients.
- The feature remains lighter than a project management system.

Completion evidence (2026-07-19): no approved pilot evidence exists to justify
a second job or project data model. For Services and Construction Services profiles, the
existing production-capable deal/pipeline record is adaptively presented as a
Job and retains its client, primary contact, owner, value, target date, stage
status, notes, assigned tasks, activity, archive/restore, saved-view, export,
tenant, and permission behavior. Delivery work remains an ordinary linked
Service Task or Site Task. Profile responses no longer advertise nonexistent
`projects` or `estimates` modules. Focused frontend tests cover the adaptive
job and task vocabulary, backend profile tests pin the supported module list,
and the PostgreSQL browser journey covers the same underlying relationships,
status transitions, task work, notes, and recovery. This is intentionally a
terminology and workflow adaptation of the mature core records, not a hidden
project-management foundation.

## Version 0.7.5 - Account Notes And Internal Handoff

Status: complete.

Goal: make sales-to-service context transfer clearer.

- Add account summary notes or handoff notes.
- Highlight important open tasks and recent deal history.
- Add activity entries for ownership or account status changes.
- Keep handoff data visible to the team, not hidden in individual notes.

Exit criteria:

- Team members can pick up customer context quickly.
- Handoff information is explicit and reviewable.

Completion evidence (2026-07-19): the won transition records a durable
`client.handoff` account activity and audit event naming the source deal, while
the resulting account summary keeps won records, close reasons, open account
tasks, recent team-visible account notes, and key contacts together with direct
drill-down. Notes remain ordinary shared record notes rather than private
handoff storage. Direct company/contact status transitions now add exact
before/after activity alongside the ordinary edit activity; bulk ownership and
status changes retain per-record activity plus reversible aggregate audit
history, and member deactivation retains its audited reassignment summary.
Disposable-PostgreSQL assertions cover explicit status activity and the existing
handoff suite covers idempotency, tenant isolation, late linking, reopening,
and account context. A separate handoff-note object or next-owner field remains
a pilot decision because current notes, ownership, and assigned tasks already
provide one reviewable source of truth.

## Version 0.7.6 - Customer Segment Views

Status: complete.

Goal: let users group customer records for follow-up and reporting.

- Add segment filters based on status, health, owner, custom fields, or tags if tags exist.
- Support saved customer segment views.
- Keep segmentation query behavior efficient.
- Avoid marketing automation scope.

Exit criteria:

- Users can find meaningful groups of customer accounts.
- Segment views are reusable.

Completion evidence (2026-07-19): ordinary client views already persist search,
retained-owner, unassigned-owner, and typed company custom-field filters. The
bounded client-health queue now saves and reapplies separately scoped customer
segments by organization/individual type, Healthy/Watch/Needs attention state,
14/30/60/90-day stale threshold, and retained owner. Scoped health segments do
not appear in ordinary client-list saved views and deliberately cannot displace
that list's default view. Existing tenant/user-scoped saved-view persistence,
the health query's tenant and owner validation, focused UI acceptance, and the
client-health PostgreSQL suite cover the reusable outcome. Tags and marketing
audience behavior are not inferred or added.

## Version 0.7.7 - Customer Activity Reports

Status: planned (deferred to Phase 4 reporting convergence).

Goal: show post-sale work and customer engagement patterns.

- Add reports for customer tasks, notes, health changes, and follow-up activity.
- Add owner and date filters.
- Add links from report rows to source records.
- Keep report definitions simple and explainable.

Exit criteria:

- Customer operations activity is visible by period.
- Reports help operators decide where to focus.

Deferral decision (2026-07-19): the live follow-up queue, client-health report,
account touchpoint history, and period/teammate sales-activity report already
provide safe operational evidence and source links. They do not constitute the
promised customer-only period report, and derived health intentionally has no
fabricated change history. Building another overlapping report now would widen
the reporting family while its general runtime is still incomplete. The
customer-only period view, exact client-rollup semantics, and any persisted
health snapshots therefore remain part of the required Phase 4 reporting
convergence; current surfaces keep their narrower production-capable labels.

## Version 0.7.8 - Customer Data Review

Status: complete.

Goal: verify that post-sale data additions remain coherent.

- Review schema constraints and indexes for customer operations features.
- Test account, customer, job, task, and note workflows together.
- Review archive behavior for customer-related records.
- Update backup/restore and data export expectations if needed.

Exit criteria:

- Customer operations data is consistent and recoverable.
- New workflows do not create orphaned or ambiguous records.

Completion evidence (2026-07-19): migrations 70-72 add tenant/account lookup,
health-query, review-schedule, and generated-task integrity without introducing
a duplicate account or job model. Won handoff, account context, adaptive jobs,
notes/tasks/activity, health rollup, recurring reviews, archive/restore, bulk
changes, duplicate merge blockers, and cross-tenant denial are exercised across
focused handlers/UI tests, disposable PostgreSQL, and the clean Chromium
lead-to-client journey. Recurring obligations cannot be orphaned by client/task
archive, demotion, merge, or bulk mutation; supported recovery is explicit.
Whole-database backup/restore includes the new state and is CI-gated. Filtered
core CSV exports remain intentionally record-focused; the portable full-tenant
package, including configuration, audit/activity, and review schedules, is a
Phase 3 offboarding requirement rather than an undocumented partial export.

## Version 0.7.9 - Customer Operations Review

Status: in progress (technical review complete; approved pilot usage evidence pending).

Goal: close the customer operations milestone before integration work.

- Smoke test end-to-end sales-to-customer lifecycle.
- Re-run the tenant isolation suite against post-sale and job/service paths.
- Review customer workflow feedback.
- Update docs and roadmap for integration priorities.

Technical review evidence (2026-07-19): the clean PostgreSQL browser journey
now covers won close context, idempotent client handoff, account summary, health
triage, review scheduling, dashboard visibility, recurring completion, and the
next generated task. The disposable-PostgreSQL suites cover the same records'
tenant, status, archive, merge, reminder, and replay paths. The capability
matrix and roadmap now distinguish adaptive Jobs from a project system, remove
unsupported module claims, and defer the distinct customer-period report to the
required Phase 4 reporting family. No approved pilot team/session or anonymized
customer-operations feedback exists in the repository, so thresholds, cadence,
handoff wording, segment usefulness, and whether a richer delivery record is
warranted cannot honestly be validated yet. That external evidence keeps this
review in progress but does not block Phase 3 engineering.
- Confirm product boundaries remain clear.

Exit criteria:

- The customer lifecycle is coherent from lead to post-sale follow-up.
- Integration work starts with clear product needs.

## Version 0.8.0 - Integrations Foundation

Status: planned.

Goal: prepare controlled integration points without turning Open CRM into a platform prematurely.

- Define public API boundaries for selected CRM resources.
- Add API token and webhook foundations.
- Add event logs for integration debugging.
- Keep integration security and observability first-class.

Exit criteria:

- External systems can integrate through deliberate, documented surfaces.
- Integration failures are debuggable by operators.

## Version 0.8.1 - Public API Shape

Status: planned.

Goal: define a stable-enough API surface for controlled external usage.

- Review current internal API paths and payloads.
- Decide which endpoints are public versus frontend-private.
- Add API versioning strategy if needed.
- Document a deprecation policy for breaking changes (notice period, sunset headers, changelog entries).
- Document request/response/error conventions.

Exit criteria:

- Public API boundaries are explicit.
- Future integrations do not depend on accidental frontend internals.

## Version 0.8.2 - API Token Management

Status: planned.

Goal: support non-browser API access safely.

- Add scoped API tokens with hashed storage.
- Add create, revoke, and last-used tracking.
- Add organization and role enforcement.
- Add audit events for token lifecycle.

Exit criteria:

- Integrations can authenticate without user session cookies.
- Tokens are revocable and never stored in plaintext.

## Version 0.8.3 - Webhook Delivery Model

Status: planned.

Goal: let external systems react to important CRM changes.

- Define webhook subscriptions for selected events.
- Add signed delivery payloads.
- Add retry and failure tracking.
- Add a UI or admin view for webhook status.

Exit criteria:

- Webhooks can be delivered securely and debugged.
- Failed deliveries are visible and retryable.

## Version 0.8.4 - Email Link Capture

Status: planned.

Goal: explore simple email-to-record workflows without full email sync.

- Define safe ways to capture email links or manually logged email summaries.
- Add fields or notes for email context if useful.
- Avoid mailbox ingestion until privacy and operational needs are clear.
- Document email integration non-goals.

Exit criteria:

- Users can record email context manually or via simple links.
- Full email sync remains out of scope until justified.

## Version 0.8.5 - Calendar Link Planning

Status: planned.

Goal: prepare for calendar-related CRM workflows without committing to provider sync.

- Identify meeting/task/date workflows that need calendar context.
- Add calendar link placeholders only if useful.
- Document provider integration requirements and risks.
- Avoid OAuth/provider complexity until usage demands it.

Exit criteria:

- Calendar integration scope is understood.
- Product value is validated before provider-specific work.

## Version 0.8.6 - Integration Event Log

Status: planned.

Goal: make integration behavior auditable and debuggable.

- Record API token usage summaries and webhook deliveries.
- Add filters for event type, status, and time range.
- Avoid logging sensitive payload secrets.
- Add retention guidance.

Exit criteria:

- Operators can troubleshoot integrations from the app.
- Logs provide enough context without exposing secrets.

## Version 0.8.7 - Import API Endpoint

Status: planned.

Goal: allow controlled programmatic data ingestion.

- Add API endpoints for import batches or selected resource creation.
- Reuse validation and rollback semantics from UI imports.
- Add rate limits and payload size controls.
- Document examples for common ingestion flows.

Exit criteria:

- External systems can send CRM data safely.
- Programmatic imports share the same integrity guarantees as UI imports.

## Version 0.8.8 - Integration Security Review

Status: planned.

Goal: harden new integration surfaces before broader usage.

- Review token scopes, webhook signing, rate limits, and audit logs.
- Add negative-path tests for cross-organization access.
- Review documentation for secret handling.
- Consider threat modeling for integration endpoints.

Exit criteria:

- Integration security controls are tested and documented.
- Cross-organization data isolation remains intact.

## Version 0.8.9 - Integration Release Review

Status: planned.

Goal: close the integration milestone before scale/reliability work.

- Smoke test token, webhook, import API, and event log workflows.
- Review operational support needs for integrations.
- Update README/API docs.
- Prioritize only validated provider integrations.

Exit criteria:

- Integration foundations are reliable enough for controlled external use.
- Provider-specific work has clear demand.

## Version 0.9.0 - Scale And Reliability

Status: in progress.

Goal: make the system resilient under larger datasets and operational failures.

- Review query performance and pagination across core resources.
- Add background job infrastructure for long-running work.
- Automate backup/restore drills where practical.
- Add monitoring and failure testing hooks.

Exit criteria:

- Open CRM can handle realistic pilot data volumes.
- Operators have stronger confidence in recovery and failure behavior.

## Version 0.9.1 - Query Performance Review

Status: in progress.

Goal: find and fix slow paths before they become production incidents.

- Review list, dashboard, report, import, and integration queries.
- Add missing indexes based on actual query patterns.
- Add representative dataset benchmarks where practical.
- Avoid premature indexing without query evidence.

Exit criteria:

- Common workflows have predictable query plans.
- Performance fixes are backed by measured query behavior.

Current convergence evidence: the real-PostgreSQL pilot gate asserts reviewed
organization-scoped index plans and exact totals for contacts, companies, deals,
and tasks, then budgets concurrent reads and writes through their real services.
Mapped import write and duplicate checks now have tenant-normalized indexes and
a 1,000-row regression budget. Dashboard, runtime report, and provider-specific
queries still need review as those end-to-end workflows converge.

## Version 0.9.2 - Pagination And Large Dataset Hardening

Status: in progress.

Goal: keep list and report pages usable as records grow.

- Review pagination behavior for all list endpoints.
- Add keyset pagination where offset pagination becomes risky.
- Ensure frontend loading states handle large pages cleanly.
- Add tests for pagination boundaries.

Exit criteria:

- Large datasets do not break core list workflows.
- Pagination behavior is consistent and documented.

Current convergence evidence: representative core lists are exercised at pilot
volumes, enforce bounded page sizes, and verify exact totals. The synchronous CSV
path is tenant-isolated and regression-tested at its explicit 10,000-row ceiling;
row 10,001 returns a clear error instead of silently truncating the dataset.
A full list-endpoint pagination boundary inventory and any evidence-driven keyset
conversion remain.

## Version 0.9.3 - Background Job Runner

Status: complete.

Goal: support long-running work without blocking HTTP requests.

- Add a minimal Postgres-backed job model or worker process.
- Support job status, retries, and failure reasons.
- Keep job execution simple and observable.
- Avoid external queues until needed.

Exit criteria:

- Long-running operations can move out of request/response paths.
- Job failures are visible and retryable.

Implementation evidence:

- Added a tenant-scoped PostgreSQL queue with stable idempotency keys, leased `FOR UPDATE SKIP LOCKED` claims, bounded exponential retries, exhausted-lease handling, dead-letter state, claim tokens, panic recovery, and graceful shutdown.
- Added admin-only queue health/filtering, safe replay, audited recovery, and explicit sequence-delivery decisions for ambiguous SMTP outcomes.
- Moved calendar reminders, automatic mailbox sync, and email sequence sends off their feature-specific execution loops. New reminders and sequence steps enqueue transactionally; mailbox cycles use a stable persisted due time.
- Added disposable-PostgreSQL acceptance tests for migrations, multi-attempt lifecycle/replay, tenant isolation, reminder idempotency, mailbox provider-message dedupe, sequence advancement, and crash/SMTP uncertainty behavior.
- Added an hourly, multi-instance-safe retention pass: successful payload/result detail compacts after 30 days and successful idempotency rows expire after 400 days in bounded `SKIP LOCKED` batches. Active and dead jobs are excluded, only seven explicitly reviewed production job types are eligible, current producers retain source-state duplicate guards, and PostgreSQL acceptance covers both cutoffs, unknown-type preservation, tenant-wide operation, idempotence, and batch limits.

## Version 0.9.4 - Async Import And Export Jobs

Status: planned.

Goal: move heavy import/export work onto the background job model.

- Run large imports asynchronously.
- Run large exports asynchronously with downloadable artifacts or safe streaming.
- Add job progress and result summaries.
- Add cleanup policies for generated files or artifacts.

Exit criteria:

- Large data operations do not time out HTTP requests.
- Users can see progress and results.

## Version 0.9.5 - Backup Automation

Status: complete (implementation and disposable acceptance; production enablement requires an approved off-host repository and credentials).

Goal: make backups less dependent on manual operator discipline.

- Add documented scheduled backup approach for the current host.
- Review configuration and secrets handling: `.env.example` drift, rotation procedure, secret scope, and inventory.
- Add backup verification metadata.
- Add alerts or logs for failed backups if monitoring exists.
- Keep backup artifacts secure and off-host where possible.

Exit criteria:

- Backups can run on a schedule.
- Operators can verify that recent backups exist.

Current convergence evidence: pinned Restic tooling produces verified custom
PostgreSQL dumps with checksum/source metadata, client-side encryption,
daily/weekly/monthly retention, repository integrity checking, atomic
success/attempt status files, and explicit failed-run output. Production scripts
reject local repositories. A daily persistent systemd timer template and
off-host credential runbook are included but intentionally not enabled or
provisioned by deployment automation.

## Version 0.9.6 - Restore Drill Automation

Status: complete (implementation and disposable acceptance; production enablement requires an approved off-host repository and credentials).

Goal: prove backups are usable, not just created.

- Add a documented restore drill procedure for disposable environments.
- Automate as much of the drill as practical.
- Record restore time and failure modes.
- Keep production restore manual and deliberate.

Exit criteria:

- Operators can regularly verify restore viability.
- Restore procedures are tested outside emergencies.

Current convergence evidence: the automated drill restores a selected encrypted
snapshot into an isolated PostgreSQL 16 container, verifies its recorded
checksum, applies current forward migrations, runs schema/data sanity checks,
records duration/results, and removes the disposable database. CI gates deploys
on the same end-to-end path and checks that seeded CRM plaintext is absent from
the Restic repository. A weekly persistent timer template is included; live
database replacement remains explicit and manual in the operations runbook.

## Version 0.9.7 - Monitoring And Alerting Hooks

Status: in progress (repository implementation complete; production scrape and alert-destination validation pending).

Goal: expose enough operational signals for production support.

- Add metrics or structured log conventions for request errors, latency, jobs, and integrations.
- Add deployment and health-check guidance for alerts.
- Add dashboard/runbook links for common incidents.
- Avoid bringing in heavy monitoring stacks unless needed.

Exit criteria:

- Production issues can be noticed and triaged quickly.
- Monitoring remains understandable for a small deployment.

Current convergence evidence: the API exposes a hidden-until-configured,
constant-time bearer-protected Prometheus endpoint with bounded route/status/
latency, live database, aggregate durable queue/lag, worker outcome,
Postmark/SMTP, and backup/restore evidence. Logs omit raw URLs, client
addresses, recipients, subjects, phone numbers, titles, and provider IDs.
Promtool-validated reference rules and initial pilot SLO/runbooks cover each
required signal without adding a runtime metrics dependency. Remaining before
completion: provision the production scrape secret, configure an approved
Alertmanager receiver, prove synthetic delivery, and measure the SLOs in a
pilot.

## Version 0.9.8 - Load And Failure Testing

Status: in progress.

Goal: validate behavior under stress and partial failure.

- Add lightweight load test scripts for common workflows.
- Test database unavailability, slow queries, and failed integrations.
- Review graceful degradation and error messages.
- Document known limits.

Exit criteria:

- The team understands practical system limits.
- Failure modes are documented and recoverable.

Current convergence evidence: real-PostgreSQL CI now seeds 12 organizations
with representative contact/company/deal/task volumes, verifies organization-
scoped query plans and totals, and runs 96 mixed list reads across 12 workers
with a 500 ms p95/2 s maximum regression budget. It also runs 32 transactional
contact creates across two tenants at a 1 s p95/3 s maximum budget, checks every
new ID through the wrong tenant, verifies exact totals, and proves bounded
closed-pool failure, one-connection pool exhaustion/recovery, and locked-table
deadline/recovery. The same gate produces and parses the tenant-isolated 10,000-
row contact export under a 5 s budget and maps/writes 1,000 contacts with
duplicate checks and progress ledgers under a 10 s budget. Postmark `503`, request deadline, and
later recovery tests complement durable sequence coverage that quarantines
ambiguous SMTP outcomes without duplicate sends. Production frontend builds
enforce raw and gzip budgets for the entry, every lazy chunk, total assets, and
CSS. Current production-URL evidence is 179.11 KiB/58.06 KiB for the entry, 44.69 KiB/12.87 KiB
for the largest lazy chunk, and 647.48 KiB/206.96 KiB total assets. Hosted
billing, invoice visibility, measured usage, and portable workspace export remain isolated in a 14.24 KiB/4.51 KiB
route and retry-key creation is a 0.15 KiB shared helper. Production builds omit
the incomplete booking-link, marketing-email, and nurture-campaign management
routes, and the bundle gate rejects their accidental inclusion; this aligns
normal exposure with executable behavior and restores aggregate headroom. Tested route
splits plus bulk/custom-field/touchpoint/close-review/account/health integration
and focused contact outreach/lead scoring/workspace/detail orchestration plus shared record selection/work, company directory/people/workspace/detail orchestration, and task directory/workspace presentation leave contacts at 449 lines,
companies at 458, deals at 460, and tasks at 496, down from 2,038, 1,364,
1,365, and 1,093 respectively.
Remaining work is production-like host evidence and later provider/feature loads.

## Version 0.9.9 - Reliability Release Review

Status: in progress.

Current convergence evidence: CI gates deploys on a Playwright/Chromium journey
against disposable PostgreSQL 16. It covers workspace bootstrap, invitation
rotation/old-link rejection/revocation/reactivation/final activation and later
member deactivate/reactivate, required custom-field administration and dynamic
import mapping with safe rollback, client/contact/deal/task creation, selected
core/custom-field duplicate merge, reversible bulk client status changes,
mentions/followed activity, task completion,
logout/login persistence, and cross-tenant denial. Backend releases now use
immutable commit-tagged images and exact release verification. Frontend Pages
artifacts now carry an exact commit marker that the deploy reads back over HTTPS;
the remaining plaintext-HTTP edge redirect is recorded as external item
`P3-O5` because the Cloudflare-proxied Pages origin currently has no custom-domain
certificate and must not have GitHub enforcement toggled blindly. New migrations
declare expand or contract compatibility; ordinary destructive migrations are
rejected. Disposable Compose acceptance proves operator rollback and automatic
restoration of the last healthy image after failed readiness. A 2026-07-21
production Docker restart exposed an API that could remain alive after its
initial database connection failed. Production startup now exits before HTTP
on that failure, the restart policy retries it, container health uses
dependency-aware `/readyz`, deploy and rollback acceptance require 45 seconds
of continuous exact-release health, and disposable Compose acceptance proves
automatic recovery from database-unavailable boot. Restore drills, broader
load/failure testing, an approved automatic production-host recovery exercise,
and the complete pilot journey remain before this review can be complete.

Goal: close the reliability milestone before production beta.

- Re-run full verification, restore drills, smoke tests, and deploy checks.
- Review open reliability risks.
- Update operations docs and README.
- Decide beta readiness criteria.

Exit criteria:

- Reliability risks are known and prioritized.
- Production beta can start with realistic operational expectations.

## Version 0.10.0 - Production Beta

Status: in progress.

Goal: reach a beta-quality product suitable for real small-team CRM usage.

- Freeze the core beta scope around proven workflows.
- Review security, reliability, data portability, and support readiness.
- Generate or refresh `THIRD_PARTY_NOTICES` and verify license obligations across Go modules and npm dependencies. Complete: the supported Linux/Node 24 and shipped Go command graphs are policy checked in CI; stale notices block deploy, and the notice ships with API and Pages artifacts.
- Prepare beta onboarding, feedback capture, and known limitations.
- Decide the post-beta roadmap based on real customer usage.

Exit criteria:

- Open CRM is ready for real beta users with documented limits.
- Future work is primarily customer-driven product development.

# Part II — Competitive SaaS Platform

> Part II assumes Part I is substantially complete: a stable modular monolith, public API + webhooks (`0.8.x`), background job runner (`0.9.3`), pagination/scale hardening (`0.9.x`), and tenant isolation tests. Each Part II family below is a major version (`1.x`) that should be decomposed into `1.x.1`, `1.x.2`, … shippable slices using the same format as Part I. The bullets under each family are the candidate slices.
>
> Sequencing rationale: `1.0` makes Open CRM sellable as SaaS (without it, no SaaS business exists). `1.1`–`1.3` deliver the highest-ROI revenue-team features (email, calling, quoting) that drive day-to-day adoption. `1.4`–`1.6` add growth, automation, and analytics. `1.7` AI is competitive table stakes and can be pulled earlier if it becomes a deal-breaker. `1.8`–`2.0` broaden into service, ecosystem, mobile, and enterprise.

## Version 1.0.0 - Multi-Tenant SaaS Platform

Status: in progress.

Goal: turn the multi-org CRM into a sellable, self-serve, metered SaaS product. This is the prerequisite for competing commercially; everything else is a feature on top of it.

Candidate slices:

- `1.0.1` Self-serve signup, email verification, and tenant provisioning (no admin bootstrap required).
- `1.0.2` Stripe billing integration: customers, subscriptions, payment methods, invoices, webhooks.
- `1.0.3` Plan tiers and seat-based pricing (Free/Starter/Pro/Enterprise) with a plan catalog.
- `1.0.4` Feature gating and entitlement checks enforced in API and UI from plan definition.
- `1.0.5` Usage metering and soft/hard limits (records, seats, emails, API calls, storage).
- `1.0.6` Billing admin UI: plan changes, upgrades/downgrades, proration, trial handling, dunning.
- `1.0.7` SSO via Google and Microsoft OAuth login (in addition to password auth).
- `1.0.8` Granular role/permission model groundwork (custom roles, per-object permissions) extending `0.4.3`.
- `1.0.9` Tenant lifecycle: trial expiry, suspension, cancellation, data retention, and self-serve export/delete (GDPR/CCPA).

Progress:

- `1.0.2` (Stripe-hosted lifecycle foundation): partial. Migration `074_stripe_billing_lifecycle.sql` adds tenant-bound durable Checkout attempts, provider references and ordering cursors, signed-event receipts, and an invoice ledger. Stripe mode creates backend-hosted Checkout and customer-portal sessions with pinned API version, bounded timeouts/responses, tenant metadata, and provider idempotency; direct browser/API plan activation is rejected. The public webhook verifies the raw-body HMAC within a five-minute replay window, rejects test/live mismatches, deduplicates event IDs and payloads, safely retries failed receipts, fails cross-tenant references closed, and applies ordered subscription/invoice state plus audit/provider telemetry. Signed subscription events—not return URLs—activate plans and reconcile provider trials, dunning grace, payment recovery, suspension, scheduled cancellation, and cancellation. Migration `075_billing_reconciliation.sql` adds tenant reconciliation attempt/success/error evidence and a bounded due index. Stripe workspaces are discovered every 15 minutes when six-hour evidence is stale, then a leased durable job retrieves current subscription state and the recent 25 invoices. It validates every tenant/customer/subscription reference, uses a retrieval-start watermark so provider polling cannot overwrite newer webhooks, records audit/provider/failure evidence, retries transient errors, and exposes dead jobs for labeled admin replay in Operations. Owners/admins now have a suspension-safe, tenant-scoped newest-first 25-invoice view in Plan & Billing with currency-correct due/paid amounts, provider attempt/next-retry/paid evidence, and absolute HTTPS invoice/PDF links stripped of unsafe schemes/userinfo; no local dunning deadline is inferred. Provider/UI tests and disposable-PostgreSQL lifecycle/reconciliation acceptance cover schedule deduplication, ordering, invoice refresh, durable retry, link safety, and cross-tenant failure. A CI-gated credential-free Stripe HTTP contract journey now connects the actual billing and CRM routes to PostgreSQL and the leased queue, proving trial discovery, one-call Checkout replay, invalid/duplicate signature behavior, webhook-only activation, provider reconciliation plus suspension-safe invoice history, past-due grace, unpaid suspension with portal recovery, reactivation, scheduled cancellation, final cancellation, and durable audit/ledger evidence. Remaining before promotion: an approved credentialed Stripe test-mode deployment smoke, explicit portal/proration/resubscription policy, automated dunning timing and quota/feature enforcement, and tenant deletion/retention policy.
- `1.0.9` (trial-expiry / cancellation enforcement): partial. Only a Stripe-configured runtime is managed. There, the shared `CheckWritable` decision treats canceled, expired-trial, unpaid, paused, incomplete, and incomplete-expired state as read-only while active, in-period trial, and past-due payment-retry grace remain writable. A centralized route-derived boundary applies that decision after authentication and role checks to every registered private tenant mutation, including update/delete/provider actions and the credential-writing OAuth callback; an executable registration scan proves coverage. It fails closed with `503` when entitlement state cannot be read, while reads, exports, billing recovery, profile/preferences, and notification acknowledgement remain available. Public lead capture locks and checks the destination subscription plus contact limit, returning a leak-safe unavailable response. Every non-recovery tenant worker checks the same policy before effects and uses a durable 15-minute deferral that does not consume attempts; billing reconciliation and workspace export generation remain exempt so recovery/offboarding continue. Fake/self-hosted mode reports `unmanaged`, exposes unlimited local usage, hides hosted controls, and skips the lifecycle/limit policy for private routes, public leads, and workers. Shared client state mirrors both boundaries. Backend, frontend, and disposable-PostgreSQL tests cover the decision, full route set, role ordering, recovery exceptions, public submission, deferral, signed provider transitions, self-hosted bypass, and read-only journey. Remaining: explicit dunning timing plus tenant deletion/retention policy.
- `1.0.9` (portable workspace export): complete. Migration `076_workspace_exports.sql` adds tenant/requester-bound export metadata, idempotency, durable status/error, checksums, counts, expiry, and artifact storage. Owner/admin requests transactionally enqueue `workspace.export.generate` and audit the request even when hosted writes are suspended. An organization lock permits one active generation regardless of browser tabs/API clients. The worker produces one repeatable-read ZIP snapshot with manifest and NDJSON for every explicitly classified tenant business table; archived/history/configuration data is included, while auth/session/token/provider-secret/Stripe-sensitive/private-mail/internal-ledger data is redacted or excluded. A runtime schema-coverage guard refuses unknown tenant tables. Generation is retryable, observable, bounded to 10 MiB per row, 200 MiB uncompressed, and 50 MiB compressed; only the three newest ready artifacts remain downloadable, otherwise files expire after seven days, retain metadata after byte cleanup, and expose SHA-256 plus audited download. The request control remains disabled until initial history reconciliation completes so a stale load cannot hide a newly queued export or stop status polling. Handler/UI, disposable-PostgreSQL redaction/expiry/idempotency/cross-tenant acceptance, and the clean browser journey cover the outcome. Larger operator-streamed export, tenant deletion, and retention policy remain separate explicit work.
- `1.0.6`/`1.0.9` (subscription status + trial): partial. Migration `015_subscription_lifecycle.sql` introduced bounded subscription states and trial end; migration `073_verified_workspace_signup.sql` adds an auditable local trial start. New self-serve workspaces remain inaccessible with no running trial clock until the owner consumes the one-time email link, when verification atomically starts the 14-day period and first session. Entitlements expose local or Stripe-reconciled status, trial/current-period end, cancellation schedule, and provider state; the billing page shows that state. Fake-provider changes activate locally, while Stripe mode changes only after a valid subscription webhook. Backend and disposable-PostgreSQL acceptance—including the app-level Stripe contract journey—covers pending, active, expired, grace, suspended, recovery, and canceled timing. Remaining: approved credentialed provider smoke evidence, explicit proration/resubscription behavior, tenant deletion, and retention policy.
- `1.0.1` (verified self-serve signup): technically complete; approved live-provider validation pending. Public signup now provisions exactly one workspace, owner, membership, business-type pipeline, audit event, and pending trial in a serializable transaction protected by an advisory-locked idempotency key and secret-safe request fingerprint. It creates no session. Postmark or the fake local provider sends a one-time 24-hour verification link; only its atomic consumption records verification, starts the 14-day trial, and creates the owner session. Correct passwords remain gated until then. Same-key delivery failure is recoverable without duplicate tenants, same-name workspaces receive stable distinct slugs, conflicting retries fail closed, and resend is enumeration-safe with a persisted recipient cooldown. Signup has a stricter 3/client/hour limiter while verify/resend retain 10/client/minute. Migration `079_shared_public_rate_limits.sql` makes those and every other public budget atomic across API processes/restarts, retains only a one-way client digest, fails closed when enforcement is unavailable, cleans up expired rows in bounded batches, and emits alertable bounded decisions. Handler, privacy, concurrent PostgreSQL, email-template, onboarding PostgreSQL, frontend, and clean-schema browser acceptance cover the outcome. Existing users were expand-safely backfilled as verified; invited users become verified only when their setup token is consumed. Remaining external evidence: run one approved live Postmark signup; a production edge/WAF remains operational work rather than application signup logic.
- `1.0.1` (password recovery and active sign-ins): technically complete; approved live-provider validation pending. Migration `083_password_recovery.sql` adds one-hour token-digest and bounded delivery-health state without persisting raw tokens. Public requests accept eligible and nonexistent/unverified/disabled accounts identically, consume a shared 5/client/hour budget plus a persisted five-minute recipient cooldown, hide local fake links unless `GO_ENV` is explicitly development/test, and retain failed provider state for immediate token-rotating retry. Completion atomically changes the global credential, consumes the link, clears stale invite setup state, invalidates every server-side session, and records one audit event in every current/historical workspace membership. Login and reset now serialize on the global user row with statement-current session deletion; a forced PostgreSQL race proves a concurrent old-password login cannot survive. My Profile lists workspace/timing evidence for active sign-ins without collecting address/fingerprint data, protects the current session, and confirms audited one/all-other revocation even during hosted suspension. Migration `085_postmark_delivery_feedback.sql` adds a separate digest-only delivery key and provider reference for each identity email plus a durable callback ledger. The hidden-until-configured, Basic-Auth callback ignores unrelated shared-stream traffic, applies only an exact locked user/email/tenant/key match, deduplicates exact replays, rejects mutated event IDs, records no recipient/body/provider payload, makes reset bounces retryable, and globally suppresses system mail after a complaint. Aggregate reset and feedback gauges/alerts have no email/user/tenant labels. Handler/email/UI, disposable-PostgreSQL eligibility/cooldown/expiry/replay/delivery recovery, exact-attempt feedback/privacy/tamper, and session-management acceptance, WCAG, and clean-browser manual-plus-reset multi-session acceptance cover the outcome. Remaining external evidence: configure the approved webhook and run signup, invite, reset, bounce, and complaint through the live Postmark sender.
- `1.0.4` (limit enforcement): complete for the currently approved static hosted resources. In Stripe mode, migration `078_billing_capacity_reservations.sql` provides expiring tenant/resource claims for contacts, deals, and seats. Every capacity-increasing path—direct creation, user reactivation, contact import, public lead capture, single restore, and bulk archive rollback—reserves against committed usage plus concurrent claims and consumes the claim under the organization lock in the same transaction as its effect. Concurrent last-slot requests cannot both succeed; cancellation and expiry recover abandoned work; cross-tenant consumption fails; configured enforcement fails closed with `503`; and limits return `402 PLAN_LIMIT_REACHED` on private paths without exposing tenant state publicly. Fake/self-hosted mode uses no-op claims and remains unrestricted. Handler, module, migration, and disposable-PostgreSQL concurrency tests cover the boundary. Durable billing-period meters and any future approved quotas for messages/jobs/API/storage remain `1.0.5` work.
- `1.0.5` (explainable source-reconciled usage): partial. Migration `077_billing_usage_snapshots.sql` persists the exact Stripe period start, adds one retained observation per tenant/period, and adds bounded partial indexes for period sources. A member-scoped API, Plan & Billing view, and once-daily tenant job reconcile active seats/contacts/deals, sent outbound email, successful workflow runs and durable jobs within the provider period or UTC-month fallback, plus estimated current row bytes across tenant-scoped PostgreSQL base tables. The 15-minute bounded discovery loop uses one UTC-date idempotency generation, skips tenants with current evidence or active work, batches discovery, and sends failures through the leased retry/dead-letter/Operations replay path even while hosted writes are suspended. Reconciliation uses one repeatable-read transaction, dynamically quotes a bounded source list, retries bounded serialization conflicts, upserts the period snapshot, exposes source/unit/scope/basis/observation details, excludes internal usage snapshots and capacity claims from tenant storage, and never makes the measurement itself an unapproved quota. PostgreSQL tests cover boundaries, concurrent repeatability, fallback, retained update, daily schedule/dedupe/worker evidence, and foreign-tenant exclusion. Remaining: approve feature/tier/limit policy; apply the proven reservation/effect invariant to any newly approved message/automation/job/storage quota; add API-use metering only when a versioned external API exists; decide index and external-object storage accounting.
- `1.0.2` (provider seam + fake default): complete. Added a `billing.Provider` interface with a `FakeProvider` (no external calls; default for tests and self-hosted/development deployments), fail-closed unconfigured providers, and bounded provider telemetry. `BILLING_PROVIDER` selects the provider. Only `stripe` is managed; fake mode cannot enforce hosted trials, suspension, or limits against self-hosted data. `POST /api/billing/change-plan` remains a development seam, while Stripe mode requires the hosted flow described above. `example.env` documents the boundary. Remaining work belongs to the partial Stripe lifecycle outcome rather than the provider seam.
- `1.0.3`/`1.0.4` (foundation): complete for the approved capacity boundary. Added migration `014_billing_plans.sql` (`plan` column on `organizations` with a CHECK constraint); a `billing` module with an in-code Free/Starter/Pro/Enterprise capacity catalog and an entitlements service computing live usage; `GET /api/billing/plans` and `GET /api/billing/entitlements`; a centralized private-write/public-lead/worker lifecycle boundary; session-derived persistent read-only UI state; and a "Plan & Billing" settings page showing usage, hosted state, and enforced capacity. The catalog deliberately returns no feature grants until policy is approved, does not advertise unfinished API/SSO/automation/reporting outcomes, and treats Stripe Checkout as recurring-price authority instead of presenting the in-code price hint as a charge. Remaining: approve and enforce the plan-feature contract; SSO remains `1.0.7`.

Exit criteria:

- A prospect can sign up, start a trial, add a card, and be billed without operator involvement.
- Features and limits are enforced by plan in both API and UI.
- Tenants can be provisioned, suspended, and offboarded cleanly with data portability.

## Version 1.1.0 - Email And Communications

Status: in progress.

Goal: make Open CRM the place revenue teams live by bringing email into the CRM. This is the single highest-impact competitive gap.

Progress:

- Email provider seam established in `1.0.1`: `email.Provider` interface, in-process `FakeProvider` outbox (default), unconfigured stub for real providers, and `EMAIL_PROVIDER` selection. The transactional/outbound foundation (`1.1.1`) builds directly on this.
- **Email architecture decision:** system email (platform → CRM users: invites, password setup, notifications) goes through the shared transactional provider (Postmark). Customer-facing email (CRM user → their contacts) goes through each user's own mailbox (SMTP or its Gmail/Microsoft OAuth API), sending as the user — never the platform's transactional provider.
- `1.1.1` (system email via Postmark): technically complete; approved live-provider validation pending. The `PostmarkProvider` is selected by `EMAIL_PROVIDER=postmark` with `POSTMARK_SERVER_TOKEN` / `POSTMARK_FROM_EMAIL` / `POSTMARK_MESSAGE_STREAM` and is used only for workspace verification, invitations/setup, password recovery, and other platform mail. Migration `085_postmark_delivery_feedback.sql`, dedicated Basic Auth configuration, exact-attempt metadata correlation, duplicate/tamper detection, complaint suppression, a 400-day secret-safe ledger, admin delivery guidance, and aggregate alerts complete the local bounce/complaint path. Remaining external evidence is a controlled approved Postmark sandbox run for all three deliveries plus bounce and spam-complaint callbacks.
- `1.1.2` (per-user mailbox connections): complete (SMTP send; IMAP reserved for two-way sync). Added migration `017_user_email_accounts.sql`, an AES-256-GCM `secrets` cipher (`CREDENTIAL_ENCRYPTION_KEY`) so SMTP passwords are encrypted at rest, a `useremail` module (upsert/get-sanitized/delete/credentials/`SendAs`), an SMTP sender supporting implicit TLS / STARTTLS, and `GET/PUT/DELETE /api/me/email-account`. A "My Email" settings page lets users connect their mailbox; passwords are never returned. Backend (cipher, validation, handler) and frontend tests added.
- `1.1.2` (mailbox sync/OAuth-IMAP metadata foundation): complete. Added migration `023_user_email_sync_foundation.sql` with provider/auth method, sync enabled/status/cursor, last sync/error, OAuth token placeholders, and IMAP TLS metadata on `user_email_accounts`. `GET /api/me/email-sync/status` exposes sanitized sync state plus Google/Microsoft OAuth readiness. Settings > My Email now stores optional IMAP sync metadata while keeping SMTP send configuration unchanged.
- `1.1.2` (OAuth mailbox callback/token exchange): complete. Added signed OAuth state, Google/Microsoft authorization-start and callback endpoints (`POST /api/me/email-sync/oauth/{provider}/start`, `GET /api/me/email-sync/oauth/{provider}/callback`), provider token exchange, encrypted OAuth access/refresh token storage, and Settings > My Email connect buttons/status messages.
- `1.1.2` (mailbox sync readiness check): complete. Added sync-state update support on `useremail`, `POST /api/me/email-sync/check`, and a Settings > My Email readiness action. The check validates IMAP password metadata or OAuth connection state and records `ready`/`error` status for the future sync worker without importing messages yet.
- `1.1.2` (inbound mailbox storage foundation): complete. Added migration `024_inbound_email_messages.sql` to make `email_messages` direction-aware (`outbound`/`inbound`) with from-address, mailbox owner, provider message/thread IDs, and received timestamp metadata. `emailmessages.RecordInbound` provides idempotent inbound storage for future IMAP/Gmail/Graph ingestion, and the member mailbox view now reads combined inbound/outbound mailbox messages while preserving sender/admin detail access controls.
- `1.1.2` (generic IMAP manual sync runner): complete. Added an injectable `mailboxsync` service plus `POST /api/me/email-sync/run` so users can manually import recent generic IMAP inbox messages into the existing inbound `email_messages` store with duplicate provider IDs skipped, cursor/last-sync metadata updated, and sync errors recorded on the mailbox account. Settings > My Email now exposes a "Run sync now" action.
- `1.1.2` (Gmail API sync fetcher): complete. Google OAuth mailbox accounts now sync through the Gmail API using the stored encrypted access token, importing recent inbox messages as normalized inbound `email_messages` with Gmail message/thread IDs and the same matching/privacy path as IMAP. Automatic sync target discovery now includes Google OAuth accounts.
- `1.1.2` (Microsoft Graph sync fetcher): complete. Microsoft OAuth mailbox accounts now sync through Microsoft Graph using the stored encrypted access token, importing recent Inbox messages as normalized inbound `email_messages` with Graph/internet message IDs and conversation IDs. Automatic sync target discovery now includes Microsoft OAuth accounts.
- `1.1.2` (OAuth sync token refresh): complete. Mailbox sync now loads OAuth token expiry metadata, refreshes expired or missing access tokens with the stored refresh token before fetching Gmail/Graph messages, persists refreshed encrypted tokens, and reports provider refresh failures in sync state instead of failing opaquely during ingestion.
- `1.1.2` (OAuth mailbox delivery): technically complete; approved live-provider validation pending. Migration `086_oauth_mail_delivery.sql` records actual delegated scopes without breaking existing read-only connections. New connections request Gmail `gmail.readonly` + `gmail.send` or Microsoft delegated `Mail.Read` + `Mail.Send`; OAuth-only setup stores no SMTP password, legacy/read-only grants receive an actionable reconnect state, Gmail sends RFC 2822 MIME as base64url, and Microsoft Graph sends base64 MIME with exact `202` acceptance semantics. A tenant/user advisory lock serializes refresh-token rotation before send/sync, fresh tokens commit before the external effect, and the provider request is never automatically retried after it begins. Bounded send/refresh metrics, exact credential-free HTTP/MIME/deadline tests, handler/UI recovery tests, and disposable-PostgreSQL encryption/concurrent-refresh/cross-tenant acceptance cover the local outcome. Remaining external evidence is controlled send, refresh, sync, and reply ingestion through approved Google Workspace and Microsoft 365 test mailboxes.
- `1.1.3` (automatic inbound record matching): complete. Added migration `025_email_message_entity_links.sql` so one email message can be linked to multiple CRM records while preserving the legacy primary `entity_type`/`entity_id` fields. Inbound sync now matches the sender email to an active contact, that contact's linked companies, and related open deals before storing the message, so the same synced email appears in contact/company/deal histories. Existing sent/received messages with a primary entity are backfilled into the link table.
- `1.1.3` (email message privacy controls): complete. Added migration `026_email_message_visibility.sql` with `shared`/`private` visibility on `email_messages`; existing inbound messages are backfilled private while outbound remains shared. New synced inbound mailbox messages default private, and per-record histories only include private messages for org admins, the sender, or the mailbox owner.
- `1.1.2` (automatic mailbox sync worker): complete. Due generic IMAP/password, Google OAuth, and Microsoft OAuth accounts are discovered from a persisted `next_sync_at`, scheduled with a stable cycle idempotency key, and ingested through the durable PostgreSQL worker. Provider message IDs make repeated ingestion harmless, and success schedules the next cycle.
- `1.1.6` (send-from-record): updated to route through the sending user's mailbox (`SendAs`) instead of the shared provider; returns `EMAIL_ACCOUNT_REQUIRED` when the user has not connected their email. Merge-field rendering and contact-timeline logging unchanged.
- `1.1.4` (email outbox/log): complete. Added migration `018_email_messages.sql` and an `emailmessages` module recording every customer email send (status `sent`/`failed`, recipient, subject, body, linked record, sender). Sends from contacts are recorded automatically. `GET /api/email-messages` serves both the per-record history (`?entityType=contact&entityId=` — any member) and the org-wide log (no filter — admin only). Frontend: an admin "Email Log" settings page and a lazy-loaded email history on the contact detail. Backend handler tests and a frontend page test added. Live server configured with Postmark (system mail) + `CREDENTIAL_ENCRYPTION_KEY` (per-user SMTP) and verified healthy.
- `1.1.2` (admin sets member mailbox): complete. Org admins/owners can connect, view, and remove a team member's mailbox via `GET/PUT/DELETE /api/users/{id}/email-account` (membership-verified before write). Frontend: a "Set up email for a member" panel on the Users settings page with a member selector. Backend handler tests (admin gating, non-member rejection) and a frontend flow test added.
- `1.1.6` (send-from-company/deal): complete. Added `POST /api/companies/{id}/email` and `POST /api/deals/{id}/email`, both sending through the current user's connected mailbox, rendering record-specific merge fields, recording to `email_messages`, and adding a note to the source record. Frontend: shared record email composer on company and deal detail pages with lazy template/history loading. Backend company/deal send tests and a frontend company-send flow test added.
- `1.1.6` (personal mailbox/sent view): complete. Added member-safe `GET /api/me/email-messages` backed by `emailmessages.ListBySender`, plus a primary-nav "Mailbox" page showing the current user's sent CRM emails and links back to source contacts/companies/deals. Backend scoping test and frontend route test added.
- `1.1.6` (email message detail): complete. Added `GET /api/email-messages/{id}` with admin-or-sender access control so users can inspect full body/error detail without exposing other users' message bodies to members. The Mailbox and admin Email Log now include "View details" panels. Backend access-control tests and frontend detail tests added.
- `1.1.4` (open tracking foundation): privacy/retention contract complete locally; provider/pilot validation pending. Migration `091_email_engagement_tracking_privacy.sql` leaves collection off per one-to-one send unless the sender explicitly confirms authorization, binds the acknowledgement and expiry to the message, ends collection after 90 days, and immediately expires legacy rows with no acknowledgement. Expired observations disappear from APIs before cleanup; pixels become indistinguishable no-ops. An immediate/hourly 500-row `SKIP LOCKED` pass transactionally scrubs open tokens/counts/timestamps with aggregate metrics, stale/error alerts, a runbook, and no client-address/user-agent/referrer retention. Handler/UI/migration/metrics and disposable-PostgreSQL expiry/replay acceptance cover the local result.
- `1.1.4` (click tracking foundation): privacy/retention contract complete locally; provider/pilot validation pending. Random 256-bit tokens and server-stored validated HTTP(S) targets remain behind separate fail-closed shared public limits and no-referrer/no-index responses. The same explicit 90-day window gates collection. At expiry and after observation scrubbing, old delivered links continue to redirect without incrementing message or link counters; target mappings remain only to preserve that recipient-facing behavior. UI copy labels signals approximate, exposes off/active/expired state truthfully, and a focused policy document records consent responsibility and retained data. Real mail-scanner/privacy-proxy behavior and pilot policy wording remain validation work.
- `1.1.5` (templates/snippets/merge fields): complete. Added an authenticated merge-field catalog for contact/company/deal email tokens, reusable organization-scoped email snippets, Settings > Email Templates management for templates and snippets, and record-composer snippet insertion so users do not need to memorize placeholders or retype common fragments.
- `1.1.8` (sequence definition foundation): complete. Added migration `021_email_sequences.sql` for organization-scoped sequence metadata and ordered steps, a backend `emailsequences` module with admin/writer CRUD endpoints (`/api/email-sequences`), and a Settings > Email Sequences page for drafting cadence definitions. This intentionally does not enroll contacts, schedule sends, or detect replies yet.
- `1.1.8` (sequence enrollment/schedule-state foundation): complete. Added migration `022_email_sequence_enrollments.sql` for active/paused/completed/cancelled contact enrollments with `current_step_order` and `next_send_at`, backend list/create/cancel endpoints (`/api/email-sequence-enrollments`), and a contact-detail Sequences panel for enrolling contacts. This stores scheduler state only; automated sends do not run yet.
- `1.1.8` (sequence reply detection foundation): complete. Inbound synced email can stop future sequence sends; the outcome-evidence slice below now requires exact tenant, active contact email, enrolling mailbox, accepted-send, and received-time qualification before completing active/paused enrollments.
- `1.1.8` (sequence send worker foundation): complete. Enrollment and each subsequent step now enqueue transactionally on the durable PostgreSQL runner. A delivery ledger snapshots the recipient/content before SMTP, advances exactly once, and quarantines an ambiguous provider result as `uncertain` rather than silently resending. Admins can confirm it sent without SMTP or explicitly approve one retry; both decisions are atomic and audited.
- `1.1.8` (sequence approval and pause controls): complete. Migration `087_email_sequence_approvals.sql` expand-safely pauses legacy active definitions and adds revision-bound approval evidence. Draft/edit APIs cannot activate content; admins/owners approve the exact revision, any writer can pause, active or historically enrolled content cannot be mutated/deleted, and contact enrollment lists/accepts only active current-revision approvals. The durable runner locks and rechecks the enrollment plus definition immediately before claiming a provider attempt, while a paused job defers without consuming attempts; UI copy discloses that an attempt already claimed before pause may finish. Approval/pause audit events plus handler, focused UI, unit, and disposable-PostgreSQL tenant/race/history acceptance cover the local outcome. Customer-mail outcome analytics, deliverability controls, and approved live-provider evidence remained for the following slices.
- `1.1.8` (sequence outcome evidence and safe exits): complete. Migration `088_email_sequence_outcomes.sql` stores future finish/reply/suppression reasons and exact inbound reply evidence while leaving historical completed rows explicitly unclassified. Inbound mail exits active or paused enrollments only for the matched contact in the same tenant, sent to the enrolling user's mailbox, after at least one provider-accepted delivery; duplicate, cross-tenant, wrong-sender, wrong-mailbox, and pre-send inbound paths are covered. Suppression exits immediately instead of scheduling the remaining cadence, and a recovered `sending` ledger becomes operator-reviewable `uncertain` without another provider call. Tenant-scoped API listings expose enrollment, active/paused, accepted-send, reply, finish, suppression, cancellation, queued, and recovery counts; the compact UI shows the pilot-facing enrollment, accepted, reply, finish, suppression, and review subset. Disposable-PostgreSQL, runner unit/integration, migration, and focused UI tests cover the slice. Provider acceptance is not proof of delivery; the following correlation slices add later reply and delivery-feedback evidence, while approved live-provider evidence remains before capability promotion.
- `1.1.8` (hosted sequence send safety): complete as a provisional operating boundary. A Stripe-hosted runtime now atomically consumes both a shared tenant 24-hour budget and sender one-hour budget in PostgreSQL after approval/duplicate/suppression checks but before claiming a provider attempt. Defaults are 1,000 and 100 respectively, configurable only as positive bounded integers; an invalid hosted policy prevents startup. Exhaustion releases the durable job until the denied window resets without spending an execution attempt, claiming the delivery, or calling the mailbox provider. Group rejection rolls back every budget reservation, opaque tenant/sender keys remain hashed, and stable lock ordering plus concurrent disposable-PostgreSQL runner acceptance cover multiple API instances. These are operator safety caps rather than pricing entitlements; fake/self-hosted billing remains unrestricted. Validate thresholds with pilot and provider-reputation evidence before treating them as settled policy.
- `1.1.8` (header/provider-thread-qualified replies): complete locally; approved live-provider evidence pending. Migration `089_email_message_correlation.sql` adds bounded correlation evidence without guessing historical rows. The durable runner generates and stores an opaque RFC `Message-ID` before claim/provider effects, sends the same ID through SMTP/Gmail/Microsoft MIME, atomically stores Gmail message/thread receipts with acceptance, and records the identifiers on ordinary successful customer-email logs. IMAP and Gmail raw MIME ingest normalized `Message-ID`, `In-Reply-To`, and `References`; Microsoft Graph retrieves raw MIME and retains its conversation identifier. A reply now exits a cadence only for an exact header reference or a provider thread resolving to exactly one eligible enrollment under the existing tenant/contact/mailbox/time checks. Uncorrelated messages and ambiguous shared threads fail closed; provider keys never become metrics labels and are removed from portable exports. Header injection/bounds/deduplication, exact provider HTTP/MIME receipts, service mapping, migration safety, and disposable-PostgreSQL exact/wrong/ambiguous/duplicate/cross-tenant paths cover the local outcome. Remaining: retain approved Gmail/Microsoft/SMTP send-and-reply evidence.
- `1.1.8` (customer-mail delivery feedback and safe exits): complete locally; approved live-provider evidence pending. Migration `090_customer_email_feedback.sql` preserves provider acceptance and records later `bounced`/`complaint` outcomes on outbound logs and sequence deliveries, adds a 400-day tenant-scoped feedback ledger, and adds exact accepted-outbound RFC uniqueness per mailbox while permitting an earlier failed attempt retained under the same durable ID. IMAP, Gmail, and Microsoft raw MIME ingestion recognizes only RFC 3464 `multipart/report; report-type=delivery-status` terminal `failed`/`5.x` recipients and RFC 5965 `feedback-report` complaints; temporary/delayed DSNs and ordinary mail are ignored. State changes require exactly one same-tenant, same-mailbox, post-acceptance opaque RFC message match with no recipient disagreement. A match suppresses the recipient, stops active/paused cadences, links inbound evidence, records a system audit event, and gives complaints precedence over bounces; unmatched evidence remains durable and unapplied. Email Log and sequence analytics distinguish accepted delivery from later outcomes, portable exports strip internal feedback links, aggregate customer bounce/complaint/unapplied metrics have alerts/runbook coverage, and bounded retention covers both feedback ledgers. Parser/provider/unit/UI/migration tests plus disposable-PostgreSQL foreign-tenant, wrong-mailbox, wrong-recipient, missing/unmatched, duplicate, direct-email, explicit retry, sequence-exit, and bounce-to-complaint paths cover the local result. Remaining: retain controlled live bounce/complaint evidence for each approved provider and investigate provider-specific authentication/reputation evidence because DSN/ARF formats alone are not universal sender authenticity.
- `1.1.7` (unsubscribe/suppression foundation): complete locally; approved provider-signing evidence pending. Organization-scoped suppression and HMAC-signed links now use a scanner-safe read-only confirmation `GET` plus an exact, non-redirecting, idempotent RFC 8058 `POST`. Direct CRM and sequence sends retain text/HTML fallbacks and add bounded HTTPS `List-Unsubscribe` plus `List-Unsubscribe-Post` headers through the common SMTP/Gmail/Microsoft MIME path. Suppression evidence is monotonic (`complaint` > `manual` > `unsubscribed` > `bounce`), the public route remains available during hosted suspension, and handler/provider/disposable-PostgreSQL tests cover malformed forms, tenant scope, replay, and precedence. Open CRM cannot prove that a downstream provider preserves and DKIM-signs both headers, so raw delivered-header evidence remains required for each approved provider. Bulk list selection, campaign UI, jurisdiction-specific consent policy, and richer compliance reporting remain deferred to the marketing/bulk-email slice.
- `1.1.9` (shared team inbox and threaded replies): complete locally, but not promoted beyond foundation pending approved live-provider and pilot evidence. Shared inbound assignment/open/closed coordination retains reversible explicit-disclosure privacy, optimistic concurrency, transactional actor/assignee/tenant revalidation, immediate access removal, and content-free audit evidence. Migration `092_email_threaded_replies.sql` adds a tenant-scoped stable conversation root plus a durable request-key/hash reply ledger. Mailbox and Team Inbox now render chronological threads and reply only through the acting teammate's own connected mailbox—never by impersonating the original receiver. Source privacy is inherited, RFC `Message-ID`/`In-Reply-To`/`References` and provider threads are preserved, and reply tracking is off. The provider boundary persists immutable content/header/sender evidence before one claim, rechecks current sender identity and suppression immediately before effect, atomically finalizes provider acceptance with an outbound thread message, and refuses automatic replay. OAuth/SMTP/finalization ambiguity and five-minute interrupted claims become operator-visible `uncertain`; only the original actor can retry, owners/admins can confirm sent or mark not sent, and each resolution is audited. An immediate/minutely bounded `SKIP LOCKED` recovery pass, aggregate gauges, validated alerts, and a runbook cover stuck/uncertain work. Handler/MIME/UI/metrics/migration tests and disposable-PostgreSQL tenant/role/idempotency/claim/finalize/recovery/correlation acceptance cover the local result. Remaining: approved Gmail/Microsoft/SMTP raw-header/thread, suppression, forced-ambiguity recovery, and pilot policy evidence.

Candidate slices:

- `1.1.1` Outbound transactional/CRM email infrastructure via a provider (Postmark/SendGrid) with domain auth (SPF/DKIM/DMARC) and bounce/complaint handling.
- `1.1.2` Two-way mailbox sync via Gmail and Microsoft 365 OAuth (and generic IMAP/SMTP) with per-user connection.
- `1.1.3` Automatic email logging to matching contacts/companies/deals with privacy controls (shared vs. private).
- `1.1.4` Email open and link-click tracking with per-message and aggregate engagement.
- `1.1.5` Email templates, snippets, and merge fields: complete.
- `1.1.6` One-to-one send from record pages and a connected-inbox view.
- `1.1.7` Bulk/mass email with list selection, unsubscribe management, and CAN-SPAM/GDPR compliance footers. Suppression/unsubscribe primitives are complete; bulk campaign UX remains future work.
- `1.1.8` Email sequences / cadences: multi-step automated outreach with conditions and reply detection.
- `1.1.9` Shared team inboxes and assignment for collaborative reply workflows: locally complete with own-mailbox threaded replies and explicit recovery; live-provider/pilot evidence pending.

Exit criteria:

- Users send and receive email inside the CRM with automatic activity logging.
- Sequences and bulk email run reliably with compliance and deliverability safeguards.

## Version 1.2.0 - Telephony, SMS, And Meeting Scheduling

Status: in progress.

Goal: add real-time communication channels and remove scheduling friction.

Progress:

- `1.2.1` (click-to-call and call logging foundation): complete. Added persistent organization-scoped `call_logs`, a telephony provider seam with fake/default and unconfigured real-provider behavior, API endpoints to list/start/complete calls, contact-detail call start/logging UI, and call history display. Real Twilio call origination, inbound routing, recording, SMS, and scheduling remain future slices.
- `1.2.2` (inbound call logging foundation): complete. Added manual inbound call logging for contacts using the shared call log store, including voicemail/missed-call dispositions, notes, and `call.logged`/`call.failed` activity timeline entries. Real carrier webhook routing, voicemail media capture, and assignment rules remain future provider-specific work.
- `1.2.3` (call recording controls foundation): complete. Added recording metadata fields on call logs, consent state, retention-until tracking, recording delete markers, a writer-only recording controls API, contact-detail controls for recording links/consent/retention, and activity timeline entries for recording updates/deletes. Real provider recording capture, media storage, consent prompts, and automated retention deletion remain future provider/ops work.
- `1.2.4` (SMS foundation): complete. Added organization-scoped SMS message history, phone-number opt-out suppressions, a fake/default SMS provider seam using the telephony provider selector, contact-detail SMS composer with starter templates and merge-field rendering, manual inbound SMS logging with STOP-style opt-out detection, manual opt-out controls, and SMS activity timeline entries. Real Twilio SMS send/receive webhooks, carrier delivery receipts, reusable managed SMS template CRUD, and bulk texting remain future slices.
- `1.2.5` (calendar meeting foundation): complete. Added organization-scoped calendar events, user availability blocks, a fake/default calendar provider seam, APIs for listing/scheduling/cancelling meetings and current-user availability, contact-detail meeting scheduling/history/cancellation UI, and meeting activity timeline entries. Real Google/Microsoft OAuth calendar event sync, free/busy imports, external attendee invites, webhooks, and conflict resolution remain future provider-specific work.
- `1.2.6` (booking links foundation): complete. Added organization-scoped calendar booking links with slugs, duration/buffer/timezone metadata, active/inactive state, selected host members, owner vs round-robin assignment mode, authenticated APIs for listing/creating/updating links, and a development-only Settings > Booking Links UI for managing links and weekly availability. Production navigation/bundles deliberately omit that management route because public booking pages, guest self-scheduling, slot generation, real round-robin assignment, external calendar conflict checks, reminders, and rescheduling/cancellation flows remain future slices.
- `1.2.7` (meeting reminders foundation): complete. Scheduled events transactionally enqueue durable reminder jobs; delivery locks the tenant reminder row, creates one in-app `meeting.reminder` notification plus activity entry, and makes duplicate/replayed jobs no-ops. Cancellation skips pending reminders. Customer-facing email/SMS reminders, configurable offsets, guest preferences, provider notifications, and delivery analytics remain future slices.

Candidate slices:

- `1.2.1` Click-to-call and call logging via a provider (Twilio) with disposition and notes: foundation complete.
- `1.2.2` Inbound call routing, voicemail, and call activity timeline entries: manual logging foundation complete.
- `1.2.3` Call recording with consent controls and retention policy: metadata/control foundation complete.
- `1.2.4` SMS send/receive with templates and opt-out handling: contact-level fake-provider foundation complete.
- `1.2.5` Calendar two-way sync (Google/Microsoft) for meetings and availability: meeting/availability foundation complete.
- `1.2.6` Meeting scheduler / booking links (Calendly-style) with round-robin and team availability: authenticated booking-link/availability foundation complete.
- `1.2.7` Meeting reminders and automatic activity logging: in-app reminder/activity foundation complete.

Exit criteria:

- Users can call, text, and book meetings from the CRM with logged outcomes.
- Communication compliance (consent, opt-out, retention) is enforced.

## Version 1.3.0 - Sales Acceleration And CPQ

Status: in progress.

Goal: support full sales execution from quote to close, extending the `0.6.x` sales workflow.

Progress:

- `1.3.1` (product/service catalog foundation): complete. Added organization-scoped catalog items with product/service type, SKU, description, unit price, currency, unit, active/inactive state, authenticated APIs for listing/creating/updating/archiving items, and a Settings > Product Catalog UI. Deal line items, quote totals, discounts, taxes, multi-currency exchange rates, and proposal generation remain future slices.
- `1.3.2` (deal line items foundation): complete. Added organization-scoped deal line items tied optionally to catalog items, quantity/unit pricing, per-line discounts, tax rates, calculated line totals, deal-level subtotal/discount/tax/total summaries, an authenticated API to replace deal line items, automatic deal value recalculation from saved line items, line-item activity logging, and a deal detail line-item editor. Quote/proposal documents, tax rules, approval workflows, and multi-currency conversion remain future slices.
- `1.3.3` (quote/proposal PDF foundation): complete. Added a branded quote/proposal PDF download generated from current deal details, saved line items, and calculated totals, plus a deal detail download action. Quote persistence/versioning, approval workflows, customer sending, e-signature, and terms/template management remain future slices.
- `1.3.4` (e-signature status tracking foundation): complete. Added organization-scoped deal signature requests with signer identity, native tracking provider metadata, quote filename, draft/sent/signed/declined/voided statuses, lifecycle timestamps, authenticated APIs for creating requests and updating status, activity logging, and deal detail signature tracking UI. Actual signing ceremonies, provider webhooks, customer delivery, audit certificates, and reusable terms/templates remain future slices.
- `1.3.5` (multiple pipelines foundation): complete. Added organization-scoped deal pipelines, backfilled existing stages into a default pipeline, scoped stage uniqueness by pipeline, exposed authenticated APIs for listing/creating pipelines, copied default stage templates into new pipelines, and added pipeline metadata/filtering to deal list/detail/export flows. The later `0.6.1` outcome moved creation into admin settings and completed rename/default plus custom stage classification/reordering; team/business-unit ownership rules and per-pipeline permissions remain future scope only if pilot evidence requires them.
- `1.3.6` (quotas and forecasting dashboard foundation): complete. Added organization-scoped per-user sales quota records by period, admin quota upsert API, current-quarter forecast calculations using won revenue plus stage-weighted open pipeline, team/member attainment and coverage metrics, and dashboard quota editing/forecast display. Forecast categories, custom stage probabilities, quota history, rollups by team/business unit, and advanced forecast analytics remain future slices.
- `1.3.7` (multi-currency exchange-rate foundation): complete. Added organization base currency settings, manual organization exchange-rate records, admin API/UI for saving rates, and base-currency conversion for deal-list, dashboard pipeline, quota, and weighted forecast rollups while preserving per-record deal/catalog currencies. Automated FX providers, historical rate selection beyond latest manual rates, quote-level FX disclosures, and realized gain/loss accounting remain future slices.
- `1.3.8` (immutable quote-version convergence): complete for finalization, connected-mailbox delivery, customer access, and explicit receipt. A writer can finalize saved line items with recipient, validity, and terms; one transaction locks the deal, allocates a version, snapshots all commercial identity/line/total fields, stores the exact PDF bytes and SHA-256, and records activity/audit evidence. Hashed actor-scoped idempotency collapses concurrent retries and rejects changed requests. Delivery persists an exact sender/recipient/message intent, stable RFC Message-ID, digest-only expiring customer token, and suppression check before the SMTP/Gmail/Microsoft boundary; provider acceptance records the shared outbound email/activity/audit transactionally, while ambiguous effects are quarantined for sender/admin Sent-folder resolution and never automatically retried. Preparation and claims lock and revalidate active sender membership first; user disable/revocation cannot race a late intent, fails prepared intent, and quarantines already claimed sends as uncertain in the same lifecycle transaction. Public preview/PDF access is bounded/no-store, counters are explicitly approximate, and idempotent receipt evidence is explicitly not approval or signature. Startup/minutely recovery, aggregate metrics/alerts, portable business evidence without tokens/correlation hashes, handler/UI/migration/disposable-PostgreSQL recovery/expiry/tenant tests, and the PostgreSQL browser journey through a real SMTP sandbox plus customer WCAG scan cover the boundary. Approval, reusable templates, active expiration workflow, quote-level FX disclosure, approved live-mailbox/pilot evidence, and legal signing remain future slices.

Candidate slices:

- `1.3.1` Product/service catalog with pricing, SKUs, and currency: foundation complete.
- `1.3.2` Deal line items, discounts, taxes, and totals: foundation complete.
- `1.3.3` Quote/proposal generation with branded PDF output: foundation complete.
- `1.3.4` E-signature flow (native or DocuSign/Dropbox Sign integration) with status tracking: foundation complete.
- `1.3.5` Multiple pipelines per team/business unit (extends `0.6.1`): foundation complete.
- `1.3.6` Quotas, goals, and team forecasting dashboards (extends `0.6.2`): foundation complete.
- `1.3.7` Multi-currency support with exchange-rate handling: foundation complete.
- `1.3.8` Immutable quote finalization, connected-mailbox delivery, customer access, and explicit non-signature receipt: complete; approval/legal signing remains separate.

Exit criteria:

- A rep can build a quote from a catalog, send it for signature, and convert it to a closed deal.
- Managers can set quotas and track weighted forecast against them.

## Version 1.4.0 - Marketing And Lead Generation

Status: in progress.

Goal: capture and nurture demand, not just manage existing relationships.

Progress:

- `1.4.1` (embeddable lead capture forms foundation): complete and hardened at the application boundary. Organization-scoped forms use generated public IDs, map standard fields into attributed CRM lead contacts, preserve raw business payloads, and expose admin APIs/UI plus an embed snippet. Admins configure the exact consent statement; every public write now requires a 256-bit, digest-only, organization/form-bound challenge that is usable after two seconds and expires after 30 minutes. Submission re-locks the active form/challenge, snapshots consent text/time, creates the contact/activity/submission, and consumes the challenge transactionally. Exact request retries return the original effect while changed, premature, expired, cross-form, and cross-tenant attempts fail closed. New submissions no longer store raw client addresses or user agents. The dedicated Contacts workspace is again present in normal navigation so pre-client leads can be found and assigned. Separate shared PostgreSQL challenge/write budgets, bounded outcomes/alerts, handler and disposable-PostgreSQL concurrency/tenant tests, and the Chromium hosted-page capture/assignment plus WCAG journey cover the local result. Custom dynamic mapping, spam-review UX, edge/WAF reputation controls, automatic scoring/routing, analytics, and pilot validation remain future slices.
- `1.4.2` (hosted landing pages foundation): complete. Added organization-scoped landing pages tied to existing lead capture forms, globally unique public slugs, active/inactive state, light/blue/dark themes, authenticated APIs for listing/creating/updating pages, a public page lookup endpoint, a Settings > Landing Pages UI, and a public `/lp/:slug` frontend route that renders marketing copy plus the challenge- and consent-bound lead form flow. The clean PostgreSQL browser journey creates the definition and page, submits from an isolated public browser with UTM evidence, scans WCAG A/AA, then finds and assigns the lead. Rich page templates, drag-and-drop editing, custom domains, SEO metadata controls, conversion analytics, A/B tests, campaign enrollment, and pilot evidence remain future slices.
- `1.4.3` (lead source and UTM/campaign attribution tracking foundation): complete. Added first-touch lead source, source URL, and standard UTM fields on contacts created from public lead submissions; persisted attribution columns on raw lead submissions; derived attribution from submitted source URLs and explicit embed fields; surfaced attribution on contact detail/list APIs, contact detail UI, hosted landing page submissions, lead form embed snippets, and contacts CSV exports. Full campaign objects, attribution reports, multi-touch history, and automated routing/scoring remain future slices.
- `1.4.4` (list segmentation and dynamic/saved audiences foundation): complete. Added organization-scoped lead audience definitions with reusable filters, dynamic member-count previews against contacts, authenticated APIs for listing/creating/updating/previewing audiences, and a Settings > Audiences UI for saving source/campaign/status/email-availability segments. Campaign enrollment, audience member drill-down, exclusion rules, advanced boolean logic, and scheduled snapshots remain future slices.
- `1.4.5` (marketing email campaigns with scheduling and per-campaign analytics foundation): complete as a hidden foundation. Added organization-scoped marketing email campaign definitions tied to active saved audiences, stored schedule/status metadata, captured audience recipient-count snapshots, persisted per-campaign analytics counters, exposed authenticated APIs for listing/creating/updating campaigns, and retained a development-only Settings > Email Campaigns UI. Production navigation/bundles omit it until bulk recipient expansion, mailbox delivery, unsubscribe enforcement at send time, open/click attribution into campaign counters, send approvals, and campaign reports are complete.
- `1.4.6` (drip/nurture campaigns built on the sequence engine foundation): complete as a hidden foundation. Added organization-scoped nurture campaign plans that bind active saved audiences to existing email sequences, validate active campaigns against active sequences, snapshot eligible audience counts, expose authenticated APIs for listing/creating/updating nurture campaigns, and retain a development-only Settings > Nurture Campaigns UI. Production navigation/bundles omit it until automatic audience enrollment, enrollment refresh scheduling, suppression-aware bulk launch approvals, per-nurture performance rollups, and reply/exit rules are complete.
- `1.4.7` (rule-based lead scoring and routing/assignment foundation): complete. Added organization-scoped scoring rules over contact status, source, UTM, title, email, phone, and email-domain signals; persisted contact score/grade/scored-at metadata; exposed admin APIs for scoring-rule management plus a contact evaluation endpoint; routed unassigned contacts to rule-selected team members; logged lead scoring activity; surfaced scoring in contact list/detail; and added a Settings > Lead Scoring UI. Automatic scoring on form submission, bulk rescoring, rule simulation over audiences, SLA queues, and round-robin assignment remain future slices.
- `1.4.8` (lead capture from chat/website widget foundation): complete. Added organization-scoped website widget definitions tied to existing lead capture forms, stable public widget IDs, light/blue/dark themes, bottom-left/bottom-right/inline embed positions, authenticated APIs for listing/creating/updating widgets, a public widget lookup endpoint, a `/widget/:publicId` frontend renderer, and a Settings > Website Widgets UI with iframe embed snippets. Widget submissions now use the same delayed one-time challenge and exact consent statement as hosted pages. Live chat, bot conversation trees, agent handoff, widget analytics, edge reputation controls, automatic scoring/routing on submission, and custom script-loader embeds remain future slices.

Candidate slices:

- `1.4.1` Embeddable web forms with field mapping to CRM records: foundation complete.
- `1.4.2` Hosted landing pages with form capture and basic templates: foundation complete.
- `1.4.3` Lead source and UTM/campaign attribution tracking: foundation complete.
- `1.4.4` List segmentation and dynamic/saved audiences: foundation complete.
- `1.4.5` Marketing email campaigns with scheduling and per-campaign analytics: foundation complete.
- `1.4.6` Drip/nurture campaigns built on the sequence engine (`1.1.8`): foundation complete.
- `1.4.7` Rule-based lead scoring and routing/assignment: foundation complete.
- `1.4.8` Lead capture from chat/website widget (optional): foundation complete.

Exit criteria:

- New leads can be captured, attributed, scored, routed, and nurtured automatically.
- Marketing activity ties cleanly into the sales pipeline.

## Version 1.5.0 - Workflow Automation Engine

Status: in progress.

Goal: let admins automate CRM work without code — a core differentiator across all SaaS CRMs.

Convergence reconciliation (2026-07-19): the broad definition, condition,
action, timing, approval, and run artifacts described below remain a stored/API
foundation, but their former general-purpose visual editor is no longer exposed
in normal product navigation. Version `0.6.3` replaced that surface with a
bounded deal follow-up task editor and implemented only that subset's
transactional, idempotent runtime. Conditions, other targets/actions, scheduled
or approval steps, provider dispatch, and general retry orchestration still do
not execute and must not be inferred complete from the historical foundation
entries.

Progress:

- `1.5.1` (trigger model foundation): complete. Added organization-scoped workflow automation definitions with typed triggers for record created/updated, deal stage changed, date reached, form submitted, inbound email, and webhook events; persisted target entity and JSON trigger config metadata; exposed authenticated APIs for listing/creating/updating automation trigger definitions; and added a Settings > Automations UI. Trigger detection, condition evaluation, action execution, scheduling/delays, webhooks, visual editing, run history, loop protection, and retry/idempotency remain future slices.
- `1.5.2` (condition/branching foundation): complete. Extended workflow automation definitions with all/any condition logic and validated condition arrays over contact, company, deal, task, form-submission, inbound-email, and webhook fields; added a pure condition evaluator for equality, inequality, contains, exists, greater-than, and less-than checks; exposed conditions through the workflow automation APIs; and expanded Settings > Automations with condition editing. Visual branching, nested condition groups, trigger-time record hydration, action execution, schedule/delay handling, run history, and retry/idempotency remain future slices.
- `1.5.3` (action library foundation): complete. Added ordered workflow action definitions with validation for update field, create task, send email, send SMS, assign owner, add to sequence, call webhook, and notify action types; persisted action arrays on workflow automations; exposed actions through the workflow automation APIs; and expanded Settings > Automations with action editing. Action execution, provider dispatch, trigger-time record hydration, visual action cards, delays/schedules, run history, and retry/idempotency remain future slices.
- `1.5.4` (visual workflow builder foundation): complete. Added a guided Settings > Automations builder that visualizes trigger, condition, and action steps; provides target-aware condition field/operator controls; adds/removes condition chips and ordered action cards; and keeps advanced JSON editing as the persisted source of truth. Drag/drop layout, nested branches, full action-specific forms, record hydration previews, execution, delays/schedules, run history, and retry/idempotency remain future slices.
- `1.5.5` (scheduled/time-delay action foundation): complete. Added validated per-action timing metadata for relative `delayMinutes` and absolute `scheduledAt` plans, normalized scheduled action times to UTC, exposed timing controls in the visual builder, and added a pure planned-action-time helper for the future background runner. Trigger detection, action queue persistence, due-action selection, provider dispatch, run history, loop protection, and retry/idempotency remain future slices.
- `1.5.6` (approval/human-in-the-loop action foundation): complete. Added a validated `request_approval` workflow action definition with approval name, approver role, and message metadata; normalized approver roles for admin, owner, and record-owner routing; exposed approval action fields in the visual builder; and kept advanced JSON editing as the persisted source of truth. Approval queues, runtime pause/resume behavior, approver notifications, audit history, and execution gating remain future slices.
- `1.5.7` (automation run history/retry foundation): complete. Added persistent workflow automation run records with per-automation idempotency keys, statuses, trigger payload metadata, condition results, action progress, retry count, error text, and UTC timestamps; exposed recent run history through an authenticated API and Settings > Automations panel; and added service helpers for future idempotent run recording and terminal completion. Trigger detection, action queue execution, provider dispatch, loop protection, retry scheduling, and action-level attempt history remain future slices.

Candidate slices:

- `1.5.1` Trigger model: record created/updated, stage changed, date reached, form submitted, inbound email, webhook: foundation complete.
- `1.5.2` Condition/branching engine with AND/OR rules over record fields: foundation complete.
- `1.5.3` Action library: update field, create task, send email/SMS, assign owner, add to sequence, call webhook, notify: foundation complete.
- `1.5.4` Visual workflow builder UI: foundation complete.
- `1.5.5` Scheduled and time-delay actions on the background job runner (`0.9.3`): foundation complete.
- `1.5.6` Approval steps and human-in-the-loop actions: foundation complete.
- `1.5.7` Automation run history, error handling, and safe retry/idempotency: foundation complete.

Exit criteria:

- Admins can build multi-step automations visually and audit their runs.
- Automations execute reliably with guardrails against loops and duplicate actions.

## Version 1.6.0 - Reporting And Analytics

Status: in progress.

Goal: move from fixed reports to a self-service analytics layer.

Progress:

- `1.6.1` (custom report builder foundation): complete as a hidden foundation. Added organization-scoped custom report definitions for contacts, companies, deals, and tasks with validated selected fields, filters, grouping, and aggregation metadata plus authenticated list/create/update APIs. The builder UI remains development-only and is excluded from production navigation and bundles; production Reports contains only executable fixed reports. Runtime report query execution, chart rendering, dashboards, sharing permissions, scheduled delivery, exports, and analytics read-model/performance work remain future slices.
- `1.6.2` (chart/visualization type foundation): complete as hidden metadata/editor work. Extended custom report definitions with validated visualization metadata for table, bar, line, funnel, pie, and KPI views and exposed it through authenticated APIs plus the development-only builder. Production navigation and bundles omit the non-executable selector until runtime chart rendering, report query execution, dashboard widgets, shared/personal layouts, and export rendering are complete.

Candidate slices:

- `1.6.1` Custom report builder (choose object, fields, filters, grouping, aggregation): foundation complete.
- `1.6.2` Chart/visualization types (table, bar, line, funnel, pie, KPI): foundation complete.
- `1.6.3` Configurable dashboards with draggable widgets, shared and personal.
- `1.6.4` Pipeline/funnel conversion analytics and velocity metrics.
- `1.6.5` Revenue, activity, and cohort analytics with date-range and owner filters.
- `1.6.6` Scheduled report delivery (email export) and export to CSV/PDF.
- `1.6.7` Query performance and read-model strategy for analytics at scale.

Exit criteria:

- Users build and share custom reports and dashboards without engineering help.
- Analytics queries remain performant on large tenant datasets.

## Version 1.7.0 - AI And Intelligence

Status: planned.

Goal: reach AI parity with modern CRMs, where AI assistance is now a primary buying criterion. Built on hosted LLM/inference providers with strict tenant data isolation.

Candidate slices:

- `1.7.1` AI email and message drafting/reply with tone and context from the record.
- `1.7.2` Call and meeting transcription plus AI summaries and action items.
- `1.7.3` Record and thread summarization (account/deal recaps).
- `1.7.4` Predictive lead and deal scoring beyond rules (model-assisted).
- `1.7.5` Next-best-action and at-risk-deal recommendations.
- `1.7.6` Natural-language / semantic search across CRM data.
- `1.7.7` CRM copilot: conversational assistant that can answer questions and take actions over tenant data.
- `1.7.8` Data enrichment (company/contact firmographics) via providers.
- `1.7.9` AI governance: per-tenant opt-in/out, no-train guarantees, prompt/data isolation, cost metering.

Exit criteria:

- Users get AI drafting, summarization, scoring, and a copilot grounded in their own data.
- AI usage is metered, governed, and isolated per tenant.

## Version 1.8.0 - Service, Support, And Customer Portal

Status: planned.

Goal: extend beyond sales into post-sale service, matching suites like HubSpot Service and Zoho Desk. Builds on the `0.7.x` customer operations work.

Candidate slices:

- `1.8.1` Ticket/case object with status, priority, and assignment.
- `1.8.2` Email-to-ticket and form-to-ticket intake.
- `1.8.3` SLA policies, escalations, and business hours.
- `1.8.4` Knowledge base / help articles (internal and public).
- `1.8.5` Customer portal for clients to submit and track requests.
- `1.8.6` CSAT/feedback collection and reporting.

Exit criteria:

- Support teams can run a help desk against the same CRM records.
- Customers have a self-service portal and knowledge base.

## Version 1.9.0 - Ecosystem And Extensibility

Status: planned.

Goal: make Open CRM extensible and integrated, so it fits existing tool stacks. Builds on the `0.8.x` integration foundation.

Candidate slices:

- `1.9.1` Custom object builder (fully user-defined objects, not just custom fields from `0.5.5`).
- `1.9.2` Mature public REST API + GraphQL (optional) with versioning and SDKs.
- `1.9.3` Integration marketplace UI with OAuth app connections.
- `1.9.4` First-party integrations: Slack, Google Workspace, Microsoft 365, Mailchimp, accounting (QuickBooks/Xero), Zapier/Make.
- `1.9.5` App/extension framework for third-party UI cards and actions.
- `1.9.6` Webhook management UI and event subscriptions for partners (extends `0.8.3`).

Exit criteria:

- Customers can model their own objects and connect their existing tools.
- Third parties can build on a documented, versioned platform.

## Version 1.10.0 - Mobile And Real-Time Collaboration

Status: planned.

Goal: meet users where they work and make the app feel live.

Candidate slices:

- `1.10.1` Responsive PWA with offline-capable core workflows (builds on `0.3.7b`).
- `1.10.2` Native mobile apps (iOS/Android) or wrapped PWA with push notifications.
- `1.10.3` Real-time record updates and presence via websockets/SSE.
- `1.10.4` Collaborative comments, @mentions, and live activity (extends `0.4.5`).
- `1.10.5` Mobile-optimized calling, email, and quick-logging.
- `1.10.6` Unified notification delivery: in-app, email, push, mobile.

Exit criteria:

- Field and remote users can run core CRM workflows from mobile.
- Multiple users see live updates without manual refresh.

## Version 2.0.0 - Enterprise And General Availability

Status: planned.

Goal: clear enterprise procurement and reach general availability as a competitive CRM SaaS.

Candidate slices:

- `2.0.1` Enterprise SSO: SAML 2.0 and SCIM provisioning/deprovisioning.
- `2.0.2` Advanced security: field-level permissions, IP allowlists, encryption at rest controls, session policies.
- `2.0.3` Audit and compliance posture: SOC 2 controls, data processing records, configurable data residency.
- `2.0.4` Sandbox/staging tenant environments and config migration.
- `2.0.5` High-availability deployment, read replicas, and tenant-scaling strategy.
- `2.0.6` Uptime SLA, status page, and tiered support operations.
- `2.0.7` Internationalization (i18n) and localization across the product.
- `2.0.8` Partner/marketplace program and certification.
- `2.0.9` GA readiness review: security audit, load/failure testing at scale, pricing/packaging finalization.

Exit criteria:

- Open CRM passes enterprise security and procurement review.
- The product is generally available with the breadth to compete head-to-head with established CRM SaaS vendors.
