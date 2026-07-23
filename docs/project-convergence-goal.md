# Open CRM Project Convergence Goal

Audit date: 2026-07-19

This document turns the current codebase and roadmap into one execution goal. It is deliberately outcome-oriented: Open CRM has enough feature breadth, so the next stage should finish, harden, and validate complete user workflows before adding another product category.

The assessment below is the audit baseline that established this execution order, not a live status page. Resolved slices remain here as historical evidence; `docs/capability-matrix.md` is the canonical current-state source and `docs/professionalization-roadmap.md` records implementation evidence.

## Current-state assessment

Open CRM has a strong engineering base:

- A clean modular-monolith direction: Go `net/http`, React/Vite, PostgreSQL, plain SQL, server-side sessions, Docker, and a deliberately small dependency set.
- At audit time, a substantial implemented surface: 55 database migrations, about 43,000 lines of Go, about 24,000 lines of frontend source, 93 Go test files, and 46 frontend test files.
- Working core CRM flows for contacts, companies, deals, tasks, notes, activity, saved views, exports, audit events, notifications, ownership, role gates, and tenant scoping.
- Broad post-MVP foundations for billing, email and mailbox sync, sequences, shared inboxes, calling, SMS, meetings, product/quote data, lead capture, campaigns, automation definitions, and report definitions.
- At audit time, healthy baseline checks: Go tests, `go vet`, formatting, 103 frontend tests, frontend lint, and the production build passed locally.

The codebase is not yet at the roadmap's final product outcome:

- The roadmap says Part I should be finished before Part II, but much of `0.4.5` through `0.10.0` remains planned while Part II foundations have advanced through `1.6.x`.
- Several roadmap statuses conflict between the summary and detailed sections. The README still presents a portfolio-oriented MVP surface while the roadmap describes a commercial SaaS competitor.
- Many Part II capabilities persist definitions and expose management UI but do not perform the promised job. Examples include real telephony/calendar providers, marketing delivery, broad workflow action/scheduling execution, report dashboards/sharing/scheduled delivery, and public booking. Workflow execution now has bounded deal-event and conditional lead-form outcomes: deal rules atomically create an explicitly versioned 1–5-task playbook with independent due dates and can use one explicitly versioned typed event-time snapshot condition without activating legacy definitions, while lead tasks retain exactly one immutable whole-day creation schedule, separate due offset, and reversible operator spam quarantine/recovery before future qualification or routing. Exact `deal_approval_task_plan_v1` and `deal_task_notify_plan_v1` contracts can respectively pause a captured 1–5-task deal plan for one owner/admin/current-owner decision or append one literal same-transaction in-app notification after all tasks commit; they are mutually exclusive. Exact `deal_assign_owner_v1` rules can instead assign one active same-tenant owner after deal creation, stage change, or a direct owner edit. That action preserves the ordinary preference-aware assignment notification, records typed changed or no-op evidence, and emits a nested owner-change event from the exact successful parent action. Notification membership is revalidated at event time, capped at 50 active recipients, idempotent per run/action/recipient, and rolls back the whole event on zero/overflow recipients. Immutable action snapshots, current actor/requester/ownership checks, cancellation on definition changes or requester deactivation, audit/activity/export evidence, approval/loop metrics and alerting, rolling and fresh PostgreSQL tests, and Chromium acceptance cover those bounded outcomes. Every nested event must identify an exact successful same-tenant parent action; ancestor re-entry, execution beyond eight causal hops, and additional fan-out after 50 existing runs in one causal tree are retained and audited as skipped runs. All supported paths retain immutable ordered per-action evidence, including captured labels, lifecycle state, attempts, schedules, terminal reasons, tenant-validated task or owner outputs, notification delivery count, and causal identity. Lead inspection also reconciles the exact tenant-scoped durable queue state, so retryable/dead work is not mislabeled as perpetually running and an admin can invoke the existing audited replay from retained trigger evidence. All other action, schedule, and branch families remain hidden. Saved table and explicitly versioned grouped numeric bar reports now share a bounded tenant-safe screen, exact accessible table, and audited CSV runtime; pre-contract bar metadata and line/funnel/pie/KPI charts remain hidden. Stripe and first-party quote signing have executable production-equivalent contract paths, but both remain below pilot-validated maturity and still have named completion work.
- Fake providers are useful development seams, but billing, telephony/SMS, and calendar currently have no real provider implementation. Environment variables advertise integrations that the runtime does not load or implement.
- At audit time, the in-process mailbox, sequence, and reminder workers were not a general durable job system. This was resolved by the PostgreSQL job/outbox slice recorded in the capability matrix and roadmap, including bounded multi-instance-safe retention for successful work without deleting active or dead-letter recovery evidence.
- At audit time, production deploys were triggered independently of the main CI workflow. Deploy workflows now require all successful CI jobs, publish immutable commit-tagged API images, verify exact release identity, enforce expand/contract migration policy, preflight the new migration image and protected database credentials before an existing PostgreSQL container can be recreated, and automatically restore the last healthy image after failed readiness. Disposable acceptance proves credential drift leaves the current API/database identities, release state, and readiness untouched.
- At audit time, public abuse counters were process-local. Public credential, signup, lead, provider-webhook, unsubscribe, and tracking budgets now use atomic privacy-preserving PostgreSQL windows shared across processes, fail closed on store errors, and emit alertable decisions. Lead writes also require a digest-only, delayed, one-time form challenge and explicit challenge-bound consent, collapse exact retries transactionally, and no longer collect new raw client addresses or user agents; an approved production edge/WAF remains required for volumetric and reputation defenses.
- At audit time backups/restores were manual and time-series monitoring was absent. Encrypted Restic backup verification, retention, CI acceptance, isolated scheduled-drill tooling, protected operational metrics, validated alert rules, redacted logging, initial SLOs, and local read/write/query/database/provider failure budgets now exist; approved off-host credentials/timer activation, production scrape/alert-destination validation, and production-like host evidence remain.
- At audit time dashboard query performance and cross-panel consistency were unreviewed. The fixed operational dashboard now returns deal/task/contact/activity/client-review and explainable forecast panels from one tenant-scoped repeatable-read snapshot under a five-second deadline. Quota mutation and its returned snapshot share one serializable transaction with active actor/target revalidation, bounded serialization retries, and proven pre-commit timeout rollback. Migration 118 adds measured recent-activity/contact indexes; fresh and rolling migration tests plus equal two-tenant 10,000-deal/contact/task and 20,000-activity acceptance prove exact isolation, bounded plans, a below-two-second regression budget, and forced-timeout recovery. Configurable report dashboards remain intentionally incomplete and hidden.
- At audit time list pagination had no complete route-level inventory and the four core services accepted arbitrary positive page sizes/offsets. All 106 registered GET routes are now digest-gated to an explicit cardinality/ordering/overflow review. Contacts, companies, deals, and tasks share a handler-and-service page contract capped at 100 rows and a 50,000-row offset, with overflow-safe parsing plus adjacent-page stability, no-overlap, tenant-separation, and maximum-plus-one PostgreSQL evidence. Offset paging remains the measured compatible choice until an approved larger workload or query-plan regression justifies a keyset contract. Record-local notes and contact/company/deal/task activity use a bounded opaque `(created_at,id)` keyset cursor, 50-row detail first pages, visible older-history continuation, and equal-time/concurrent-insert/final-page/tenant PostgreSQL evidence. Company linked people now use a bounded 50-row detail embed plus searchable offset continuation, exact totals, explicit load-more UI, relationship-only writer endpoints, stable primary/account context, generic-edit data-loss prevention, atomic individual-person replacement with unlink-to-zero protection, tenant/role/browser acceptance, and a 1,000-link budget. Shared inbox now uses a strict 50/default, 100/max snapshot-bound opaque cursor over its open/closed bucket, effective received time, and ID, with visible continuation plus 1,001-row PostgreSQL and Chromium evidence. Lead review uses the same strict limits with an immutable creation-time/ID cursor, visible ID-deduplicated continuation, exact form-scoped status counts, mutation refresh, a 1,001-row PostgreSQL/index budget, and Chromium row-51 evidence. Product catalog management now uses exact searchable/status-filtered 50/default and 100/max pages, service-level writer revalidation, a concurrency-safe 100-active-item ceiling, pageable inactive history, and active-only quote selection with 1,001-row PostgreSQL and row-51 Chromium evidence. Quote-template administration now has the same searchable/status-filtered page and offset bounds, exact repeatable-read totals, a serialized 100-active-template ceiling, pageable archived history, and a complete active-only deal selector that detects page drift and preserves legacy overflow; 1,001-row PostgreSQL and row-51 Chromium evidence cover it. Email templates and snippets now use independent exact searchable 50/default and 100/max pages, repeatable-read totals, exact-revision update/delete, transactional writer/audit boundaries, separate serialized 100-stored ceilings, and complete drift-detecting composer loaders; 1,001-row-per-catalog PostgreSQL plus row-51/WCAG Chromium evidence cover both. Email-sequence definition management now adds exact repeatable-read searchable/status-filtered pages with the same bounds, transactional actor revalidation and lifecycle audit, exact-revision update/delete/approval, a serialized 100-active ceiling, selected-page outcome hydration, and a complete drift-detecting active-only selector that preserves legacy overflow; 1,001-row PostgreSQL and row-51 SMTP/WCAG Chromium evidence cover it. Saved-view management now uses exact repeatable-read 50/default and 100/max personal pages, stable default/name/ID order, exact-revision update/delete, transactional writer revalidation, serialized default changes and a 100-stored-per-user/entity ceiling, plus a complete drift-detecting legacy-safe loader; 1,001-row PostgreSQL and row-51/create/default/update/delete/WCAG Chromium evidence cover it. Workflow-definition management now uses stable 50/default and 100/max pages, a 50,000-row maximum offset, exact stored-definition and active-action summaries from one repeatable-read snapshot, visible continuation, mutation refresh, and 1,001-row PostgreSQL plus row-51 Chromium evidence. Saved-report-definition management now uses the same stable page and offset bounds with an exact same-snapshot tenant total, active/update/ID order, visible continuation, mutation refresh, and matching 1,001-row PostgreSQL plus row-51 Chromium evidence; only the other mutable definition catalogs retain explicit scale decisions rather than silent caps.
- At audit time, the frontend built as one roughly 531 kB minified JavaScript chunk. Route-level lazy loading now keeps the entry, every lazy chunk, aggregate assets, and CSS within CI-gated raw/gzip budgets. Focused contact/company/deal/task orchestration reduces their parent routes from 2,038, 1,364, 1,365, and 1,093 lines to 453, 463, 474, and 298 with no production-route exception to the 500-line ceiling. The tested 275-line task-directory hook owns filters, URL/history synchronization, bootstrap/options, loading, and request identity: a late initial or superseded list response cannot replace newer work, while one failed form-option request does not discard successful options. Selection-aware loading and guarded snapshot mutations reject obsolete cross-record responses, serialize incompatible commercial actions, suppress duplicate work, and validate returned identities. Production now includes exact client-period activity and pipeline-entry cohort conversion/velocity reports plus bounded saved-table and explicitly versioned grouped-bar builder/result/export paths while rejecting pre-contract bars and incomplete custom line/funnel/pie/KPI, dashboard, audience/scoring, booking, calling/SMS, and marketing/nurture management surfaces. The current measured build is 178.87/57.99 KiB entry, 55.12/15.73 KiB largest lazy chunk, and 778.16/242.36 KiB aggregate raw/gzip under reviewed 779/243 KiB aggregate ceilings; every entry/per-route/CSS ceiling remains unchanged. The task route itself is 26.09/7.92 KiB raw/gzip. The shared guarded saved-view manager is 233 lines. The product-catalog route is 7.72/2.55 KiB, bounded quote-template administration is 11.73/3.76 KiB in a 340-line route, the lead-forms route is 15.83/5.14 KiB, bounded email-template/snippet management is 11.02/2.92 KiB in a 346-line route, and bounded sequence definition/history management is 12.91/3.98 KiB in a 313-line route plus the 160-line drill-down. The Reports route is 50.52/12.07 KiB, with separate 162-line client-period and 193-line pipeline-cohort components, 352-line saved-report orchestration, 245-line pure tested catalog/form model, and focused 26/33-line table/bar renderers. The 279-line Operations route adds durable filtered-export request, progress, failure, download, and replay visibility. The task-automation route is 433 lines with a separately tested 345-line executable-contract/form model, a 67-line run-inspection component, and a 51-line approval queue; its 40.13/11.17 KiB chunk exposes bounded teammate-notification authoring/delivery plus root/nested causal and loop-guard evidence alongside immutable task/approval outcomes while staying below the unchanged per-route budget. The shared record-email composer is 362 lines. The 426-line request composition file and all production `internal/app` files remain below 500 lines; the service-contract catalog and explicit runtime dependency container are separate 438- and 70-line files. Session-cookie authentication, public onboarding, and tenant user lifecycle live in focused 90-, 125-, and 296-line handlers instead of one 491-line security-domain mix, with unchanged route behavior and the reviewed audit-producer inventory. Authenticated email-message/inbox operations, response projection/privacy decisions, and public open/click tracking now live in focused 224-, 221-, and 56-line files instead of one 487-line mixed handler; every click outcome, including an invalid or unsafe target, receives the same no-store/no-referrer/no-index boundary. All 265 explicit route registrations are split across sub-500-line registration files and covered by package-wide inventory and hosted-write-policy guards.
- The workflow module's former 1,352-line service mixed public models, definition persistence, run persistence, definition validation, and condition evaluation. Those responsibilities remain behind a package-wide 500-line CI ratchet; approval remains in focused 119-line storage, 252-line decision, 93-line finalization, 149-line cancellation, and 133-line capture files, while causal checks and notification execution add focused 119- and 93-line boundaries rather than rebuilding a mixed service. The primary deal-rule runtime remains below the ratchet at 476 lines.
- At audit time, the full npm audit reported eight development-tool vulnerabilities, including high/critical findings, and the API/container still used unsupported Go 1.23 and Alpine 3.20. Supported Node 24, Go 1.26, and Alpine 3.24 toolchains are now pinned; the frontend high-severity audit, Go reachable-vulnerability scan, zero-unsuppressed-finding Go static-security analysis, and a reproducible shipped-dependency license inventory with generated third-party notices are CI-gated.
- At audit time, there were no browser-level end-to-end smoke tests. CI now runs an unmanaged/self-hosted Chromium pilot journey and a separate managed Stripe-mode lifecycle journey against isolated disposable PostgreSQL 16 databases, plus automated WCAG A/AA scans of critical public, core-record, team, import, reporting, billing-state, and export surfaces. The Stripe journey drives the production adapter through a local HTTP sandbox and proves signed-event authority, dunning/recovery, cancellation, direct-write enforcement, and suspension-safe export without replacing approved provider evidence; the remaining Phase 2–4 outcomes still extend these journeys as they become executable.
- The original mapped CSV write held an HTTP request through as many as 1,000 rows. Migration 119 now commits its batch, seven-day recovery source, audit evidence, and `import.execute` job atomically, then the leased worker revalidates actor/capacity and resumes 50-row checkpoints while the UI exposes progress and Operations replay. Success clears source bytes immediately, expiry clears unresolved bytes, deactivation quiesces pending work, and portable exports omit raw uploads. Migration 120 adds a distinct tenant/requester-bound filtered CRM export ledger: exact contact/company/deal/task criteria, idempotency conflict detection, the `crm.export.generate` job, 500-row progress checkpoints, 50,000-row/50 MiB limits, SHA-256, five retained artifacts, seven-day cleanup, audited request/readiness/download, active-actor revalidation, deactivation quiescence, Operations recovery, and exact-filter handoff from all four core lists. The original 10,000-row synchronous path remains for fast bounded downloads, completing `0.9.4` locally.
- At audit time, audit events had an admin list but no executable retention/export contract. PostgreSQL now enforces workspace-lifetime append-only history and rejects secret-like top-level metadata, typed and nested JSON reads safely, the admin surface provides an audited exact-filter CSV with explicit 10,000-row refusal, and the complete workspace ZIP carries the full audit dataset. A digest-gated producer inventory forces policy review when the source set changes.
- At audit time, the original open-core aspiration, current MIT license, hosted SaaS plans, and commercial feature boundary had not been reconciled. `docs/product-vision.md` now records the current MIT plus managed-hosting position and requires a separate explicit legal/business decision before any licensing boundary changes.

## Improved final vision

Open CRM should become the most trustworthy open and self-hostable revenue-and-client operations CRM for 5–50-person B2B service teams, with an optional managed SaaS offering.

It should win through:

- Fast onboarding and data portability.
- A coherent lead-to-client workflow: capture, qualify, assign, communicate, progress a deal, quote, close, and hand off to delivery.
- Excellent email and follow-up workflows without forcing a large enterprise suite.
- Clear ownership, permissions, auditability, and tenant isolation.
- Reliable automation and reporting that do real work, not merely store definitions.
- Simple deployment and operations for self-hosters, plus safe billing and tenant lifecycle management for the hosted product.
- Focused, responsive, accessible UI with progressive disclosure instead of exposing every foundation as a top-level product feature.

This is a stronger target than “match every established CRM.” Head-to-head breadth against HubSpot, Salesforce, Zoho, Pipedrive, Close, and Copper is not a useful near-term finish line for a small project. Open CRM should first be unmistakably good for one customer profile and one complete operating loop. AI, native mobile apps, a marketplace, custom objects, help desk breadth, enterprise compliance, and multi-region infrastructure remain later options that require pilot evidence.

## Execution order

### Phase 0 — Establish one source of truth

- Freeze new feature families while convergence work is underway.
- Replace contradictory version prose with a capability matrix whose states are: `not started`, `schema/API foundation`, `usable with fake provider`, `production-capable`, and `validated with a real provider/pilot`.
- Reconcile README positioning, product vision, roadmap status, environment configuration, screenshots, and release/version metadata with the executable code.
- Map every route and background operation to its authentication requirement, role, tenant boundary, plan entitlement, rate limit, observability, and test coverage.
- Decide and document the community/hosted distribution and licensing strategy before relying on “open core” as the business model.

Exit criteria:

- Documentation contains no known status contradictions.
- Every exposed capability has an honest maturity state and named remaining outcome.
- One pilot customer profile and one lead-to-client journey are the explicit product priority.

### Phase 1 — Close production trust gaps

- Update supported runtime/development dependencies and make the full audit policy explicit. Remove the current high/critical development-tool findings.
- Make deployment depend on successful required CI checks. Add safe migration/deploy rollback guidance and a post-deploy smoke check.
- Add a small browser-level suite against a real PostgreSQL database for signup/login, invite, contact/company/deal creation, task completion, email-provider sandbox behavior, and tenant isolation.
- Implement a durable PostgreSQL-backed job/outbox model with transactional claims, idempotency keys, bounded retries, dead-letter visibility, and operator replay. Move sequence sends, mailbox sync, reminders, and future automation/campaign delivery onto it.
- Automate encrypted off-host backups, retention, restore verification, and scheduled restore drills.
- Add actionable metrics and alerts for request errors/latency, database health, job lag/failures, email/provider failures, and backup freshness.
- Rate-limit and protect public signup, forms, landing pages, widgets, tracking, unsubscribe, and future webhook endpoints against abuse.
- Refactor the largest API/frontend hotspots along domain seams and lazy-load route bundles, preserving behavior and tests.

Exit criteria:

- A failed required quality gate cannot deploy.
- No known high/critical dependency findings remain under the chosen audit policy.
- Worker actions are safe across restarts and multiple API instances, and duplicate external effects are covered by tests.
- A restore drill and a failed-job replay can be completed from the runbook.
- The critical browser journey passes in CI against PostgreSQL.

### Phase 2 — Finish the pilot-ready core CRM

- Complete team lifecycle: disable/reactivate users, reassign owned work, invalidate sessions, preserve history, and audit every transition.
- Complete collaboration: record followers, mentions, useful notifications, and a focused activity digest.
- Complete data operations: import mapping and write, validation, idempotency, rollback, bulk edits/archive/reassignment, duplicate detection/merge, archive/restore, and data-quality review.
- Complete adaptable CRM data: organization-defined custom fields with validation, filtering, export, permissions, and migration-safe storage.
- Complete sales configuration: pipeline/stage management, probabilities, win/loss reasons, reminders, and coherent forecast behavior.
- Complete the post-sale handoff needed by the target service team without creating a separate help-desk product.
- Validate responsive, keyboard, and screen-reader behavior on the critical journey.

Exit criteria:

- A 5–50-person pilot team can migrate real data, manage access safely, run its daily work, recover mistakes, and export its data without developer assistance.
- The lead-to-client journey has browser-level acceptance coverage.
- No critical workflow depends on direct database repair or a fake provider.

### Phase 3 — Make the hosted product commercially real

- Add verified self-serve signup, tenant provisioning, trial lifecycle, and abuse controls.
- Implement Stripe customers, Checkout/customer portal, subscriptions, invoices, signed/idempotent webhooks, proration, dunning, and reconciliation.
- Centralize plan entitlements and subscription write enforcement so every relevant API/background path is covered consistently.
- Meter seats, records, messages, automation executions, API use, and storage with explainable usage views.
- Complete suspension, cancellation, retention, tenant export, and deletion workflows.
- Add at least one production provider for each communication capability that remains in the pilot scope. Hide or label unconfigured foundations rather than presenting them as finished features.
- Document privacy, consent, deliverability, suppression, recording, and data-processing behavior.

Exit criteria:

- A new customer can sign up, verify identity, start a trial, pay, invite a team, understand limits, cancel, and export/delete data without operator intervention.
- Provider webhook retries and duplicate events are safe and observable.
- Feature availability is consistent across UI, API, and workers.

### Phase 4 — Finish existing differentiators one at a time

Complete one family to its user outcome before starting the next:

1. Email/shared inbox/sequences: reliable send and sync, bounce/complaint handling, reply detection, deliverability guidance, sequence approvals, compliance, and analytics.
2. Quotes/signature: versioned branded quotes, delivery, a real signing ceremony/provider, audit certificate, and closed-deal conversion.
3. Workflow automation: event capture, hydrated conditions, durable action execution, approvals, retries, loop protection, and useful run inspection.
4. Reporting: safe runtime query execution, tables/charts, dashboards, sharing, exports, scheduled delivery, and performance limits.

Marketing campaigns, public booking, telephony/SMS, and other existing foundations should be promoted only when their real provider/runtime loop and compliance requirements are finished. Otherwise they remain clearly labeled foundations or are hidden from normal navigation.

Exit criteria:

- Each promoted family satisfies its roadmap exit criteria with a real integration or production-equivalent sandbox, observability, failure recovery, and pilot validation.
- No roadmap item is called complete solely because its schema, definition editor, or fake-provider path exists.

### Phase 5 — Expand from evidence

- Collect pilot activation, retention, task completion, response-time, pipeline progression, and support-friction evidence using privacy-respecting product metrics.
- Choose subsequent work from observed customer pain.
- Consider AI, support/tickets, marketplace/custom objects, real-time collaboration, native mobile, SSO/SCIM, and enterprise controls only when evidence justifies their operational and product cost.

## Definition of done for every slice

A slice is complete only when it includes, as applicable:

- A user-visible outcome and acceptance scenario.
- Tenant-scoped data design, constraints, indexes, and reversible migration/rollback thinking.
- API behavior with stable errors, authentication, roles, entitlements, validation, body limits, and abuse controls.
- UI behavior with loading, empty, error, permission, plan-limit, responsive, keyboard, and accessibility states.
- Unit, handler, database integration, cross-tenant, and browser acceptance tests proportional to risk.
- Idempotency, retries, timeout behavior, and recovery for external effects.
- Structured logs, metrics, audit events, and an operator-visible failure path.
- Updated README/capability matrix/roadmap/runbook and real screenshots where relevant.
- Passing formatting, vet, tests, lint, build, dependency audit, migration, and smoke checks.
- Validation with a real provider sandbox or pilot when the feature claims external integration readiness.

## Copy/paste goal prompt

```text
Turn Open CRM from a broad collection of well-tested feature foundations into a trustworthy, pilot-ready, self-hostable CRM with an optional commercially operable hosted SaaS offering for 5–50-person B2B service teams.

Use docs/project-convergence-goal.md as the governing execution brief. Preserve the Go/React/PostgreSQL modular-monolith architecture, explicit SQL, server-side sessions, tenant isolation, minimal-dependency philosophy, and existing working behavior. Do not pursue feature-count parity with enterprise CRM suites and do not start another feature family while a higher-priority foundation or end-to-end workflow is incomplete.

First establish an evidence-backed source of truth: audit executable behavior, tests, migrations, provider implementations, CI/deploy behavior, operations, and docs; create an honest capability matrix using the maturity states “not started,” “schema/API foundation,” “usable with fake provider,” “production-capable,” and “validated with a real provider/pilot”; reconcile roadmap and README contradictions; and explicitly define the target customer and critical lead-to-client journey.

Then execute the phases in order:
1. Production trust: dependency/runtime hygiene, deploys gated by CI, PostgreSQL browser smoke tests, durable idempotent jobs/outbox, automated backups and restore drills, monitoring/alerts, public-endpoint abuse controls, and maintainability/code-splitting hotspots.
2. Pilot-ready CRM: safe user disable/reactivate/reassignment, mentions/follows, complete imports and rollback, bulk operations, duplicate merge, custom fields, archive/restore, configurable sales workflow, and post-sale handoff.
3. Commercial SaaS: verified signup, real Stripe billing/webhooks/dunning, centralized entitlements and metering, suspension/cancellation/export/delete, and production provider integrations for the capabilities kept in pilot scope.
4. Existing differentiators, one complete family at a time: email/inbox/sequences, quotes/signature, workflow execution, and runtime reporting/dashboards. A schema, management UI, fake provider, or stored definition is not completion.
5. Expansion only from pilot evidence; defer AI, help desk, marketplace/custom objects, native mobile, real-time, and enterprise breadth until justified.

For every slice, deliver the user outcome across database, API, permissions, plan enforcement, UI, accessibility, tests, observability, failure recovery, docs, and operations. Test cross-tenant and forbidden paths. Make external effects idempotent and replayable. Keep changes small, reviewable, and vertically complete. Preserve unrelated user changes. Never mark an item complete until its acceptance criteria pass and its maturity label is truthful.

Start with Phase 0 and Phase 1. Maintain a short current plan, verify after each slice, update the capability matrix and roadmap as part of the same change, and continue through the highest-priority safe work until the pilot-release exit criteria are genuinely satisfied or an external credential/business decision is the only remaining blocker. When blocked by a provider or product decision, leave the codebase in a verified state and report the exact decision, evidence, options, and recommended choice.
```
