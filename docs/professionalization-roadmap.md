# Open CRM Professionalization Roadmap

This roadmap starts at version `0.1.1` and moves the project from MVP-complete to professional-grade without changing the core product direction: a small, explicit CRM built from a Go API, React web app, and Postgres.

Each version is a shippable slice. The goal is to improve safety, reliability, maintainability, and operator trust without introducing unnecessary platform complexity.

After `0.3.0`, the baseline infrastructure work is complete. Future versions should be driven by product usefulness, operator workflows, and measured reliability improvements rather than speculative architecture.

## Progress

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
- `0.4.4` Notification Preferences: planned.
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

Status: planned.

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
