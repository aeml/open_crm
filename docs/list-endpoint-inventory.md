# List Endpoint and Pagination Inventory

Audit date: 2026-07-22

Registered GET route count: `101`

Registered GET route digest: `81624712da868e2566e68f6f5b4aea8f94b63cf8c4175f36949d234521bd478c`

This is the Phase 1 inventory for every production GET route that can return a
collection or a large generated result. The executable guard derives the count
and digest from every production `http.ServeMux` registration, including GET
routes that return one record. Any GET route addition, removal, or rename must
therefore reopen this review instead of silently introducing an unbounded list.

The security and tenant-isolation inventories remain authoritative for
authentication, role, and SQL ownership. This document records result
cardinality, ordering, overflow behavior, and the evidence-based pagination
decision.

## Contracts

| Surface | Result contract | Stable order / continuation | Overflow and failure behavior | Current evidence and decision |
| --- | --- | --- | --- | --- |
| Core contacts, companies, deals, and tasks: `GET /api/contacts`, `/companies`, `/deals`, `/tasks` | Offset page; default `page=1`, `pageSize=20`; maximum page size 100; maximum offset 50,000; exact filtered totals | Contacts: last name, first name, ID; companies: name, ID; deals: pipeline position, stage position, ID; tasks: completion bucket, due time, ID. Every order ends in the immutable ID tie-breaker | Missing values use defaults. Explicit malformed/non-positive values, page sizes above 100, arithmetic overflow, and offsets above 50,000 return `400` before the service/database. Services repeat the same check for non-HTTP callers | Handler tests cover all four rejection paths and the exact 50,000 boundary. The real-PostgreSQL pilot gate checks 100-row adjacent pages, repeat stability, no overlap, exact totals, tenant separation, and direct-service rejection. Offset paging remains appropriate for the approved hosted ceiling of 50,000 contacts/deals and the UI's page-number navigation; introduce keyset paging only if a larger approved workload or measured plan regression requires it. |
| Saved report results: `GET /api/report-definitions/{definitionID}/results` | Offset page; default size 50; page and page size each max 100; at most 10,000 addressable rows | Static typed registry supplies deterministic source ordering with ID tie-breaker; response includes `hasMore` | Invalid page is `400`; query deadline is five seconds; unsupported/historical definitions fail closed | Handler/UI/real-PostgreSQL all-source, overflow, timeout, tenant, and 100-row performance acceptance. |
| Operational reports: sales activity, pipeline funnel, follow-up, client activity, client health, data quality, dashboard, collaboration digest | Complete bounded aggregate or a fixed/request-bounded sample (25, 50, or 100 rows depending on the report) | Every record sample has a documented timestamp/ID or semantic grouping order | Date/range/limit validation is explicit where caller-controlled; report queries have bounded deadlines where their runtime can scale | Handler and real-PostgreSQL report reconciliation/performance tests. These are reports, not record browsers; paging is deferred unless a pilot needs complete drill-down rather than the current focused sample/export. |
| Lead submission review: `GET /api/lead-capture-submissions` | Fixed newest 50 plus exact status counts | Creation time then ID descending | Invalid form/status filters are `400`; review workflow deliberately shows the most recent queue | Browser quarantine/recovery journey and PostgreSQL tenant/lifecycle acceptance. Add cursor continuation before a pilot needs more than the newest review window. |
| Audit and operator job lists: `GET /api/audit-events`, `/api/admin/background-jobs` | Audit max 100/default 50; jobs max 200/default 50 | Creation time then ID descending | Invalid job filters are `400`; service caps protect non-HTTP callers. Complete audit history is available through filtered CSV or the workspace package | Handler, overflow/export, replay, and PostgreSQL acceptance. |
| Email/mailbox lists: `GET /api/email-messages`, `/api/me/email-messages`, `/api/shared-inbox/email-messages`, `/api/record-email-deliveries`, `/api/email-threads/{messageID}` | General/mailbox/shared max 500/default 100; entity timelines, record-delivery recovery, and threads fixed at 100 | Provider/creation time and ID tie-breakers; threads use message chronology | Caller limits outside the service boundary fall back to the safe default; entity/thread views deliberately return only their bounded recovery window | Handler plus SMTP/Gmail/Microsoft contract, uncertainty, recovery, tenant, and browser evidence. Cursor paging remains required before shared inbox promotion. |
| Entity communications: `GET /api/calendar-events`, `/api/calls`, `/api/sms-messages` | Max 200/default 50 per exact tenant entity | Creation/start time then ID descending | Invalid entity scope is rejected; oversized/malformed limits fall back to the service default | Handler and service/provider tests. These fake-provider foundations remain hidden from production navigation where applicable. |
| Data-operation histories: `GET /api/imports`, `/api/data-operations/bulk`, `/api/data-operations/archive`, `/api/data-operations/duplicates`, `/api/workspace-exports` | Imports/archive default 50/max 100; bulk default 20/max 100; duplicate review max 50; workspace export history fixed at 20 | Creation/archive time and ID tie-breakers, or deterministic duplicate score/key order | Services cap every caller. Synchronous data/export outcomes have separate explicit 1,000/import, 10,000-row CSV, and 50 MiB ZIP limits | Handler and real-PostgreSQL rollback/merge/archive/export/tenant/performance acceptance. |
| Definitions and small workspace catalogs: users, pipelines/stages, custom fields, saved views, quote/email templates and snippets, product catalog, lead forms/pages/widgets, audiences/scoring/campaign definitions, workflow definitions, report definitions, sequences/enrollments, booking links, availability | Complete tenant-scoped catalog, currently unpaged | Every query has an explicit semantic order with an ID or unique-position tie-breaker | Active product ceilings or pilot administration patterns keep these small today, but several definition families do not yet enforce a total stored-row ceiling. They must not become event/history stores. Hidden foundation catalogs are not maturity evidence | Handler and tenant tests cover current paths. Add explicit cursor/page contracts or enforced total ceilings before a production-like workload shows material growth, when historical rows become visible, or in the same slice that raises an approved active ceiling. |
| Users' sessions, notifications, record followers, notes, quote approvals, invoices | Fixed security/work queues include notifications 50, quote approvals 100, and invoices 25. Sessions/followers are naturally team-bounded. The standalone notes list and nested record-detail activity arrays are currently complete and unpaged | Security or event time plus ID, or deterministic member order | The bounded queues serve a focused actionable/current set. Notes and nested activities remain the material unbounded record-local gap found by this inventory; full durable history is portable through workspace export but normal UI reads still need a disclosed continuation contract | Handler, lifecycle, role, tenant, and browser acceptance cover semantics. Add bounded cursor continuation plus UI disclosure for notes and record activities before closing roadmap `0.9.2`. |
| Synchronous CSV exports and PDFs/certificates | Core/audit/saved-report CSV refuses row 10,001; binary quote/certificate routes return one bounded immutable artifact | Export registry ordering is stable and formula-safe; artifacts are exact version/request lookups | Overflow is explicit and produces no partial success/audit evidence; database/report deadlines remain bounded | 10,000/10,001 real-PostgreSQL performance and overflow tests, browser download reconciliation, and quote artifact digest/header tests. |
| Singleton/detail/status/public lookup GET routes | One resource or bounded status document; core detail responses also embed the record-local activity arrays called out above | Exact tenant/token/resource lookup | Missing/foreign resources are non-disclosing; public routes have shared abuse budgets where applicable | Covered by `docs/security-surface-inventory.md` and `docs/tenant-isolation-matrix.md`; included in the GET digest so a route cannot be mistaken for a harmless singleton without review. |

## Findings and thresholds

- The material gap found by this audit was the four core list services accepting
  arbitrary positive page sizes and offsets. They now share one overflow-safe
  platform contract at both handler and service boundaries.
- Offset paging is retained deliberately. The maximum supported offset matches
  the largest currently advertised hosted record ceiling, page-number UX needs
  exact totals, and the real-PostgreSQL pilot gate remains far below budget.
- Catalogs and focused work queues are not evidence for large-dataset browsing.
  Their complete-list shape is a known pilot-scale constraint, not a promise of
  unlimited scale.
- Standalone notes and nested contact/company/deal/task activity arrays remain
  complete unpaged reads. The next pagination slice must add bounded cursor
  continuation and a visible older-history path without making workspace export
  the ordinary way to inspect a record.
- New list endpoints must define tenant scope, stable total order with an ID
  tie-breaker, page/limit maximum, malformed-input behavior, overflow behavior,
  timeout, and handler plus PostgreSQL boundary tests before the GET digest is
  updated.

## Maintenance rule

When a GET route or collection query changes:

1. Reclassify its cardinality here and in the security surface inventory.
2. Test malformed, maximum, maximum-plus-one, empty-last-page, stable-order, and
   cross-tenant behavior proportional to the surface.
3. Use keyset pagination only when measured offset behavior, concurrent-write
   semantics, or an approved workload requires it; record the compatibility
   plan for existing page-number clients.
4. Update the expected GET count/digest only after the review and documentation
   are complete.
