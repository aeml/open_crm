# Open CRM Roadmap

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
- `0.2.2` Frontend Maintainability: complete.
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
- `0.4.5` Mention And Follow Model: planned.
- `0.4.6` Team Activity Digest: planned.
- `0.4.7` Admin User Lifecycle Hardening: planned.
- `0.4.8` Team Usage Reporting: planned.
- `0.4.9` Team Release Review: planned.
- `0.5.0` CRM Data Operations: planned.
- `0.5.1` Bulk Actions: planned.
- `0.5.2` Duplicate Management: planned.
- `0.5.3` Import Mapping UI: planned.
- `0.5.4` Import Validation And Rollback: planned.
- `0.5.5` Custom Fields Foundation: planned.
- `0.5.6` Custom Field Filtering: planned.
- `0.5.7` Data Quality Reports: planned.
- `0.5.8` Archive And Retention Controls: planned.
- `0.5.9` Data Operations Review: planned.
- `0.6.0` Sales Workflow Maturity: planned.
- `0.6.1` Pipeline Configuration: planned.
- `0.6.2` Deal Probability And Forecasting: planned.
- `0.6.3` Task Automation Rules: planned.
- `0.6.4` Reminder Workflow: planned.
- `0.6.5` Sales Activity Reporting: planned.
- `0.6.6` Contact Touchpoint Tracking: planned.
- `0.6.7` Quote Or Proposal Placeholder Flow: planned.
- `0.6.8` Win Loss Review: planned.
- `0.6.9` Sales Workflow Review: planned.
- `0.7.0` Customer Operations: planned.
- `0.7.1` Post-Sale Account View: planned.
- `0.7.2` Client Health Signals: planned.
- `0.7.3` Renewal And Follow-Up Tasks: planned.
- `0.7.4` Service Or Job Tracking: planned.
- `0.7.5` Account Notes And Internal Handoff: planned.
- `0.7.6` Customer Segment Views: planned.
- `0.7.7` Customer Activity Reports: planned.
- `0.7.8` Customer Data Review: planned.
- `0.7.9` Customer Operations Review: planned.
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
- `0.9.0` Scale And Reliability: planned.
- `0.9.1` Query Performance Review: planned.
- `0.9.2` Pagination And Large Dataset Hardening: planned.
- `0.9.3` Background Job Runner: planned.
- `0.9.4` Async Import And Export Jobs: planned.
- `0.9.5` Backup Automation: planned.
- `0.9.6` Restore Drill Automation: planned.
- `0.9.7` Monitoring And Alerting Hooks: planned.
- `0.9.8` Load And Failure Testing: planned.
- `0.9.9` Reliability Release Review: planned.
- `0.10.0` Production Beta: planned.

### Part II — Competitive SaaS Platform

- `1.0.0` Multi-Tenant SaaS Platform (signup, billing, plan gating, SSO): in progress.
- `1.1.0` Email And Communications (2-way sync, tracking, templates, sequences): in progress.
- `1.2.0` Telephony, SMS, And Meeting Scheduling: planned.
- `1.3.0` Sales Acceleration And CPQ (catalog, quotes, e-sign, quotas): in progress.
- `1.4.0` Marketing And Lead Generation (forms, pages, campaigns, scoring): in progress.
- `1.5.0` Workflow Automation Engine (visual builder): planned.
- `1.6.0` Reporting And Analytics (custom report builder, dashboards): planned.
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

- Add `.nvmrc` and optional `.node-version` for Node 18.
- Add package manager metadata where useful.
- Update README local development instructions.
- Confirm frontend tests and builds run from a clean checkout.

Exit criteria:

- A new contributor can install and test with the documented runtime versions.
- Local frontend verification no longer depends on an accidental global Node/npm version.

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
- Move repeated response and decode helpers into small platform/web utilities.
- Avoid new framework dependencies unless they clearly remove more complexity than they add.

Exit criteria:

- Handler files are easier to review in isolation.
- No behavior changes beyond tested refactors.

## Version 0.2.2 - Frontend Maintainability

Status: complete.

Goal: make route components easier to evolve without changing the visual language.

- Extract large route sections into feature-local components where it reduces complexity.
- Add consistent loading, empty, error, and retry states.
- Add request cancellation with `AbortController` through the shared API client.
- Add minimal ESLint configuration.

Exit criteria:

- Large routes are easier to modify safely.
- Search and detail loading no longer leave unnecessary in-flight requests.

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

Status: planned.

Goal: make core CRM workflows usable on smaller viewports without committing to a separate mobile app.

- Audit list, detail, settings, and dashboard views at common breakpoints.
- Fix overflow, table reflow, and side-nav behavior on narrow widths.
- Verify touch target sizing for primary actions.
- Add a small set of layout tokens or utilities only if they reduce duplication.

Exit criteria:

- Operators can use the CRM from a phone or tablet for read-heavy workflows.
- Layout regressions are visible in tests where practical.

## Version 0.3.7c - Error Boundaries And Session UX

Status: planned.

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

- Enable Dependabot or Renovate for Go modules, npm, and GitHub Actions.
- Add `go mod tidy` and `npm audit --omit=dev` checks to CI as advisory or required gates.
- Document the dependency budget rules from `mvp.md` in a short policy doc.
- Track third-party version skew in the roadmap rather than chasing every minor bump.

Exit criteria:

- Security-relevant updates surface as PRs without manual checking.
- The dependency surface stays explicit and small.

Completion notes:

- Added `.github/dependabot.yml` — weekly PRs for `gomod`, `npm`, and `github-actions`.
- Added `go mod tidy` diff check and `npm audit --audit-level=high` gate to CI.
- Promoted `golang.org/x/crypto` from indirect to direct in `go.mod`.
- CI path triggers extended to include `.github/dependabot.yml`.

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

Status: planned.

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

## Version 0.4.5 - Mention And Follow Model

Status: planned.

Goal: let team members intentionally pull others into record context.

- Add record followers or watchers for core entities.
- Add simple `@mention` parsing in notes if it fits the UI.
- Generate notification events for mentions/follows.
- Keep permissions scoped to organization membership.

Exit criteria:

- Users can subscribe to relevant record updates.
- Mentions/follows produce reviewable in-app events.

## Version 0.4.6 - Team Activity Digest

Status: planned.

Goal: help teams understand what changed recently without opening every record.

- Add team-wide recent activity views with filters.
- Add "my followed records" activity filtering.
- Add date and actor filters.
- Keep activity queries indexed.

Exit criteria:

- Users can review relevant team changes from one place.
- Activity remains useful without becoming an unfiltered firehose.

## Version 0.4.7 - Admin User Lifecycle Hardening

Status: planned.

Goal: make user access changes safer over time.

- Add disable/reactivate user flows.
- Decide how reassignment works for disabled users.
- Invalidate sessions for disabled users.
- Add audit entries for lifecycle events.

Exit criteria:

- Admins can remove access without deleting historical ownership context.
- Disabled users cannot keep active sessions.

## Version 0.4.8 - Team Usage Reporting

Status: planned.

Goal: give admins basic visibility into whether the CRM is being used.

- Add reports for active users, records created, tasks completed, and notes added.
- Keep reporting lightweight and operationally useful.
- Avoid surveillance-style metrics that do not help the CRM workflow.
- Add date range filters.

Exit criteria:

- Admins can see adoption and usage trends.
- Reports are simple enough to trust and explain.

## Version 0.4.9 - Team Release Review

Status: planned.

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

Status: planned.

Goal: make data maintenance practical for real CRM usage.

- Support bulk workflows for common list maintenance.
- Improve import/export beyond the initial foundation.
- Add duplicate and data quality tools.
- Keep every destructive operation reversible or explicitly confirmed.

Exit criteria:

- Operators can maintain CRM data without one-record-at-a-time friction.
- Data operations are safe enough for production use.

## Version 0.5.1 - Bulk Actions

Status: planned.

Goal: reduce repetitive record maintenance.

- Add multi-select list interactions.
- Support bulk archive, owner assignment, status changes, and task completion where appropriate.
- Add confirmation flows for destructive bulk actions.
- Add backend safeguards for organization scoping and row counts.

Exit criteria:

- Common bulk edits can be completed safely from list views.
- Accidental destructive actions are hard to trigger.

## Version 0.5.2 - Duplicate Management

Status: planned.

Goal: help users identify and resolve duplicate contacts and companies.

- Add duplicate candidate detection views.
- Support manual merge or archive workflows.
- Preserve notes, tasks, activities, and links during merge decisions.
- Add tests for duplicate resolution edge cases.

Exit criteria:

- Users can resolve obvious duplicates without direct database work.
- Merges do not orphan related records.

## Version 0.5.3 - Import Mapping UI

Status: planned.

Goal: make CSV import usable for real-world files with varied column names.

- Add upload and column mapping screens.
- Suggest mappings for common column names.
- Show preview rows before import.
- Persist no file contents longer than needed.

Exit criteria:

- Users can map CSV columns without editing files by hand.
- Imports remain transparent before data is written.

## Version 0.5.4 - Import Validation And Rollback

Status: planned.

Goal: make imports safe after validation moves from preview to write.

- Write imports as tracked batches.
- Add row-level success/failure reporting.
- Support rollback for recently imported batches where feasible.
- Add operational guidance for failed imports.

Exit criteria:

- Import results are auditable.
- Bad imports can be corrected without manual SQL in common cases.

## Version 0.5.5 - Custom Fields Foundation

Status: planned.

Goal: support lightweight organization-specific data without turning into a schema builder.

- Add custom field definitions for selected entities.
- Support a small initial set of field types.
- Store values with validation and organization scoping.
- Keep core fields first-class and explicit.

Exit criteria:

- Organizations can capture a few business-specific fields.
- Custom fields do not compromise core schema clarity.

## Version 0.5.6 - Custom Field Filtering

Status: planned.

Goal: make custom fields useful in day-to-day list workflows.

- Add list display options for custom fields.
- Add filtering for supported custom field types.
- Include custom fields in saved views where appropriate.
- Review query plans before broad usage.

Exit criteria:

- Custom field data can be used to find records.
- Filtering remains performant on realistic datasets.

## Version 0.5.7 - Data Quality Reports

Status: planned.

Goal: surface incomplete or suspicious CRM data before it causes workflow issues.

- Add reports for missing owners, missing contact details, stale deals, and incomplete records.
- Make reports actionable with links to filtered records.
- Support business-profile-specific quality rules.
- Add tests for report counts and filters.

Exit criteria:

- Operators can find data cleanup work without manual searches.
- Reports produce explainable counts.

## Version 0.5.8 - Archive And Retention Controls

Status: planned.

Goal: make archived data behavior explicit.

- Add archived list views and restore actions where appropriate.
- Document what archive means for related notes, tasks, activities, and reports.
- Add retention planning for future hard-delete needs.
- Keep destructive deletion out unless required by real usage.

Exit criteria:

- Users can find and restore archived records.
- Data lifecycle behavior is documented and predictable.

## Version 0.5.9 - Data Operations Review

Status: planned.

Goal: close the data operations milestone safely.

- Test import, export, duplicate, bulk, custom field, and archive flows together.
- Re-run the tenant isolation suite against new bulk and custom-field write paths.
- Review data integrity constraints after new features.
- Update documentation for data operations.
- Identify scale risks before sales workflow expansion.

Exit criteria:

- Data operations are reliable enough for non-technical operators.
- New data features do not undermine schema integrity.

## Version 0.6.0 - Sales Workflow Maturity

Status: planned.

Goal: make deal management useful beyond a basic pipeline list.

- Improve pipeline configuration and forecasting.
- Add sales activity and touchpoint tracking.
- Add reminders and lightweight automation.
- Keep workflow configurable without becoming a workflow engine.

Exit criteria:

- Sales operators can manage pipeline health and next actions in Open CRM.
- Sales features remain understandable and maintainable.

## Version 0.6.1 - Pipeline Configuration

Status: planned.

Goal: let organizations adapt deal stages to their sales process.

- Add stage create, rename, reorder, close/won/lost settings.
- Protect existing deals during stage changes.
- Add validation for stage uniqueness and ordering.
- Add tests for stage transitions.

Exit criteria:

- Admins can configure pipelines without database edits.
- Existing deals remain valid after stage configuration changes.

## Version 0.6.2 - Deal Probability And Forecasting

Status: planned.

Goal: provide simple revenue forecasting without complex sales ops tooling.

- Add probability or confidence fields to deals or stages.
- Add weighted forecast totals.
- Add close-date range filters.
- Make forecast assumptions visible in the UI.

Exit criteria:

- Users can see unweighted and weighted pipeline values.
- Forecast numbers are easy to explain.

## Version 0.6.3 - Task Automation Rules

Status: planned.

Goal: remove repetitive follow-up setup for predictable CRM events.

- Add simple rules for creating tasks on deal creation, stage change, or archive.
- Keep rule conditions intentionally limited.
- Add per-organization rule settings.
- Add tests to prevent duplicate task creation.

Exit criteria:

- Common follow-up tasks can be generated automatically.
- Automation is simple enough for operators to audit.

## Version 0.6.4 - Reminder Workflow

Status: planned.

Goal: help users avoid missed follow-ups.

- Add due-soon and overdue reminder surfaces.
- Add in-app notification events for assigned tasks.
- Prepare email reminders only if in-app reminders prove useful.
- Add digest-friendly reminder queries.

Exit criteria:

- Users can see what follow-ups are due without manual filtering.
- Reminder behavior is predictable and not noisy.

## Version 0.6.5 - Sales Activity Reporting

Status: planned.

Goal: make sales effort and outcomes visible.

- Add reports for deals created, won, lost, moved stages, and notes/tasks by date range.
- Add owner filters.
- Add basic conversion rates by stage where data supports it.
- Avoid vanity metrics without clear operator value.

Exit criteria:

- Sales teams can review activity and outcomes by period.
- Reports match underlying record history.

## Version 0.6.6 - Contact Touchpoint Tracking

Status: planned.

Goal: make follow-up history clearer for contacts and companies.

- Add last-contacted or last-touch indicators from notes/tasks/activity.
- Add stale contact views.
- Add touchpoint summaries to record detail pages.
- Keep automatic inference transparent.

Exit criteria:

- Users can find contacts or companies that need follow-up.
- Touchpoint dates are understandable and traceable.

## Version 0.6.7 - Quote Or Proposal Placeholder Flow

Status: planned.

Goal: leave room for proposal tracking without building a full quoting system prematurely.

- Add optional proposal/quote status fields on deals if validated by usage.
- Add attachment/link placeholders if needed.
- Document explicit non-goals for quoting.
- Avoid payment, catalog, and document-generation complexity.

Exit criteria:

- Sales users can track whether a proposal exists.
- The feature does not pretend to be a full CPQ system.

## Version 0.6.8 - Win Loss Review

Status: planned.

Goal: capture useful outcome context when deals close.

- Add close reason fields for won/lost deals.
- Add optional notes on close.
- Add win/loss reporting.
- Keep close reason options configurable only if real usage needs it.

Exit criteria:

- Closed deals explain why they closed.
- Win/loss reporting has useful context.

## Version 0.6.9 - Sales Workflow Review

Status: planned.

Goal: close the sales workflow milestone before expanding customer operations.

- Test pipeline configuration, automation, reminders, and reports together.
- Re-run the tenant isolation suite against automation and reporting paths.
- Review query plans for sales reports.
- Update docs for sales workflows.
- Identify product feedback from real sales usage.

Exit criteria:

- Sales workflows are coherent end-to-end.
- Remaining sales work is prioritized from usage data.

## Version 0.7.0 - Customer Operations

Status: planned.

Goal: support the post-sale relationship after a deal becomes a customer account.

- Add account/customer views that connect won deals, companies, contacts, tasks, and history.
- Add customer health and renewal-oriented workflows.
- Support service/job tracking where business profile calls for it.
- Keep the CRM general enough for small service and sales teams.

Exit criteria:

- Users can manage customer relationships after the initial sale.
- Post-sale workflows build on existing CRM records instead of creating a separate product.

## Version 0.7.1 - Post-Sale Account View

Status: planned.

Goal: make customer accounts easier to review after conversion.

- Add account summary panels for companies or individual clients.
- Show related won deals, open tasks, recent notes, and key contacts.
- Add links from won deals to customer/account context.
- Preserve existing company/contact detail workflows.

Exit criteria:

- Users can understand account state from one page.
- Post-sale context does not duplicate core CRM records.

## Version 0.7.2 - Client Health Signals

Status: planned.

Goal: provide lightweight health indicators for customer relationships.

- Define manual health statuses or simple derived signals.
- Add stale follow-up, overdue task, and open issue indicators where available.
- Add health filters to customer views.
- Keep health rules transparent.

Exit criteria:

- Users can quickly identify accounts that may need attention.
- Health indicators are explainable and editable where appropriate.

## Version 0.7.3 - Renewal And Follow-Up Tasks

Status: planned.

Goal: support recurring customer follow-up without calendar complexity.

- Add renewal or review date fields where useful.
- Add task generation for upcoming renewals/reviews.
- Add dashboard sections for customer follow-ups.
- Avoid full subscription billing logic.

Exit criteria:

- Users can track future customer follow-up obligations.
- Renewal/review tasks appear in existing task workflows.

## Version 0.7.4 - Service Or Job Tracking

Status: planned.

Goal: support business profiles that need jobs/projects connected to clients.

- Define a minimal job/service record if usage validates it.
- Link jobs to clients, contacts, notes, tasks, and activity.
- Add status tracking without full project management complexity.
- Keep labels adaptive by business profile.

Exit criteria:

- Service businesses can track active work against clients.
- The feature remains lighter than a project management system.

## Version 0.7.5 - Account Notes And Internal Handoff

Status: planned.

Goal: make sales-to-service context transfer clearer.

- Add account summary notes or handoff notes.
- Highlight important open tasks and recent deal history.
- Add activity entries for ownership or account status changes.
- Keep handoff data visible to the team, not hidden in individual notes.

Exit criteria:

- Team members can pick up customer context quickly.
- Handoff information is explicit and reviewable.

## Version 0.7.6 - Customer Segment Views

Status: planned.

Goal: let users group customer records for follow-up and reporting.

- Add segment filters based on status, health, owner, custom fields, or tags if tags exist.
- Support saved customer segment views.
- Keep segmentation query behavior efficient.
- Avoid marketing automation scope.

Exit criteria:

- Users can find meaningful groups of customer accounts.
- Segment views are reusable.

## Version 0.7.7 - Customer Activity Reports

Status: planned.

Goal: show post-sale work and customer engagement patterns.

- Add reports for customer tasks, notes, health changes, and follow-up activity.
- Add owner and date filters.
- Add links from report rows to source records.
- Keep report definitions simple and explainable.

Exit criteria:

- Customer operations activity is visible by period.
- Reports help operators decide where to focus.

## Version 0.7.8 - Customer Data Review

Status: planned.

Goal: verify that post-sale data additions remain coherent.

- Review schema constraints and indexes for customer operations features.
- Test account, customer, job, task, and note workflows together.
- Review archive behavior for customer-related records.
- Update backup/restore and data export expectations if needed.

Exit criteria:

- Customer operations data is consistent and recoverable.
- New workflows do not create orphaned or ambiguous records.

## Version 0.7.9 - Customer Operations Review

Status: planned.

Goal: close the customer operations milestone before integration work.

- Smoke test end-to-end sales-to-customer lifecycle.
- Re-run the tenant isolation suite against post-sale and job/service paths.
- Review customer workflow feedback.
- Update docs and roadmap for integration priorities.
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

Status: planned.

Goal: make the system resilient under larger datasets and operational failures.

- Review query performance and pagination across core resources.
- Add background job infrastructure for long-running work.
- Automate backup/restore drills where practical.
- Add monitoring and failure testing hooks.

Exit criteria:

- Open CRM can handle realistic pilot data volumes.
- Operators have stronger confidence in recovery and failure behavior.

## Version 0.9.1 - Query Performance Review

Status: planned.

Goal: find and fix slow paths before they become production incidents.

- Review list, dashboard, report, import, and integration queries.
- Add missing indexes based on actual query patterns.
- Add representative dataset benchmarks where practical.
- Avoid premature indexing without query evidence.

Exit criteria:

- Common workflows have predictable query plans.
- Performance fixes are backed by measured query behavior.

## Version 0.9.2 - Pagination And Large Dataset Hardening

Status: planned.

Goal: keep list and report pages usable as records grow.

- Review pagination behavior for all list endpoints.
- Add keyset pagination where offset pagination becomes risky.
- Ensure frontend loading states handle large pages cleanly.
- Add tests for pagination boundaries.

Exit criteria:

- Large datasets do not break core list workflows.
- Pagination behavior is consistent and documented.

## Version 0.9.3 - Background Job Runner

Status: planned.

Goal: support long-running work without blocking HTTP requests.

- Add a minimal Postgres-backed job model or worker process.
- Support job status, retries, and failure reasons.
- Keep job execution simple and observable.
- Avoid external queues until needed.

Exit criteria:

- Long-running operations can move out of request/response paths.
- Job failures are visible and retryable.

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

Status: planned.

Goal: make backups less dependent on manual operator discipline.

- Add documented scheduled backup approach for the current host.
- Review configuration and secrets handling: `.env.example` drift, rotation procedure, secret scope, and inventory.
- Add backup verification metadata.
- Add alerts or logs for failed backups if monitoring exists.
- Keep backup artifacts secure and off-host where possible.

Exit criteria:

- Backups can run on a schedule.
- Operators can verify that recent backups exist.

## Version 0.9.6 - Restore Drill Automation

Status: planned.

Goal: prove backups are usable, not just created.

- Add a documented restore drill procedure for disposable environments.
- Automate as much of the drill as practical.
- Record restore time and failure modes.
- Keep production restore manual and deliberate.

Exit criteria:

- Operators can regularly verify restore viability.
- Restore procedures are tested outside emergencies.

## Version 0.9.7 - Monitoring And Alerting Hooks

Status: planned.

Goal: expose enough operational signals for production support.

- Add metrics or structured log conventions for request errors, latency, jobs, and integrations.
- Add deployment and health-check guidance for alerts.
- Add dashboard/runbook links for common incidents.
- Avoid bringing in heavy monitoring stacks unless needed.

Exit criteria:

- Production issues can be noticed and triaged quickly.
- Monitoring remains understandable for a small deployment.

## Version 0.9.8 - Load And Failure Testing

Status: planned.

Goal: validate behavior under stress and partial failure.

- Add lightweight load test scripts for common workflows.
- Test database unavailability, slow queries, and failed integrations.
- Review graceful degradation and error messages.
- Document known limits.

Exit criteria:

- The team understands practical system limits.
- Failure modes are documented and recoverable.

## Version 0.9.9 - Reliability Release Review

Status: planned.

Goal: close the reliability milestone before production beta.

- Re-run full verification, restore drills, smoke tests, and deploy checks.
- Review open reliability risks.
- Update operations docs and README.
- Decide beta readiness criteria.

Exit criteria:

- Reliability risks are known and prioritized.
- Production beta can start with realistic operational expectations.

## Version 0.10.0 - Production Beta

Status: planned.

Goal: reach a beta-quality product suitable for real small-team CRM usage.

- Freeze the core beta scope around proven workflows.
- Review security, reliability, data portability, and support readiness.
- Generate or refresh `THIRD_PARTY_NOTICES` and verify license obligations across Go modules and npm dependencies.
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

- `1.0.9` (trial-expiry / cancellation enforcement): partial. Added `EnforceWritable` (and a pure `checkWritable` decision) to the billing service; contact, deal, and user-invite creates now return `402 SUBSCRIPTION_INACTIVE` when the subscription is canceled or the trial has expired. Active, in-period trial, and past-due (grace) states remain writable. Fails open on transient billing errors; the frontend surfaces the message via existing error handling and the billing-page banners. Backend tests cover the decision matrix and the contact-create block path. Remaining: read-only/suspended mode UX, broader write coverage (updates), and provider-driven status transitions.
- `1.0.6`/`1.0.9` (subscription status + trial): partial. Added migration `015_subscription_lifecycle.sql` (`subscription_status` with CHECK + `trial_ends_at` on `organizations`); new signups start a 14-day `trialing` subscription, existing orgs are `active`. Entitlements now include a `subscription` block (status, trial end, computed days left, in-trial flag); the billing page shows a trial/status banner. Changing to a paid plan activates the subscription and ends the trial. Backend tests cover trial-day computation and active/expired states. Remaining: dunning, proration, real Stripe webhooks driving status, suspension enforcement on trial expiry.
- `1.0.1` (invite email via fake provider): partial. Added an `email` module with a `Provider` seam, an in-memory `FakeProvider` outbox (default), and an unconfigured stub; `EMAIL_PROVIDER`/`EMAIL_FROM_*`/`WEB_BASE_URL` env wiring; and a templated `SendUserInvite` that emails new users an account-activation link. User invites now send this email best-effort (failures never block the invite). This also establishes the email foundation reused by `1.1`. Remaining: self-serve signup email verification, real delivery provider.
- `1.0.4` (limit enforcement): complete. Added `EnforceCanCreate` to the billing service and a `CanCreateMore` decision helper; create handlers for contacts, deals, and user invites now reject over-limit writes with `402 PLAN_LIMIT_REACHED` and an upgrade message. Enforcement fails open on transient billing-read errors and is skipped when billing is unconfigured. Frontend surfaces the message through existing create-error handling. Backend tests cover the decision logic and the contact-create block/allow paths.
- `1.0.2` (provider seam + fake default): complete. Added a `billing.Provider` interface with a `FakeProvider` (no external calls; default for tests and unconfigured deployments) and an unconfigured stub for real providers. `BILLING_PROVIDER` env selects the provider. Added `POST /api/billing/change-plan` (owner/admin only, audited) and a plan-switch UI on the billing settings page. A comprehensive `example.env` documents all provider configuration (billing, email, telephony, SSO, AI, storage), each defaulting to a fake/disabled provider. Remaining: real Stripe integration (checkout, webhooks, proration).
- `1.0.3`/`1.0.4` (foundation): complete. Added migration `014_billing_plans.sql` (`plan` column on `organizations` with a CHECK constraint); a `billing` module with an in-code plan catalog (Free/Starter/Pro/Enterprise: seat/contact/deal limits + feature keys) and an entitlements service computing live usage; `GET /api/billing/plans` and `GET /api/billing/entitlements`; and a "Plan & Billing" settings page showing usage-against-limits and a plan comparison. Backend + frontend tests added. Remaining: Stripe integration (`1.0.2`), self-serve signup hardening (`1.0.1`), enforcement of limits on write paths, and SSO (`1.0.7`).

Exit criteria:

- A prospect can sign up, start a trial, add a card, and be billed without operator involvement.
- Features and limits are enforced by plan in both API and UI.
- Tenants can be provisioned, suspended, and offboarded cleanly with data portability.

## Version 1.1.0 - Email And Communications

Status: in progress.

Goal: make Open CRM the place revenue teams live by bringing email into the CRM. This is the single highest-impact competitive gap.

Progress:

- Email provider seam established in `1.0.1`: `email.Provider` interface, in-process `FakeProvider` outbox (default), unconfigured stub for real providers, and `EMAIL_PROVIDER` selection. The transactional/outbound foundation (`1.1.1`) builds directly on this.
- **Email architecture decision:** system email (platform → CRM users: invites, password setup, notifications) goes through the shared transactional provider (Postmark). Customer-facing email (CRM user → their contacts) goes through each user's own mailbox (SMTP), sending as the user — never the platform's provider.
- `1.1.1` (system email via Postmark): complete. Added a `PostmarkProvider` (mirrors the Mendola customer-panel sender) selected by `EMAIL_PROVIDER=postmark` with `POSTMARK_SERVER_TOKEN` / `POSTMARK_FROM_EMAIL` / `POSTMARK_MESSAGE_STREAM`. Used for invites and other platform email only.
- `1.1.2` (per-user mailbox connections): complete (SMTP send; IMAP reserved for two-way sync). Added migration `017_user_email_accounts.sql`, an AES-256-GCM `secrets` cipher (`CREDENTIAL_ENCRYPTION_KEY`) so SMTP passwords are encrypted at rest, a `useremail` module (upsert/get-sanitized/delete/credentials/`SendAs`), an SMTP sender supporting implicit TLS / STARTTLS, and `GET/PUT/DELETE /api/me/email-account`. A "My Email" settings page lets users connect their mailbox; passwords are never returned. Backend (cipher, validation, handler) and frontend tests added.
- `1.1.2` (mailbox sync/OAuth-IMAP metadata foundation): complete. Added migration `023_user_email_sync_foundation.sql` with provider/auth method, sync enabled/status/cursor, last sync/error, OAuth token placeholders, and IMAP TLS metadata on `user_email_accounts`. `GET /api/me/email-sync/status` exposes sanitized sync state plus Google/Microsoft OAuth readiness. Settings > My Email now stores optional IMAP sync metadata while keeping SMTP send configuration unchanged.
- `1.1.2` (OAuth mailbox callback/token exchange): complete. Added signed OAuth state, Google/Microsoft authorization-start and callback endpoints (`POST /api/me/email-sync/oauth/{provider}/start`, `GET /api/me/email-sync/oauth/{provider}/callback`), provider token exchange, encrypted OAuth access/refresh token storage, and Settings > My Email connect buttons/status messages.
- `1.1.2` (mailbox sync readiness check): complete. Added sync-state update support on `useremail`, `POST /api/me/email-sync/check`, and a Settings > My Email readiness action. The check validates IMAP password metadata or OAuth connection state and records `ready`/`error` status for the future sync worker without importing messages yet.
- `1.1.2` (inbound mailbox storage foundation): complete. Added migration `024_inbound_email_messages.sql` to make `email_messages` direction-aware (`outbound`/`inbound`) with from-address, mailbox owner, provider message/thread IDs, and received timestamp metadata. `emailmessages.RecordInbound` provides idempotent inbound storage for future IMAP/Gmail/Graph ingestion, and the member mailbox view now reads combined inbound/outbound mailbox messages while preserving sender/admin detail access controls.
- `1.1.2` (generic IMAP manual sync runner): complete. Added an injectable `mailboxsync` service plus `POST /api/me/email-sync/run` so users can manually import recent generic IMAP inbox messages into the existing inbound `email_messages` store with duplicate provider IDs skipped, cursor/last-sync metadata updated, and sync errors recorded on the mailbox account. Settings > My Email now exposes a "Run sync now" action.
- `1.1.2` (Gmail API sync fetcher): complete. Google OAuth mailbox accounts now sync through the Gmail API using the stored encrypted access token, importing recent inbox messages as normalized inbound `email_messages` with Gmail message/thread IDs and the same matching/privacy path as IMAP. Automatic sync target discovery now includes Google OAuth accounts.
- `1.1.2` (Microsoft Graph sync fetcher): complete. Microsoft OAuth mailbox accounts now sync through Microsoft Graph using the stored encrypted access token, importing recent Inbox messages as normalized inbound `email_messages` with Graph/internet message IDs and conversation IDs. Automatic sync target discovery now includes Microsoft OAuth accounts.
- `1.1.2` (OAuth sync token refresh): complete. Mailbox sync now loads OAuth token expiry metadata, refreshes expired or missing access tokens with the stored refresh token before fetching Gmail/Graph messages, persists refreshed encrypted tokens, and reports provider refresh failures in sync state instead of failing opaquely during ingestion.
- `1.1.3` (automatic inbound record matching): complete. Added migration `025_email_message_entity_links.sql` so one email message can be linked to multiple CRM records while preserving the legacy primary `entity_type`/`entity_id` fields. Inbound sync now matches the sender email to an active contact, that contact's linked companies, and related open deals before storing the message, so the same synced email appears in contact/company/deal histories. Existing sent/received messages with a primary entity are backfilled into the link table.
- `1.1.3` (email message privacy controls): complete. Added migration `026_email_message_visibility.sql` with `shared`/`private` visibility on `email_messages`; existing inbound messages are backfilled private while outbound remains shared. New synced inbound mailbox messages default private, and per-record histories only include private messages for org admins, the sender, or the mailbox owner.
- `1.1.2` (automatic mailbox sync worker): complete. Added due-account discovery for enabled generic IMAP/password, Google OAuth, and Microsoft OAuth mailboxes and an in-process mailbox sync worker that starts after API boot, polls due accounts every 15 minutes, runs the same ingestion path as `POST /api/me/email-sync/run`, and logs batch import/failure counts. A durable external job runner remains a future slice.
- `1.1.6` (send-from-record): updated to route through the sending user's mailbox (`SendAs`) instead of the shared provider; returns `EMAIL_ACCOUNT_REQUIRED` when the user has not connected their email. Merge-field rendering and contact-timeline logging unchanged.
- `1.1.4` (email outbox/log): complete. Added migration `018_email_messages.sql` and an `emailmessages` module recording every customer email send (status `sent`/`failed`, recipient, subject, body, linked record, sender). Sends from contacts are recorded automatically. `GET /api/email-messages` serves both the per-record history (`?entityType=contact&entityId=` — any member) and the org-wide log (no filter — admin only). Frontend: an admin "Email Log" settings page and a lazy-loaded email history on the contact detail. Backend handler tests and a frontend page test added. Live server configured with Postmark (system mail) + `CREDENTIAL_ENCRYPTION_KEY` (per-user SMTP) and verified healthy.
- `1.1.2` (admin sets member mailbox): complete. Org admins/owners can connect, view, and remove a team member's mailbox via `GET/PUT/DELETE /api/users/{id}/email-account` (membership-verified before write). Frontend: a "Set up email for a member" panel on the Users settings page with a member selector. Backend handler tests (admin gating, non-member rejection) and a frontend flow test added.
- `1.1.6` (send-from-company/deal): complete. Added `POST /api/companies/{id}/email` and `POST /api/deals/{id}/email`, both sending through the current user's connected mailbox, rendering record-specific merge fields, recording to `email_messages`, and adding a note to the source record. Frontend: shared record email composer on company and deal detail pages with lazy template/history loading. Backend company/deal send tests and a frontend company-send flow test added.
- `1.1.6` (personal mailbox/sent view): complete. Added member-safe `GET /api/me/email-messages` backed by `emailmessages.ListBySender`, plus a primary-nav "Mailbox" page showing the current user's sent CRM emails and links back to source contacts/companies/deals. Backend scoping test and frontend route test added.
- `1.1.6` (email message detail): complete. Added `GET /api/email-messages/{id}` with admin-or-sender access control so users can inspect full body/error detail without exposing other users' message bodies to members. The Mailbox and admin Email Log now include "View details" panels. Backend access-control tests and frontend detail tests added.
- `1.1.4` (open tracking foundation): complete. Added migration `019_email_open_tracking.sql` with tracking tokens and open counters, a public no-auth tracking-pixel endpoint (`GET /api/email-messages/open/{token}`), multipart text+HTML SMTP sends with a hidden tracking pixel for CRM customer email, and open-count display in Mailbox/Admin Email Log. Backend tracking/MIME tests and frontend open-count assertions added.
- `1.1.4` (click tracking foundation): complete. Added migration `020_email_click_tracking.sql` with aggregate click counters plus a per-link token table, rewrote HTTP(S) URLs in outbound CRM email HTML through a public tracked redirect (`GET /api/email-messages/click/{token}`), and added click-count display in Mailbox/Admin Email Log. Backend redirect/link-recording tests and frontend click-count assertions added.
- `1.1.5` (templates/snippets/merge fields): complete. Added an authenticated merge-field catalog for contact/company/deal email tokens, reusable organization-scoped email snippets, Settings > Email Templates management for templates and snippets, and record-composer snippet insertion so users do not need to memorize placeholders or retype common fragments.
- `1.1.8` (sequence definition foundation): complete. Added migration `021_email_sequences.sql` for organization-scoped sequence metadata and ordered steps, a backend `emailsequences` module with admin/writer CRUD endpoints (`/api/email-sequences`), and a Settings > Email Sequences page for drafting cadence definitions. This intentionally does not enroll contacts, schedule sends, or detect replies yet.
- `1.1.8` (sequence enrollment/schedule-state foundation): complete. Added migration `022_email_sequence_enrollments.sql` for active/paused/completed/cancelled contact enrollments with `current_step_order` and `next_send_at`, backend list/create/cancel endpoints (`/api/email-sequence-enrollments`), and a contact-detail Sequences panel for enrolling contacts. This stores scheduler state only; automated sends do not run yet.
- `1.1.8` (sequence reply detection foundation): complete. Inbound synced email that matches a contact now completes that contact's active/paused sequence enrollments and clears `next_send_at`, so replies stop future sequence sends.
- `1.1.8` (sequence send worker foundation): complete. Added an in-process sequence runner that polls due active enrollments, renders contact merge fields, sends the current step through the enrolling user's mailbox, records sent/failed messages in `email_messages`, advances to the next step using its delay, and completes the enrollment after the last step. Failed sends are postponed for retry instead of being hammered every worker interval.
- `1.1.7` (unsubscribe/suppression foundation): complete. Added organization-scoped recipient suppressions, HMAC-signed public unsubscribe links, one-to-one send suppression checks with compliance footers, and sequence-runner suppression enforcement that records suppressed sends and advances enrollments instead of retrying forever. Bulk list selection, campaign UI, and richer compliance reporting remain deferred to the marketing/bulk-email slice.
- `1.1.9` (shared team inbox foundation): complete. Added shared inbound email queue metadata, team inbox listing, assignment/open/closed status updates, mailbox-owner/admin sharing controls for private synced messages, member-safe detail access for shared inbound messages, and a Team Inbox UI for collaborative follow-up.

Candidate slices:

- `1.1.1` Outbound transactional/CRM email infrastructure via a provider (Postmark/SendGrid) with domain auth (SPF/DKIM/DMARC) and bounce/complaint handling.
- `1.1.2` Two-way mailbox sync via Gmail and Microsoft 365 OAuth (and generic IMAP/SMTP) with per-user connection.
- `1.1.3` Automatic email logging to matching contacts/companies/deals with privacy controls (shared vs. private).
- `1.1.4` Email open and link-click tracking with per-message and aggregate engagement.
- `1.1.5` Email templates, snippets, and merge fields: complete.
- `1.1.6` One-to-one send from record pages and a connected-inbox view.
- `1.1.7` Bulk/mass email with list selection, unsubscribe management, and CAN-SPAM/GDPR compliance footers. Suppression/unsubscribe primitives are complete; bulk campaign UX remains future work.
- `1.1.8` Email sequences / cadences: multi-step automated outreach with conditions and reply detection.
- `1.1.9` Shared team inboxes and assignment for collaborative reply workflows: foundation complete.

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
- `1.2.6` (booking links foundation): complete. Added organization-scoped calendar booking links with slugs, duration/buffer/timezone metadata, active/inactive state, selected host members, owner vs round-robin assignment mode, authenticated APIs for listing/creating/updating links, and a Settings > Booking Links UI for managing links and weekly availability. Public booking pages, guest self-scheduling, slot generation, real round-robin assignment, external calendar conflict checks, reminders, and rescheduling/cancellation flows remain future slices.
- `1.2.7` (meeting reminders foundation): complete. Added persistent meeting reminder records for scheduled calendar events, automatic pending-reminder skipping when meetings are cancelled, and an in-process reminder worker that creates in-app `meeting.reminder` notifications plus `meeting.reminder_sent` activity timeline entries when reminders are due. Customer-facing email/SMS reminders, configurable reminder offsets, guest reminder preferences, calendar-provider notifications, and reminder delivery analytics remain future slices.

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
- `1.3.5` (multiple pipelines foundation): complete. Added organization-scoped deal pipelines, backfilled existing stages into a default pipeline, scoped stage uniqueness by pipeline, exposed authenticated APIs for listing/creating pipelines, copied default stage templates into new pipelines, added pipeline metadata/filtering to deal list/detail/export flows, and added pipeline selection/creation UI on the deals page. Pipeline renaming/reordering, custom stage management, team/business-unit ownership rules, and per-pipeline permissions remain future slices.
- `1.3.6` (quotas and forecasting dashboard foundation): complete. Added organization-scoped per-user sales quota records by period, admin quota upsert API, current-quarter forecast calculations using won revenue plus stage-weighted open pipeline, team/member attainment and coverage metrics, and dashboard quota editing/forecast display. Forecast categories, custom stage probabilities, quota history, rollups by team/business unit, and advanced forecast analytics remain future slices.
- `1.3.7` (multi-currency exchange-rate foundation): complete. Added organization base currency settings, manual organization exchange-rate records, admin API/UI for saving rates, and base-currency conversion for deal-list, dashboard pipeline, quota, and weighted forecast rollups while preserving per-record deal/catalog currencies. Automated FX providers, historical rate selection beyond latest manual rates, quote-level FX disclosures, and realized gain/loss accounting remain future slices.

Candidate slices:

- `1.3.1` Product/service catalog with pricing, SKUs, and currency: foundation complete.
- `1.3.2` Deal line items, discounts, taxes, and totals: foundation complete.
- `1.3.3` Quote/proposal generation with branded PDF output: foundation complete.
- `1.3.4` E-signature flow (native or DocuSign/Dropbox Sign integration) with status tracking: foundation complete.
- `1.3.5` Multiple pipelines per team/business unit (extends `0.6.1`): foundation complete.
- `1.3.6` Quotas, goals, and team forecasting dashboards (extends `0.6.2`): foundation complete.
- `1.3.7` Multi-currency support with exchange-rate handling: foundation complete.

Exit criteria:

- A rep can build a quote from a catalog, send it for signature, and convert it to a closed deal.
- Managers can set quotas and track weighted forecast against them.

## Version 1.4.0 - Marketing And Lead Generation

Status: in progress.

Goal: capture and nurture demand, not just manage existing relationships.

Progress:

- `1.4.1` (embeddable lead capture forms foundation): complete. Added organization-scoped lead capture form definitions with generated public IDs, mapped standard fields into CRM lead contacts on public submissions, stored raw submission payloads and request metadata, exposed admin APIs for listing/creating/updating forms plus an unauthenticated public submission endpoint, and added a Settings > Lead Forms UI with a standard mapped field set and HTML form embed snippet. Hosted landing pages, custom dynamic field builders, attribution/UTM capture, scoring, routing, campaign enrollment, spam protection, and richer submission management remain future slices.
- `1.4.2` (hosted landing pages foundation): complete. Added organization-scoped landing pages tied to existing lead capture forms, globally unique public slugs, active/inactive state, light/blue/dark themes, authenticated APIs for listing/creating/updating pages, a public page lookup endpoint, a Settings > Landing Pages UI, and a public `/lp/:slug` frontend route that renders the marketing copy plus embedded lead form submission flow. Rich page templates, drag-and-drop editing, custom domains, SEO metadata controls, analytics, attribution, A/B tests, and campaign enrollment remain future slices.
- `1.4.3` (lead source and UTM/campaign attribution tracking foundation): complete. Added first-touch lead source, source URL, and standard UTM fields on contacts created from public lead submissions; persisted attribution columns on raw lead submissions; derived attribution from submitted source URLs and explicit embed fields; surfaced attribution on contact detail/list APIs, contact detail UI, hosted landing page submissions, lead form embed snippets, and contacts CSV exports. Full campaign objects, attribution reports, multi-touch history, and automated routing/scoring remain future slices.
- `1.4.4` (list segmentation and dynamic/saved audiences foundation): complete. Added organization-scoped lead audience definitions with reusable filters, dynamic member-count previews against contacts, authenticated APIs for listing/creating/updating/previewing audiences, and a Settings > Audiences UI for saving source/campaign/status/email-availability segments. Campaign enrollment, audience member drill-down, exclusion rules, advanced boolean logic, and scheduled snapshots remain future slices.
- `1.4.5` (marketing email campaigns with scheduling and per-campaign analytics foundation): complete. Added organization-scoped marketing email campaign definitions tied to active saved audiences, stored schedule/status metadata, captured audience recipient-count snapshots, persisted per-campaign analytics counters, exposed authenticated APIs for listing/creating/updating campaigns, and added a Settings > Email Campaigns UI. Bulk recipient expansion, mailbox delivery, unsubscribe enforcement at send time, open/click attribution into campaign counters, send approvals, and campaign reports remain future slices.
- `1.4.6` (drip/nurture campaigns built on the sequence engine foundation): complete. Added organization-scoped nurture campaign plans that bind active saved audiences to existing email sequences, validate active campaigns against active sequences, snapshot eligible audience counts, expose authenticated APIs for listing/creating/updating nurture campaigns, and add a Settings > Nurture Campaigns UI. Automatic audience enrollment, enrollment refresh scheduling, suppression-aware bulk launch approvals, per-nurture performance rollups, and reply/exit rules remain future slices.
- `1.4.7` (rule-based lead scoring and routing/assignment foundation): complete. Added organization-scoped scoring rules over contact status, source, UTM, title, email, phone, and email-domain signals; persisted contact score/grade/scored-at metadata; exposed admin APIs for scoring-rule management plus a contact evaluation endpoint; routed unassigned contacts to rule-selected team members; logged lead scoring activity; surfaced scoring in contact list/detail; and added a Settings > Lead Scoring UI. Automatic scoring on form submission, bulk rescoring, rule simulation over audiences, SLA queues, and round-robin assignment remain future slices.
- `1.4.8` (lead capture from chat/website widget foundation): complete. Added organization-scoped website widget definitions tied to existing lead capture forms, stable public widget IDs, light/blue/dark themes, bottom-left/bottom-right/inline embed positions, authenticated APIs for listing/creating/updating widgets, a public widget lookup endpoint, a `/widget/:publicId` frontend renderer, and a Settings > Website Widgets UI with iframe embed snippets. Live chat, bot conversation trees, agent handoff, widget analytics, spam protection, automatic scoring/routing on submission, and custom script-loader embeds remain future slices.

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

Progress:

- `1.5.1` (trigger model foundation): complete. Added organization-scoped workflow automation definitions with typed triggers for record created/updated, deal stage changed, date reached, form submitted, inbound email, and webhook events; persisted target entity and JSON trigger config metadata; exposed authenticated APIs for listing/creating/updating automation trigger definitions; and added a Settings > Automations UI. Trigger detection, condition evaluation, action execution, scheduling/delays, webhooks, visual editing, run history, loop protection, and retry/idempotency remain future slices.
- `1.5.2` (condition/branching foundation): complete. Extended workflow automation definitions with all/any condition logic and validated condition arrays over contact, company, deal, task, form-submission, inbound-email, and webhook fields; added a pure condition evaluator for equality, inequality, contains, exists, greater-than, and less-than checks; exposed conditions through the workflow automation APIs; and expanded Settings > Automations with condition editing. Visual branching, nested condition groups, trigger-time record hydration, action execution, schedule/delay handling, run history, and retry/idempotency remain future slices.
- `1.5.3` (action library foundation): complete. Added ordered workflow action definitions with validation for update field, create task, send email, send SMS, assign owner, add to sequence, call webhook, and notify action types; persisted action arrays on workflow automations; exposed actions through the workflow automation APIs; and expanded Settings > Automations with action editing. Action execution, provider dispatch, trigger-time record hydration, visual action cards, delays/schedules, run history, and retry/idempotency remain future slices.
- `1.5.4` (visual workflow builder foundation): complete. Added a guided Settings > Automations builder that visualizes trigger, condition, and action steps; provides target-aware condition field/operator controls; adds/removes condition chips and ordered action cards; and keeps advanced JSON editing as the persisted source of truth. Drag/drop layout, nested branches, full action-specific forms, record hydration previews, execution, delays/schedules, run history, and retry/idempotency remain future slices.
- `1.5.5` (scheduled/time-delay action foundation): complete. Added validated per-action timing metadata for relative `delayMinutes` and absolute `scheduledAt` plans, normalized scheduled action times to UTC, exposed timing controls in the visual builder, and added a pure planned-action-time helper for the future background runner. Trigger detection, action queue persistence, due-action selection, provider dispatch, run history, loop protection, and retry/idempotency remain future slices.

Candidate slices:

- `1.5.1` Trigger model: record created/updated, stage changed, date reached, form submitted, inbound email, webhook: foundation complete.
- `1.5.2` Condition/branching engine with AND/OR rules over record fields: foundation complete.
- `1.5.3` Action library: update field, create task, send email/SMS, assign owner, add to sequence, call webhook, notify: foundation complete.
- `1.5.4` Visual workflow builder UI: foundation complete.
- `1.5.5` Scheduled and time-delay actions on the background job runner (`0.9.3`): foundation complete.
- `1.5.6` Approval steps and human-in-the-loop actions.
- `1.5.7` Automation run history, error handling, and safe retry/idempotency.

Exit criteria:

- Admins can build multi-step automations visually and audit their runs.
- Automations execute reliably with guardrails against loops and duplicate actions.

## Version 1.6.0 - Reporting And Analytics

Status: planned.

Goal: move from fixed reports to a self-service analytics layer.

Candidate slices:

- `1.6.1` Custom report builder (choose object, fields, filters, grouping, aggregation).
- `1.6.2` Chart/visualization types (table, bar, line, funnel, pie, KPI).
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
