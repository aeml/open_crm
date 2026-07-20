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
- Many Part II capabilities persist definitions and expose management UI but do not perform the promised job. Examples include Stripe billing, real telephony/calendar providers, marketing delivery, workflow trigger/action execution, report query execution, public booking, and real e-signature.
- Fake providers are useful development seams, but billing, telephony/SMS, and calendar currently have no real provider implementation. Environment variables advertise integrations that the runtime does not load or implement.
- At audit time, the in-process mailbox, sequence, and reminder workers were not a general durable job system. This was resolved by the PostgreSQL job/outbox slice recorded in the capability matrix and roadmap.
- At audit time, production deploys were triggered independently of the main CI workflow. Deploy workflows now require all successful CI jobs, publish immutable commit-tagged API images, verify exact release identity, enforce expand/contract migration policy, and automatically restore the last healthy image after failed readiness.
- At audit time, public abuse counters were process-local. Public credential, signup, lead, provider-webhook, unsubscribe, and tracking budgets now use atomic privacy-preserving PostgreSQL windows shared across processes, fail closed on store errors, and emit alertable decisions; an approved production edge/WAF and lead-form bot/consent controls remain.
- At audit time backups/restores were manual and time-series monitoring was absent. Encrypted Restic backup verification, retention, CI acceptance, isolated scheduled-drill tooling, protected operational metrics, validated alert rules, redacted logging, and initial SLOs now exist; approved off-host credentials/timer activation, production scrape/alert-destination validation, and a load/failure suite remain.
- At audit time, the frontend built as one roughly 531 kB minified JavaScript chunk. Route-level lazy loading now keeps the entry at 177.99 KiB and CI enforces entry/lazy/total/CSS raw+gzip budgets. Focused presentation components reduce the company and deal parent routes from 984 and 1,064 lines to 863 and 887 under tightened 900-line ceilings. The API composition root is now 369 lines, with all 204 explicit registrations split into sub-500-line platform, foundation, and core-CRM files and covered by package-wide inventory/write-policy guards; the remaining record routes and support-handler aggregation remain maintainability hotspots.
- At audit time, the full npm audit reported eight development-tool vulnerabilities, including high/critical findings, and the API/container still used unsupported Go 1.23 and Alpine 3.20. Supported Node 24, Go 1.26, and Alpine 3.24 toolchains are now pinned; the frontend high-severity audit and Go reachable-vulnerability scan are CI-gated.
- At audit time, there were no browser-level end-to-end smoke tests. CI now runs a Chromium pilot journey against disposable PostgreSQL 16; the larger Phase 2–4 outcomes still need to extend that journey as they become executable.
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
