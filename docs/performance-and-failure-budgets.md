# Performance And Failure Budgets

Last reviewed: 2026-07-22

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
- 1,001 activities on one representative record for cursor-page evidence;
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
| Core list page size / maximum offset | 100 rows / 50,000 rows |
| Two adjacent record-history cursor pages (1,001 rows, 100/page) | 2 s |
| Each of the first two shared-inbox cursor pages (1,001 rows, 100/page) | 2 s |
| Each of the first two filtered lead-review cursor pages (1,001 rows, 100/page) | 2 s |
| Each adjacent email-template and snippet management page (1,001 definitions/catalog, up to 100 returned) | 2 s each |
| Each adjacent personal saved-view management page (1,001 definitions, up to 100 returned) | 2 s each |
| Active and adjacent email-sequence definition pages (1,001 definitions, up to 100 returned) | 2 s each |
| Company linked-person page (1,000 links, 100 returned) | 2 s |
| Transactional contact-create p95 | 1 s |
| Any transactional contact create | 3 s |
| Client-period activity page (500 clients, 100 returned) | 2 s |
| Saved-report page (100 rows) | 2 s |
| Core or saved-report export (10,000 rows) | 5 s |
| Mapped/deduplicated 1,000-row contact import | 10 s |
| Exhausted one-connection pool deadline | 200 ms |
| Closed database-pool failure | 1 s |

Current local Go 1.26/PostgreSQL 16.14 evidence was approximately 26.2 ms read
p95/48.5 ms maximum and 11.2 ms write p95/maximum. Those measurements are
diagnostic only; the checked budgets are intentionally wider to tolerate shared
CI hosts while still catching query-plan, transaction, or N+1 regressions. The
same fixture now reads adjacent maximum-size pages for contacts, companies,
deals, and tasks, repeats the first page against an unchanged dataset, and
requires exact totals, stable ordering, no adjacent-page overlap, and separate
tenant results. Direct service calls must reject page sizes above 100 and
offsets above 50,000 before querying PostgreSQL; handler tests independently
require malformed, non-positive, oversized, and excessive requests to return
`400` before calling a service. The shared calculation checks bounds before
multiplication, so an extreme page number cannot overflow into a small offset.
The 50,000 ceiling matches the largest advertised hosted contact/deal capacity;
[`list-endpoint-inventory.md`](list-endpoint-inventory.md) records why offset
pagination remains the compatible measured choice and the exact trigger for a
future keyset contract.

Record-local note and activity history uses a separate keyset shape because new
events commonly arrive while an operator reads older work. The gate seeds 1,001
activities for one contact, reads two adjacent 100-row pages using the opaque
`(created_at,id)` cursor, and requires both requests together to finish within
two seconds without a repeated boundary row. Dedicated PostgreSQL acceptance
also fixes equal-timestamp order, inserts a newer row between page requests,
requires complete terminal-page traversal, and proves a foreign tenant receives
no rows. Migration 108 supplies matching tenant/entity/time/ID indexes for notes
and activities with bounded deployment lock and statement timeouts.

Company linked people use the compatible core offset shape because exact totals,
explicit page continuation, and search are part of relationship management. A
dedicated freshly migrated PostgreSQL acceptance seeds 121 linked people to
prove 50/50/final-page order and no overlap, exact search by name/email/title,
foreign denial, bounded detail, and preservation of every unseen relationship
during an ordinary client edit. The same test seeds 1,000 links and requires a
100-row page plus exact total under two seconds. Migration 109 adds the matching
tenant/company/contact access path with bounded deployment lock and statement
timeouts. Primary changes serialize on the company row, unchanged PUTs add no
duplicate activity, archived primaries cannot block an active replacement, and
unlink deterministically promotes one remaining active person. Individual-client
PUT atomically replaces the sole link, while DELETE cannot create a zero-link state.

Shared inbox uses a separate mutable-queue keyset. Its default is 50 and maximum
is 100; malformed cursors and limits fail before service work, and direct callers
repeat the bound. The opaque cursor retains the first-page database snapshot,
open/closed work bucket, effective received time, and ID. Rows arriving or being
assigned/closed after that snapshot wait for refresh, preventing an already seen
open message from moving into a later closed page. Dedicated freshly migrated
PostgreSQL acceptance traverses 1,001 equal-time rows, requires the first two
100-row requests individually to finish within two seconds, asserts the cursor
index plan, final-page termination, direct overflow rejection, and foreign/private/
outbound exclusion, then proves both a newer arrival and an updated first-page row
do not enter continuation while a refresh exposes current state. Migration 110
adds the matching tenant/bucket/time/ID partial index with bounded deployment
lock and statement timeouts.

Lead-submission review uses an immutable creation-key cursor over a live review
queue. Its default is 50 and maximum is 100; malformed cursors and limits fail
before service work, direct callers repeat the bound, and exact counts remain
scoped to the selected tenant/form. Dedicated freshly migrated PostgreSQL
acceptance seeds 1,001 equal-time submissions across forms and review states,
traverses all 350 matches without overlap, requires the first and adjacent
100-row pages individually below two seconds, asserts the combined
tenant/form/status/time/ID index, rejects direct overflow, and proves foreign
tenant exclusion. A later arrival remains above an older traversal and appears
on refresh; a review mutation intentionally reconciles through the same refresh
because status filters describe a live work queue. Migration 111 also adds the
unfiltered tenant/time/ID access path with bounded deployment lock and statement
timeouts.

Personal saved-view management uses the compatible offset contract: 50 rows by
default, 100 maximum, a 50,000-row maximum offset, and an exact repeatable-read
total for one tenant/user/entity. Dedicated freshly migrated PostgreSQL
acceptance seeds 1,001 views, budgets each adjacent 50-row page below two
seconds, repeats page two, rejects direct overflow, proves tenant and user
separation, and asserts the `(organization_id,user_id,entity_type,is_default,
lower(name),id)` management index. The same suite serializes a 100-stored final
slot and proves exact-revision/default transitions; a rolling migration test
proves historical and old-binary inserts receive revision 1. The browser loads
all 51 seeded definitions, exercises create/default/update/delete, and repeats
WCAG A/AA scanning.

The failure path also holds the only connection in a one-connection pool, proves a
waiting request observes its 200 ms deadline, releases capacity, and proves the
pool serves requests again. It separately holds an access-exclusive table lock,
proves a real service read honors the same 200 ms deadline, releases the lock,
and proves reads recover. The gate then exports the 10,000-row synchronous
contact ceiling, verifies valid tenant-isolated CSV, and budgets it at 5 seconds.
It inserts row 10,001 afterward and requires an explicit `ErrTooManyRows`
failure, preventing a partial file from masquerading as a complete export. The
same fixture creates a saved contact report, budgets its first 100-row page at
two seconds and its complete 10,000-row CSV at five seconds, verifies the parsed
header and row count, and proves a foreign workspace cannot execute or export
it. After row 10,001, saved-report export must also fail explicitly without
creating a second `report.export_downloaded` audit event.
The same 500-company tenant is promoted to active clients and receives one
qualifying note per client. The client-period gate requires exact 500-client/
500-touch totals, a bounded 100-row page under two seconds, and an empty report
when the same query is executed for the separate tenant. This catches an
unbounded linked-source aggregation or missing organization predicate before
the report can be promoted.
The same run also retains one creation event for each of 500 target deals and
one later forward transition per deal. The fixed pipeline cohort gate requires
exact cohort/current/reach/exit/forward totals and a 15.0-day median completed
entry-stage visit under two seconds, then requires the same pipeline and stage
IDs to fail closed for another workspace. This exercises the complete windowed
cohort and velocity query rather than treating a small unit fixture as scale
evidence.
It also validates, durably queues, and executes the complete 1,000-row import
ceiling through the production `import.execute` worker, including source/batch/job
atomicity, duplicate checks, activity/outcome ledgers, 50-row resume checkpoints,
exact tenant totals, foreign-tenant absence, and immediate successful-source
cleanup, within 10 seconds.
Current local evidence was approximately 17.4 ms for the 500-deal cohort report,
10.4 ms for the 500-client activity page, 1.1 ms for two 100-row pages from the
1,001-row record timeline, 35.3 ms for the 806,196-byte core export, 7.6 ms for
the 100-row saved-report page, 23.9 ms for the 453,869-byte saved-report export,
3.9/6.6 ms for the 10,000-row grouped-bar page/export, and 810 ms for the
52,937-byte/1,000-row import (545 ms on the 2026-07-22 local rerun). Test
failure output includes observed latency or the query plan/budget that regressed.

`apps/api/internal/modules/dashboard/forecast_postgres_test.go` separately gates
the complete fixed operational dashboard rather than treating its individual
panels as independent small queries. It migrates an isolated schema, seeds equal
local and foreign working sets of 10,000 deals, contacts, and tasks plus 20,000
activities per tenant, and requires the full deal/task/contact/activity,
client-review, owner, stage, quota, and FX response to reconcile exactly below
two seconds. The service itself allows at most five seconds and executes every
panel from one read-only repeatable-read snapshot. Migration 118's recent-activity
and active-recent-contact plans are asserted after `ANALYZE`; existing task,
review, membership, quota, exchange-rate, and tenant-deal indexes retain their
reviewed roles. A forced table lock must become stable `DASHBOARD_TIMEOUT`, and a
separately blocked quota update must roll back instead of returning an ambiguous
partial success. This remains a regression budget on CI hardware, not a host SLO.

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
recovery, and context deadlines. Gmail and Microsoft outbound adapters have
exact credential-free HTTP contract tests for authorization, MIME encoding,
bounded error/success responses, required response semantics, and deadlines;
concurrent real-PostgreSQL acceptance proves one serialized refresh before two
sends. No provider request is automatically retried after it starts. Durable
sequence acceptance separately proves that an ambiguous SMTP or provider-API
result is quarantined without an automatic duplicate send and requires an
audited operator decision. Remaining load/failure work before pilot validation
is a production-like host run and approved live-provider runs with retained
evidence plus the feature loads introduced by later convergence slices.

## Frontend bundle gate

`npm run build:checked` builds the production SPA and measures raw plus
level-9-gzip bytes using only Node's standard library.

| Asset boundary | Raw maximum | Gzip maximum |
| --- | ---: | ---: |
| Initial JavaScript entry | 190 KiB | 65 KiB |
| Any lazy JavaScript chunk | 60 KiB | 16 KiB |
| All JavaScript and CSS | 782 KiB | 244 KiB |
| All CSS | 20 KiB | 5 KiB |

Current production-URL evidence: 178.87 KiB/57.99 KiB entry, 55.12 KiB/15.74 KiB largest lazy
chunk, and 781.77 KiB/243.34 KiB total assets. The production contact, company,
deal, and task routes are 27.27/8.55, 45.09/13.08, 55.12/15.73, and 26.09/7.92
KiB raw/gzip respectively. Hosted billing, invoice/payment visibility, explicit self-hosted mode,
portable workspace export, and measured usage remain isolated in a 14.58 KiB/4.62 KiB settings route. Its
OAuth-mailbox peer remains separately lazy loaded at 10.63 KiB/3.21 KiB;
bounded template/snippet management is 11.02 KiB/2.92 KiB; and bounded
sequence definition/history management remains in a 12.91 KiB/3.98 KiB route. The
10.44 KiB/3.51 KiB background-operations route includes labeled replay, exact-filter
durable CRM export request/progress/download recovery, and an explicit lead-follow-up filter, while a
0.15 KiB shared helper keeps retry-key generation consistent across billing,
signup, import, merge, and bulk recovery paths. Production builds now omit the
incomplete calling, SMS, calendar/booking-link, audience, lead-scoring,
marketing-email, and nurture-campaign management surfaces; the bundle gate
rejects their accidental inclusion. The saved-report route ships table and
grouped numeric bar outcomes, but production filters its custom line/funnel/pie/KPI
controls and definitions from navigation. This
preserves normal navigation for production-capable outcomes while keeping
unfinished foundations available in development.
Future frontend slices must remain within the ratcheted ceilings. The complete custom-field outcome
adds an isolated 6.66 KiB/2.27 KiB settings route plus shared typed forms,
filtering, import/export, and duplicate-review code. Archive recovery adds a
separate 5.51 KiB/2.20 KiB settings route instead of growing the near-budget
core record screens. Live data-quality, snapshot-backed sales activity, exact
pipeline cohort conversion/velocity, traceable stale follow-up queues, exact client-period activity, and the
saved-table/grouped-bar builder and results leave the Reports route at
47.40 KiB/11.16 KiB;
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
remain unchanged. The immutable quote-signature ceremony then adds customer
consent/status/certificate UI, staff evidence and recovery controls, and retry-safe
public mutations. After consolidating those public actions and preserving a
44-pixel disclosure target, its measured 651.83/208.45 KiB build raises only the aggregate ceilings to 652/209 KiB; the
6.35/2.28 KiB public quote route, entry, per-route, and CSS limits remain bounded.
Deliberate signed-quote conversion then reuses the close-review and deal-snapshot
controls while adding retained conversion evidence, retry-safe mutation state,
and accessible won-stage handoff UI. The same browser review corrected grouped
activity-list semantics and badge contrast. Its measured 655.26/209.21 KiB build raises
only the aggregate ceilings to 656/210 KiB; the 179.11/58.06 KiB entry,
48.31/13.88 KiB largest route, and CSS ceilings remain unchanged.
The populated-browser WCAG follow-up then gives shared record-selection
checkboxes a 24-pixel target and explicit spacing across Contacts, Clients,
Deals, and Tasks. The clean rerun also keeps record-work mutation forms hidden
until their authoritative snapshot loads, so a late response cannot silently
erase user input. Its measured 655.67/209.25 KiB build remains within the same
656/210 KiB aggregate ceilings.
Active quote expiration and immutable reissue then consolidate retry-safe deal
snapshot mutations and keep the complete replacement controls inside the
existing Deals route. Its measured build remains 655.97/209.43 KiB, with a
48.74/14.01 KiB largest route, under the unchanged ceilings.
Revisioned quote preparation and independent exact-PDF approval then add a
9.49/3.11 KiB settings route and bounded deal controls while removing the
incomplete audience and lead-scoring routes from production. The measured
178.92/58.02 KiB entry and 54.78/15.63 KiB largest route remain below their
unchanged limits. The complete user outcome measures 658.59/210.07 KiB in
aggregate, so only the aggregate ceilings advance to 659/211 KiB.
Durable conditional lead-form follow-up then extends the already-lazy task
automation route to 14.71/4.67 KiB without changing the entry or largest route.
The complete production build measures 663.23/211.14 KiB in aggregate, so only
the aggregate ceilings advance to 664/212 KiB; all entry, per-route, and CSS
ceilings remain unchanged.
Whole-day scheduling for that exact outcome, separate from the task due offset,
then keeps the route at 15.07/4.73 KiB and the complete build at
663.93/211.31 KiB. The entry remains 178.93/58.02 KiB and the largest lazy chunk
54.78/15.63 KiB, so every existing ceiling remains unchanged.
Reversible lead-submission spam review and recovery then extends the already
lazy lead-forms settings route to 14.47/4.63 KiB. The complete production build
measures 669.62/212.99 KiB while the entry and largest route remain unchanged,
so only the aggregate ceilings advance to 670/214 KiB.
Promoting the bounded saved-table report outcome then adds production definition
management, latest-request cancellation, accessible paged results, and the
client contract for the tenant-safe backend executor and audited CSV export.
Its measured 31.49/8.12 KiB route keeps the entry and largest lazy route under
their existing ceilings;
the complete build measures 687.01/217.51 KiB, so only the reviewed aggregate
ceilings advance to 690/220 KiB. Its route orchestration is now 298 lines and
the separately tested pure catalog/form model is 209 lines, preserving the
unchanged 500-line ceiling without changing bundle bytes.
The event-time deal-condition slice keeps those ceilings unchanged: compact
typed catalogs and shared validation leave the complete build at
689.50/218.37 KiB, while the task-automation route remains 436 lines.
The grouped bar report outcome reuses the same tenant-safe typed query and
audited CSV runtime, adds a production builder contract plus a visual renderer
paired with the exact accessible data table, and validates 10,000-row grouping
under the existing report latency budgets. The complete build measures
692.40/219.14 KiB and the Reports route 33.91/8.81 KiB, so only the aggregate
raw ceiling advances from 690 to 693 KiB; entry, async, gzip, and CSS ceilings
remain unchanged. Its 293-line orchestration, 245-line pure model, and 33/26-line
bar/table renderers remain below the default source ceiling.
The customer-only period activity outcome adds one focused 162-line component
that reuses viewer-aware touchpoint semantics, gives current clients an exact
bounded date/owner/activity rollup with source links, and adds a 500-client
tenant-isolated PostgreSQL page budget under two seconds. The complete build is
698.98/219.95 KiB and the Reports route is 39.90/9.55 KiB, so only the aggregate
raw ceiling advances from 693 to 699 KiB; entry, async, gzip, and CSS ceilings
remain unchanged. Saved-report orchestration remains 295 lines. The 2026-07-21
local PostgreSQL 16.14 run completed the 500-client/100-row activity page in
approximately 22.4 ms; that observation is diagnostic, while the checked
two-second ceiling is authoritative.
The exact pipeline cohort outcome adds a separate 193-line component and reuses
the existing deal-stage ledger instead of activating stored custom-funnel
metadata. It adds current-outcome, exact reach/exit, and elapsed-day medians with
a five-second query deadline plus the 500-deal tenant gate above. The complete
build is 706.51/221.54 KiB and the Reports route is 47.40/11.16 KiB, so the
reviewed aggregate ceilings advance from 699/220 to 707/222 KiB while entry,
per-async-chunk, and CSS ceilings remain unchanged. The 2026-07-21 local
PostgreSQL 16.14 observation completed the 500-deal/two-stage cohort in about
14.8 ms; the checked two-second ceiling remains authoritative.
The reviewed deal-playbook outcome then adds accessible authoring for 1–5
literal tasks, a separately tested executable-contract/form model, and exact
browser task/export evidence without enlarging the entry or largest route. The
complete build is 709.29/222.36 KiB, so the reviewed aggregate ceilings advance
from 707/222 to 710/223 KiB. The task-automation chunk is 20.33/6.28 KiB; its
283-line route and 230-line pure model remain separately below the source
ceiling. Entry, per-async-chunk, CSS, and all source ceilings remain unchanged.
The durable record-email outcome then reuses one shared recovery composer across
contact, company, and deal routes and removes the superseded contact-only send
state. The measured production build is 178.83/57.98 KiB entry,
54.69/15.60 KiB largest lazy chunk, and 709.60/222.25 KiB aggregate raw/gzip.
The contact/company/deal route chunks are 26.83/8.33, 34.62/10.18, and
54.69/15.60 KiB, so all existing byte ceilings remain unchanged.
The exact template-preview/test-to-self outcome then adds dynamic custom merge
fields, server rendering, private durable test recovery, and a real-SMTP/WCAG
browser path. Consolidating contact/company/deal send clients and reusing
existing presentation styles limits the production build to 178.83/57.97 KiB
entry, 54.67/15.59 KiB largest lazy chunk, and 711.94/222.81 KiB aggregate
raw/gzip. The measured outcome advances only the aggregate raw ceiling from
710 to 713 KiB; entry, per-chunk, aggregate gzip, CSS, and source ceilings stay
unchanged.

Bounded record-history continuation then adds one shared activity-page client,
visit-safe append/deduplication state for contact/company/deal work, real task
detail hydration, and accessible older-history controls. The measured build is
178.82/57.96 KiB entry, 54.93/15.65 KiB largest lazy chunk, and
716.13/223.87 KiB aggregate raw/gzip. Entry, per-route, and CSS ceilings remain
unchanged; only the reviewed aggregate ceilings advance from 713/223 to
717/225 KiB for this complete user-visible outcome.

Bounded company-linked-person management then replaces the first-page-only
relationship editor with searchable continuation, safe linking, primary repair,
and unlink recovery. The measured build remains 178.82/57.97 KiB for the entry
and 54.93/15.64 KiB for the largest lazy chunk; the company route is
44.95/12.98 KiB and aggregate assets are 726.08/226.44 KiB raw/gzip. Entry,
per-route, and CSS ceilings remain unchanged; only the reviewed aggregate
ceilings advance from 717/225 to 727/227 KiB for this complete outcome.

Snapshot-bound shared-inbox continuation then adds a strict 50/default,
100/maximum API and accessible row-51 loading without raising those ceilings.
The measured build is 178.82/57.98 KiB for the entry, 54.93/15.65 KiB for the
largest lazy chunk, 15.19/4.42 KiB for the combined mailbox/team-inbox chunk,
and 726.99/226.82 KiB aggregate raw/gzip. Refresh, continuation, and coordination
actions are serialized in the focused route, while the backend cursor owns the
mutable queue snapshot and total order.

Bounded lead-submission review continuation then replaces the newest-50-only
operator queue with a strict 50/default, 100/maximum API, an immutable
creation-time/ID cursor, exact form-scoped status counts, ID-deduplicated
accessible loading, and refresh after a review mutation. Migration 111 supplies
the unfiltered and combined form/status cursor indexes. Fresh PostgreSQL 16.14
acceptance traverses 350 filtered rows within a 1,001-row equal-time tenant,
asserts the combined index, budgets the first and adjacent 100-row pages below
two seconds, excludes a later arrival and foreign tenant, and proves refresh
exposes the arrival. Chromium loads seeded row 51 before quarantine/recovery and
reruns the populated WCAG scan. The measured build is 178.82/57.96 KiB entry,
54.93/15.65 KiB largest lazy chunk, 15.46/5.01 KiB lead-forms route, and
728.06/227.23 KiB aggregate raw/gzip. Only the aggregate ceilings advance from
727/227 to 729/228 KiB for this complete outcome; entry, per-route, CSS, and
source ceilings remain unchanged.

Bounded product-catalog management then replaces the complete active/inactive
payload on every settings and deal load with exact searchable/status-filtered
50/default and 100/maximum management pages plus a separate active-only quote
request that continues bounded pages only for legacy workspaces already above
the new ceiling. A namespaced transaction lock makes the 100-active-item ceiling safe
across API instances and revalidates the writer before every mutation. Fresh
PostgreSQL 16.14 acceptance seeds 1,001 tenant rows, asserts the existing
tenant/status/name index, budgets the complete 100-row active set and adjacent
50-row management page below two seconds, proves literal wildcard search,
tenant/role denial, a one-success/one-limit activation race, and archive then
reactivation recovery. Chromium loads and searches row 51, creates an active
service, excludes inactive history from quote selection, and carries the
service through finalization with a populated WCAG scan. The measured build is
178.82/57.96 KiB entry, 54.93/15.64 KiB largest lazy chunk, 7.72/2.55 KiB
product-catalog route, and 730.88/228.33 KiB aggregate raw/gzip. Preserving
legacy overflow access advances the reviewed aggregate ceilings from 729/228 to
731/229 KiB; entry, per-route, CSS, and source ceilings remain unchanged.

Truthful workflow activation then adds exact active-action allocation disclosure,
latest-load and serialized-writer guards, plus non-destructive recovery for
unsupported legacy active definitions. The measured production build remains
178.82/57.97 KiB entry and 54.93/15.64 KiB largest lazy chunk, while aggregate
assets are 733.07/228.75 KiB raw/gzip. Only the reviewed aggregate raw ceiling
advances from 731 to 734 KiB; aggregate gzip, entry, per-route, CSS, and source
ceilings remain unchanged. The task-automation chunk is 22.52/6.88 KiB, with a
327-line route and separately tested 239-line contract/form model.

Bounded workflow-definition management then replaces the complete retained
definition payload with 50/default and 100/maximum pages, a 50,000 maximum
offset, exact stored-definition and active-action totals from one repeatable-read
snapshot, stable active/position/update/ID order, visible ID-deduplicated
continuation, and a fresh first page after mutations. Migration 112 adds the
matching tenant management index. Fresh PostgreSQL 16.14 acceptance seeds 1,001
local definitions plus a foreign row, asserts that index, budgets adjacent
100-row pages below two seconds, and proves repeat stability, exact summary,
direct-service bounds, and tenant exclusion. Chromium starts with 52 stored
definitions, proves row 51 is absent from the first 50 and visible after “Load
more,” then executes the reviewed deal rule. The measured build is
178.82/57.97 KiB entry, 54.93/15.65 KiB largest lazy chunk, 25.04/7.63 KiB
task-automation chunk, and 735.00/229.57 KiB aggregate raw/gzip. The route is
377 lines and retains the separately tested 239-line contract/form model. Only
the reviewed aggregate ceilings advance from 734/229 to 735/230 KiB; entry,
per-route, CSS, and source ceilings remain unchanged.

Bounded saved-report-definition management applies the same 50/default,
100/maximum, and 50,000-offset contract to the production Reports catalog, with
an exact tenant total from the same repeatable-read snapshot as the stable
active/update/ID page. Migration 113 supplies the matching management index.
Fresh PostgreSQL 16.14 acceptance seeds 1,001 local definitions plus a foreign
row, asserts that index, budgets adjacent 100-row pages below two seconds, and
proves repeat stability, exact totals, direct-service bounds, and tenant
exclusion. Chromium proves the first-50/row-51 continuation before creating,
running, and exporting both promoted report shapes. The measured production
build is 178.82/57.97 KiB entry, 54.93/15.65 KiB largest lazy chunk,
49.33/11.72 KiB Reports chunk, and 736.93/229.89 KiB aggregate raw/gzip. The
Reports orchestration is 352 lines and retains its separate 245-line tested
catalog/form model. Only the reviewed aggregate ceilings advance from 735/230
to 737/231 KiB; entry, per-route, CSS, and source ceilings remain unchanged.

Truthful durable lead-run recovery joins the user-facing run to its exact queue
row, displays attempt/dead-letter evidence, reconciles active/failed health, and
guides admins into the existing audited Operations replay. Its measured build
is 178.82/57.97 KiB entry, 54.93/15.64 KiB largest lazy chunk,
25.51/7.75 KiB task-automation chunk, 7.72/2.72 KiB Operations chunk, and
737.57/230.21 KiB aggregate raw/gzip. Only the aggregate raw ceiling advances
from 737 to 738 KiB; entry, per-route, aggregate gzip, CSS, and source ceilings
remain unchanged.

Bounded sequence-enrollment history replaces an aggregate-only management view
with an on-demand 50/default and 100/maximum immutable creation/ID cursor,
per-enrollment delivery aggregation, explicit uncertain-delivery recovery, and
stale/continuation guards. Migration 114 adds the exact tenant/sequence cursor
index. Fresh PostgreSQL 16.14 acceptance seeds 1,001 equal-time local rows plus
a foreign sequence, asserts the index, traverses repeat/adjacent/final pages,
rejects a direct oversized limit, and reconciles detailed delivery outcomes.
The SMTP Chromium journey expands the completed enrollment before its populated
WCAG scan. The measured production build is 178.82/57.96 KiB entry,
54.93/15.63 KiB largest lazy chunk, 10.00/3.16 KiB Email Sequences route, and
742.74/231.58 KiB aggregate raw/gzip. The drill-down is isolated in a 160-line
route-local component. Only the reviewed aggregate ceilings advance from
738/231 to 743/232 KiB; entry, per-route, CSS, and source ceilings remain
unchanged.

Immutable workflow action outcomes replace aggregate task counts with an
expandable, ordered action ledger for the two supported execution contracts.
The UI shows the captured label, lifecycle state, attempt count, schedule,
terminal reason, and a same-workspace created-task link without expanding the
general workflow surface. The measured production build is 178.82/57.97 KiB
entry, 54.93/15.64 KiB largest lazy chunk, 27.45/8.34 KiB task-automation
chunk, and 744.94/232.35 KiB aggregate raw/gzip. The run display is isolated
in a 61-line component. Only the reviewed aggregate ceilings advance from
743/232 to 745/233 KiB; entry, per-route, CSS, and source ceilings remain
unchanged.

Bounded quote-template management applies the same reviewed catalog shape to
retained preparation definitions: exact searchable/status-filtered 50/default
and 100/maximum management pages, a 50,000 offset ceiling, and a separate
active-only deal loader that rejects changing totals while retaining legacy
overflow. A tenant-namespaced transaction lock revalidates the owner/admin and
serializes the 100-active-template ceiling. Fresh PostgreSQL 16.14 acceptance
seeds 1,001 local rows plus a foreign sentinel, proves indexed active selection,
budgets active and adjacent management pages below two seconds, and covers
literal wildcard search, tenant/role denial, a one-success/one-limit final-slot
race, and archive/reactivation recovery. Chromium loads/searches row 51,
creates the active template, and carries it through approval, signature, and a
populated WCAG scan. The measured build is 178.82/57.96 KiB entry,
54.93/15.64 KiB largest lazy chunk, 11.73/3.76 KiB quote-template route, and
747.93/233.39 KiB aggregate raw/gzip. Only the reviewed aggregate ceilings
advance from 745/233 to 748/234 KiB; entry, per-route, CSS, and source ceilings
remain unchanged.

Bounded email-sequence definition management applies exact repeatable-read
searchable/status-filtered 50/default and 100/maximum pages with a 50,000
offset ceiling. Step and outcome joins are restricted to the selected page.
Every writer is revalidated in the transaction; update, delete, and approval
bind to the reviewed revision; lifecycle audit commits atomically; and a tenant
advisory lock serializes the 100-active final slot. Active-only selectors load
all bounded pages, reject changing totals, and preserve legacy overflow. Fresh
PostgreSQL 16.14 acceptance seeds 1,001 local definitions plus a foreign
sentinel, asserts the tenant/status/name plan, budgets the active and adjacent
management pages below two seconds each, and covers literal wildcard search,
tenant/role denial, stale revisions, idempotent audit, a one-success/one-limit
approval race, and pause/reapproval recovery. Chromium loads/searches row 51,
creates and approves the pilot cadence, excludes drafts from enrollment, then
completes the SMTP delivery and populated WCAG scan. The measured build is
178.82/57.97 KiB entry, 54.93/15.64 KiB largest lazy chunk,
12.91/3.98 KiB Email Sequences route, and 751.69/234.60 KiB aggregate raw/gzip.
Only the reviewed aggregate ceilings advance from 748/234 to 752/235 KiB;
entry, per-route, CSS, and source ceilings remain unchanged.

Bounded email-template and snippet management gives each catalog an exact
repeatable-read filtered total, literal 100-character name search, 50/default
and 100/maximum page size, and 50,000 offset ceiling. Stable case-insensitive
name/ID order uses tenant indexes. Fresh PostgreSQL 16 acceptance seeds 1,001
local rows in each catalog plus foreign sentinels, requires adjacent pages
below two seconds, and proves direct bounds, literal wildcard search, tenant
and actor denial, exact-revision update/delete, content-free transactional
audit, one-success/one-limit concurrent final-slot creates, and capacity
recovery after delete. Settings exposes independent guarded continuation while
record composers traverse every stable bounded page and preserve legacy
overflow. The measured production build is 178.82/57.97 KiB entry,
54.93/15.65 KiB largest lazy chunk, 11.02/2.92 KiB Email Templates route,
and, at that slice, 757.71/236.28 KiB aggregate raw/gzip. Only the reviewed aggregate ceilings
advanced from 752/235 to 758/237 KiB; entry, per-route, CSS, and source ceilings
remain unchanged.

Bounded saved-view management adds the 233-line shared component and focused
library/component tests without changing the entry, per-route, CSS, or source
ceilings. The complete build now measures 760.39/237.22 KiB aggregate raw/gzip;
the aggregate ratchet alone advances from 758/237 to 761/238 KiB.

Durable filtered CRM exports add exact-filter handoff from all four core lists,
an admin request/progress/failure/download path, and the shared API helper. The
new UI was reduced after its first measurement by removing redundant filter
editing while preserving the originating list criteria. At that slice the measured build was
178.87/57.99 KiB entry, 55.12/15.73 KiB largest lazy chunk, and 765.81/239.24
KiB aggregate raw/gzip. Only the reviewed aggregate ceilings advance from
761/238 to 766/240 KiB; entry, per-route, CSS, and source ceilings stay fixed.

Task-directory hardening isolates list/bootstrap/filter orchestration in a tested
275-line hook and reduces `tasks.jsx` from 500 to 298 lines. Request identity,
not abort cooperation alone, prevents a late bootstrap or superseded filter from
replacing the newest list; browser Back/Forward filter changes reload state
without writing a second history entry. Independent option application retains
successful deal/company/contact/user choices when a peer request fails. A shared
task language model and date sorter preserve exact visible behavior while keeping
the route at 26.09/7.92 KiB. The complete build measures 178.87/57.99 KiB entry,
55.12/15.73 KiB largest lazy chunk, and 765.77/239.71 KiB aggregate raw/gzip,
without changing any byte or source ceiling.

The approval-gated deal-task slice adds a bounded approver queue, explicit
decision controls, and retained action evidence to the existing automation
route. The route remains below the unchanged per-route ceiling at 34.99/10.05
KiB raw/gzip, and the complete build measures 178.87/57.98 KiB entry,
55.12/15.73 KiB largest lazy chunk, and 773.96/241.47 KiB aggregate raw/gzip.
Only the aggregate ceilings advance from 766/240 to 774/242 KiB; entry,
per-route, CSS, and source ceilings remain unchanged.

The causal-guard and teammate-notification slice adds one exact authorable
non-recursive action, finite recipient/cap disclosure, delivery-count evidence,
and root/nested/loop-guard run inspection. The route remains below the unchanged
per-route ceiling at 40.13/11.17 KiB and 433 production lines. The complete
build measures 178.87/57.99 KiB entry, 55.12/15.73 KiB largest lazy chunk, and
778.16/242.36 KiB aggregate raw/gzip. Only the aggregate ceilings advance from
774/242 to 779/243 KiB; entry, per-route, CSS, and source ceilings remain
unchanged.

The deal-owner assignment slice adds one exact trigger-capable action, active-
teammate authoring, typed changed/no-op result evidence, and nested causal-limit
inspection. The automation route remains below the unchanged per-route ceiling
at 42.80/11.65 KiB and 453 production lines; every workflow Go production file
also remains below 500 lines. The complete build measures 178.87/57.99 KiB
entry, 55.12/15.74 KiB largest lazy chunk, and 781.77/243.34 KiB aggregate
raw/gzip. Only aggregate ceilings advance from 779/243 to 782/244 KiB; entry,
per-route, CSS, and source ceilings remain unchanged.

The workflow-run API still defaults to 20 and caps at 100 runs; the normal UI
requests 25. One repeatable-read transaction selects that bounded run page and
then its ordered outcomes with a tenant/run-array query. Reviewed execution
emits at most five actions per run, so the normal screen returns at most 125
outcomes; the stored-definition validation ceiling of 25 keeps even legacy
history bounded at 2,500 outcomes for the maximum direct request. A larger
action history needs an explicit cursor contract and query-plan evidence rather
than a higher hidden limit.

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
production outreach and lead-score orchestrators, a focused contact create/detail workspace and detail orchestrator, plus focused company-directory, linked-
people, create/detail workspace presentation, directory/detail orchestration, and shared record selection/work leave the parent routes at 453 contact lines,
463 company lines, 474 deal lines, and
298 task lines, down from 2,038, 1,364, 1,365, and 1,093 respectively, without
changing their lazy-load boundaries. Tested 68-line selection and 207-line work
hooks now serve contacts, companies, and deals, abort obsolete loads, distinguish repeated
A-to-B-to-A visits, serialize per-record mutations, validate record/work
identities, and keep late saves and work off the active contact. The 123-line
contact outreach hook clears record-scoped sequence state on selection changes
and rejects late responses from prior selection epochs; the shared 303-line
record-email composer owns direct-send state and recovery. A 59-line lead-score hook rejects
duplicate evaluations, wrong-contact responses, and late results across
leave-and-return navigation. Deal directory, shared form, and editor
presentation live in focused 157-, 74-, and 87-line modules, while a 107-line
detail workspace composes editor, commercial, stage, email, and work cards. The shared selection
and work hooks plus a 164-line detail orchestrator apply the same visit identity,
route synchronization, serialization, work, and response validation contract to deals; the guarded
187-line commercial hook rejects stale or mismatched quote/proposal results and
exposes one shared pending state so line-item and proposal controls cannot race
another deal snapshot mutation.
A tested 231-line deal-directory hook owns bootstrap/options, filters, loading,
URL/history synchronization, and request identity, preventing late list replacement
without repeating the full bootstrap after each filter change.
The 178-line company-detail hook applies the same contract to direct routes,
directory selection, related-deal and work loading, and locally seeded creates.
A tested 171-line company-directory hook owns bootstrap data, filters, loading,
and request identity, so a late initial load or older search cannot overwrite the
latest directory even when a client ignores abort signals.
A 168-line contact create/detail workspace composes scoring, outreach, customer
context, review scheduling, touchpoints, and record work; a 134-line contact-detail
hook owns direct-route and directory selection, record/related-deal/work loading,
and visit-scoped form state. The contact parent route is therefore back under the
default ceiling without weakening the existing stale-response guards.
A 173-line company create/detail workspace similarly composes the client editor,
linked people, email, account/review context, touchpoints, and shared work cards
without changing the route boundary.
The shared record-work follower control also resets on record changes and
rejects late responses from an earlier contact, client, or deal.
A tested 88-line task quick-action hook plus 275-line directory-state hook,
209-line directory, 70-line create/detail workspace, 162-line view model, and
157-line detail-state modules separate task presentation and orchestration. These guards prevent
concurrent mutations of the same task, validate response identity, and update the active detail only when
it still represents that task; delayed completion cannot pull navigation back
to an earlier record. List request identity rejects late bootstrap/filter results,
and browser-history query changes synchronize state without duplicate navigation.
Full-form task saves use the same task-visit identity, suppress duplicate submission,
and cannot navigate after the task route unmounts. The task parent route is now 298 lines.
Narrowing the normal automation UI to its
executable task-rule subset also reduced that route from 669 to 261 lines.
Every production route file now uses the default source ceiling; future splits
must preserve that no-exception baseline.

The API composition root is 426 lines, down from 996. Its audited 265-route
surface is registered through 184-line platform, 297-line foundation, and
378-line core-CRM files. The security inventory and hosted-write-policy tests
scan all production files in the package, so splitting registrations cannot
silently remove a route from either guard. Shared handler helpers are isolated
in a 297-line file, invitation delivery is isolated in a 123-line handler, and
record history owns a focused 106-line handler, and `support_handlers.go` is 388
lines. The service-contract catalog is 438 lines and the unchanged explicit
runtime dependency container is isolated at 70 lines, replacing the former
500-line mixed file. The former 491-line authentication/user handler is split
into 90-line session-cookie authentication, 125-line public onboarding, and
296-line tenant user-lifecycle handlers, so these distinct security boundaries
can be reviewed without unrelated behavior. Authenticated email-message/inbox
operations, response projection/access decisions, and public tracking are
separate 224-, 221-, and 56-line files rather than one 487-line mixed handler;
the public file applies its privacy headers before every redirect decision.
The workflow module's former 1,352-line mixed service is now a 151-line public
model/service shell plus 350-line definition persistence, 338-line run
persistence, 363-line definition-contract validation, and 180-line condition
evaluation boundaries. A package-local ratchet covers every production file in
that module, including its existing activation, task execution, retained-action,
and recovery seams. Every production file in `internal/app` and
`internal/modules/workflowautomations` now has headroom under the default
500-line ceiling.

## Source-size no-growth ratchet

Source size is an imperfect complexity measure, so these limits are a temporary
no-regression ratchet rather than a claim that a file below the limit is well
designed. CI runs `npm run check:source`; backend tests apply the same rule to
the application composition and workflow automation packages.

Every production route file, every production `internal/app` Go file, and every
production `internal/modules/workflowautomations` Go file is limited to 500
lines, with no explicit exception. Tests are excluded because large
fixture-driven flow tests have different review tradeoffs; lint and normal test
execution continue to gate them.
