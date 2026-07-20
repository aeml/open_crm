# Performance And Failure Budgets

Last reviewed: 2026-07-19

These budgets are regression gates for the 5–50-person pilot profile. They are
not a capacity guarantee for an arbitrary host, a substitute for production
telemetry, or permission to expose unfinished bulk workflows. Production SLOs
and alert response remain in [`operations-runbook.md`](operations-runbook.md).

## PostgreSQL pilot read/write gate

`apps/api/internal/performance/pilot_load_postgres_test.go` runs as part of the
real-PostgreSQL backend CI job. It creates an isolated migrated schema with:

- 12 organizations;
- 1,000 contacts and tasks per organization;
- 500 companies and deals per organization;
- realistic pipelines/stages and tenant ownership relationships.

The gate first proves that representative tenant predicates for contacts,
companies, deals, and tasks use one of their reviewed organization-scoped
indexes. It then validates exact tenant totals, runs 96 mixed list reads through
the real services with 12 concurrent workers, and creates 32 contacts across two
tenants with eight concurrent workers. Every created contact is queried through
the other tenant to prove denial, and exact post-write totals are checked.

| Measure | Blocking budget |
| --- | ---: |
| Mixed service-read p95 | 500 ms |
| Any mixed service read | 2 s |
| Transactional contact-create p95 | 1 s |
| Any transactional contact create | 3 s |
| Mapped/deduplicated 1,000-row contact import | 10 s |
| Exhausted one-connection pool deadline | 200 ms |
| Closed database-pool failure | 1 s |

Current local Go 1.26/PostgreSQL 16.14 evidence was approximately 18.1 ms read
p95/21.1 ms maximum and 4.8 ms write p95/maximum. Those measurements are
diagnostic only; the checked budgets are intentionally wider to tolerate shared
CI hosts while still catching query-plan, transaction, or N+1 regressions. The
failure path also holds the only connection in a one-connection pool, proves a
waiting request observes its 200 ms deadline, releases capacity, and proves the
pool serves requests again. It separately holds an access-exclusive table lock,
proves a real service read honors the same 200 ms deadline, releases the lock,
and proves reads recover. The gate then exports the 10,000-row synchronous
contact ceiling, verifies valid tenant-isolated CSV, and budgets it at 5 seconds.
It inserts row 10,001 afterward and requires an explicit `ErrTooManyRows`
failure, preventing a partial file from masquerading as a complete export.
It also maps and writes the complete 1,000-row synchronous import ceiling,
including duplicate checks, activity/outcome ledgers, durable progress
checkpoints, exact tenant totals, and foreign-tenant absence, within 10 seconds.
Current local evidence was 806,068 export bytes in approximately 35 ms and a
52,937-byte/1,000-row import in approximately 512 ms. Test failure output
includes observed latency or the query plan/budget that regressed.

`apps/api/internal/modules/salesreports/query_plans_postgres_test.go` adds the
sales-milestone plan gate. It seeds roughly ten months of mixed stage events and
sales/non-sales activity across organizations and owners, analyzes the tables,
and requires the four report access paths to use:

- `idx_deal_stage_events_org_occurred` for tenant/date event reads;
- `idx_deal_stage_events_org_owner_occurred` for owner/date event reads;
- `idx_activities_sales_report_org_created` for tenant/date work totals;
- `idx_activities_sales_report_org_actor_created` for owner/date work totals.

The activity indexes are partial to the four actions the fixed report actually
aggregates and cover the actor/action values. Migration 69 bounds lock
acquisition at five seconds and index construction at two minutes, so a busy or
oversized installation fails the deployment safely instead of waiting without
limit. The ordinary sales-report PostgreSQL acceptance test remains the semantic
gate for exact totals, durable snapshots, disabled-user filters, and
cross-tenant denial.

The Postmark adapter is covered for provider `503`, later caller-controlled
recovery, and context deadlines. Durable sequence acceptance separately proves
that an ambiguous SMTP result is quarantined without an automatic duplicate send
and requires an audited operator decision. Remaining load/failure work before
pilot validation is a production-like host run with retained evidence plus the
provider and feature loads introduced by later convergence slices.

## Frontend bundle gate

`npm run build:checked` builds the production SPA and measures raw plus
level-9-gzip bytes using only Node's standard library.

| Asset boundary | Raw maximum | Gzip maximum |
| --- | ---: | ---: |
| Initial JavaScript entry | 190 KiB | 65 KiB |
| Any lazy JavaScript chunk | 60 KiB | 16 KiB |
| All JavaScript and CSS | 650 KiB | 207 KiB |
| All CSS | 20 KiB | 5 KiB |

Current evidence: 177.99 KiB/57.93 KiB entry, 36.32 KiB/10.47 KiB largest lazy
chunk, and 622.33 KiB/200.17 KiB total assets. The production contact, company,
deal, and task routes are 28.25/8.52, 31.90/9.50, 36.32/10.47, and 22.60/6.42
KiB raw/gzip respectively. Hosted billing, invoice/payment visibility, explicit self-hosted mode,
portable workspace export, and measured usage remain isolated in a 14.35 KiB/4.56 KiB settings route. Its
7.52 KiB/2.67 KiB background-operations route includes labeled replay, while a
0.15 KiB shared helper keeps retry-key generation consistent across billing,
signup, import, merge, and bulk recovery paths. Production builds now omit the
incomplete calling, SMS, calendar/booking-link, marketing-email, and
nurture-campaign management surfaces; the bundle gate rejects their accidental
inclusion. This restored aggregate headroom while making production exposure
match executable behavior.
Future frontend slices must remain within the ratcheted ceilings. The complete custom-field outcome
adds an isolated 6.66 KiB/2.27 KiB settings route plus shared typed forms,
filtering, import/export, and duplicate-review code. Archive recovery adds a
separate 5.51 KiB/2.20 KiB settings route instead of growing the near-budget
core record screens. Live data-quality, snapshot-backed sales activity, and
traceable stale follow-up queues leave the Reports route at 28.38 KiB/7.27 KiB;
reusable activity and touchpoint/account/client-health context remains outside
the parent record routes in 16.51 KiB/4.95 KiB and 17.52 KiB/4.99 KiB shared
chunks; the complete Clients route is 31.36 KiB/9.09 KiB. Admin pipeline
configuration and probability controls use an
isolated 7.21 KiB/2.46 KiB route and remove pipeline creation from the core Deals
route. Explainable period/stage forecasting and exact reminder buckets leave
Dashboard isolated at 20.40 KiB/5.51 KiB and split its panels from the route
orchestration source. Replacing
the non-executing general workflow builder with the bounded executable deal-task
surface reduces its lazy route from 20.62 KiB/5.86 KiB to 10.17 KiB/3.56 KiB.
The reminder workflow keeps its due counts and filter in the existing Tasks route
at 22.42 KiB/5.97 KiB and removes redundant browser-side time filtering now that
PostgreSQL owns those windows.
The production-complete touchpoint outcome raised the aggregate ratchets to
622/199 KiB. The stage-authoritative win/loss close review, reporting, and
export outcome then raised the raw aggregate ceiling to 626 KiB after
measurement. Making per-contact communication-state resets interaction-safe
raised the gzip aggregate ceiling by 1 KiB to 200 KiB after a measured 199.09
KiB build. The post-sale account outcome then moved shared account/touchpoint
context out of both parent record routes, reduced Contacts and Companies, and
raised only the measured aggregate raw ceiling by 2 KiB to 628 KiB. Explainable
client health then added the filtered client queue and task-health rollups while
keeping record detail context shared. Task-backed client review/renewal
scheduling then adds one reusable record component plus a focused dashboard
panel; the measured 641.96/203.16 KiB build raises only the aggregate ceilings
to 643/204 KiB. Verified self-serve workspace creation then adds a lazy 1.37
KiB/0.70 KiB verification route, recovery controls in the existing signup and
login routes, and the shared client calls needed to establish the first owner
session only after verification. Its measured 647.75/205.16 KiB build raises
only the aggregate ceilings to 650/207 KiB. Entry, per-route, and CSS limits
remain unchanged.
Hashes may change; the byte budgets do not. Raising a budget requires a measured
user outcome and an update to this document in the same reviewed slice.

The bundle budget limits delivery regressions but does not by itself make large
route source files maintainable. Tested contact splits moved the list, editor,
view model, calls, recording controls, SMS, meetings, email, sequences, lead
scoring, attribution, related deals, notes, tasks, and activity into focused
modules. Shared collaboration-aware record-work cards now serve contacts, companies, and deals;
company editor/view helpers, deal quote/signature/view helpers, and task view
logic are also separated. Bulk-action, custom-field, reminder, touchpoint/health,
and client-review integration plus focused development-only communications and
production outreach and lead-score orchestrators plus focused company-directory and linked-
people presentation plus shared record selection/work leave the parent routes at 644 contact lines,
766 company lines, 764 deal lines, and
737 task lines, down from 2,038, 1,364, 1,365, and 1,093 respectively, without
changing their lazy-load boundaries. Tested 68-line selection and 142-line work
hooks now serve contacts, companies, and deals, abort obsolete loads, distinguish repeated
A-to-B-to-A visits, serialize per-record mutations, validate record/work
identities, and keep late saves and work off the active contact. The 229-line
contact outreach hook also
clears record-scoped email and sequence state on selection changes and rejects
late responses from prior selection epochs. A 59-line lead-score hook rejects
duplicate evaluations, wrong-contact responses, and late results across
leave-and-return navigation. Deal directory, shared form, and editor
presentation live in focused 157-, 74-, and 87-line modules. The shared selection
and work hooks apply the same visit identity, serialization, work, and response
validation contract to deals; the guarded
185-line commercial hook rejects stale or mismatched quote/proposal results.
The shared record-work follower control also resets on record changes and
rejects late responses from an earlier contact, client, or deal.
A tested 88-line task quick-action hook prevents concurrent mutations of the
same task, validates response identity, and updates the active detail only when
it still represents that task; delayed completion cannot pull navigation back
to an earlier record. Full-form task saves now use the same task-visit identity,
suppress duplicate submission, and cannot navigate after the task route unmounts.
Narrowing the normal automation UI to its
executable task-rule subset also reduced that route from 669 to 261 lines.
Continue lowering the remaining company/deal/task exceptions along tested
orchestration seams.

The API composition root is 369 lines, down from 996. Its audited 205-route
surface is registered through 142-line platform, 246-line foundation, and
267-line core-CRM files. The security inventory and hosted-write-policy tests
scan all production files in the package, so splitting registrations cannot
silently remove a route from either guard. Shared handler helpers are isolated
in a 258-line file and `support_handlers.go` is 455 lines, so every production
file in `internal/app` now uses the default 500-line ceiling.

## Source-size no-growth ratchet

Source size is an imperfect complexity measure, so these limits are a temporary
no-regression ratchet rather than a claim that a file below the limit is well
designed. CI runs `npm run check:source`; backend tests apply the same rule to
the application composition package.

| Existing hotspot | Current lines | Maximum until next split |
| --- | ---: | ---: |
| `contacts.jsx` | 644 | 650 |
| `companies.jsx` | 766 | 775 |
| `deals.jsx` | 764 | 775 |
| `tasks.jsx` | 737 | 750 |
| `dashboard.jsx` | 477 | 550 |

All other production route files and every production `internal/app` Go file
are limited to 500 lines. Each successful split should lower or remove its
exception in the same slice. Tests are excluded because large fixture-driven
flow tests have different review tradeoffs; lint and normal test execution
continue to gate them.
