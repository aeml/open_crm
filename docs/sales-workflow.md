# Sales Workflow

Last reviewed: 2026-07-21

This is the supported pilot sales path. It joins the production-capable pieces
that are otherwise described individually in the capability matrix. The native
quote-signing path is executable production-equivalent behavior but remains a
foundation until policy review, provider evidence, and pilot validation pass;
the hidden general workflow builder is not promoted.

## Pilot path

1. An admin configures a pipeline, open-stage probabilities, and one or more
   bounded deal-to-task rules under **Pipelines** and **Automations**.
2. A teammate creates a deal, links its client and primary contact, selects the
   pipeline stage and owner, and supplies value and expected-close context.
3. A matching rule creates an ordered playbook of 1–5 assigned follow-up tasks
   in the same transaction. Each task has an independent whole-day due offset
   and appears on the deal and in the exact overdue, next-24-hours, later, or
   no-date task bucket. Due-soon and overdue in-app
   reminders are durable, preference-aware, idempotent, and replay-safe.
4. The team records notes, completed tasks, communication touchpoints, line
   items, and stage movement. Forecasts use the configured probability saved on
   each current stage and disclose excluded currency/date cases.
5. Moving a deal into won or lost requires a fixed outcome-specific reason and
   accepts optional notes. After a native signature, a writer can instead
   deliberately select a won stage and close review from the signature row. The
   public signer never closes the deal. That conversion atomically binds the
   retained certificate to the outcome, stage event, task automation, actor,
   and client handoff. Reopening clears the live close context but retains both
   immutable event-time history and signed-quote conversion evidence; replaying
   the original conversion does not re-close the deal. A win also requires a
   company or primary contact. It promotes the explicitly linked organization
   to customer; only a company-less win promotes its primary contact as an
   individual client. Reopening does not erase the customer relationship.
6. The won-deal link opens a compact account summary built from the existing
   client, won deals, open client tasks, recent client notes, and key people;
   the ordinary detail workflows remain the source of truth beneath it.
7. **Clients** triages organizations and individual clients as **Healthy**,
   **Watch**, or **Needs attention** from viewer-visible follow-up and open-task
   timing. Exact reasons, thresholds, client type, health state, and owner
   filters explain every result; no issue state is inferred without an issue
   record.
8. On the customer record, a teammate can schedule one assigned review or
   renewal task, either once or every 1, 3, 6, or 12 months. Dashboard exposes
   the obligation, and completing a recurring task creates exactly the next
   future task without replay duplicates or missed-period bursts.
9. **Reports** reconciles deal creation and movement, won/lost outcomes and
   reasons, notes, task work, teammate ownership/actors, and recent deal events
   over a bounded UTC date range. Deal exports retain the current close context.
10. **Reports > Client activity** then reconciles current organization or
    individual clients over an inclusive UTC period. Current-owner and
    with/without-activity filters, exact note/task/touch/day counts, and the
    latest source link identify post-sale work and put clients with no qualifying
    activity first without fabricating historical health changes.

The clean PostgreSQL-backed Chromium journey performs this sequence as one
workflow, including a pipeline rename after deal creation, forecast continuity,
two-task playbook authoring/WCAG/execution/export evidence, reminder visibility,
real-SMTP delivery, public signing,
matching customer/staff certificate evidence, deliberate signed-quote conversion,
report reconciliation, close review, post-sale health triage, recurring renewal
advancement, exact client-period source/count reconciliation with WCAG, and
cross-tenant collection exclusion.

## Reporting semantics and scale boundary

- Deal metrics use the owner snapshotted on each deal-stage event. Note and task
  metrics use the teammate who performed the activity. Disabled teammates remain
  available for historical filtering; a foreign-tenant teammate is rejected.
- Won revenue sums the immutable deal value converted to workspace base currency
  at each real transition into won. The event retains the value, currencies,
  effective exchange rate/date/source, and converted amount. Missing value or
  event-time FX inputs are counted and excluded rather than inferred from the
  mutable deal or today's rates; a reopened deal won again is another outcome.
- Stage conversion is event-based, not a deal-cohort funnel. Reclosing a reopened
  deal creates another real outcome.
- Stage, won-revenue, and close-reason tracking start times are disclosed
  independently, so pre-ledger history is never presented as complete. All
  panels in one Sales activity response share one repeatable-read snapshot.
- Report ranges are inclusive UTC dates, limited to 366 days, recent event output
  is capped at 50 rows, and the fixed sales-activity calculation remains
  separate from saved reports. Saved contact, company, deal, and task tables
  use typed filters and optional grouping/aggregation; grouped bars require the
  explicit `grouped_bar_v1` execution contract, one category, and a count,
  numeric sum, or numeric average and retain an exact accessible table.
  Historical unversioned bars remain hidden. Fixed sales activity and both saved
  report contracts use a five-second query deadline; saved results use pagination capped
  at 100 rows across at most 100 pages. Owners/admins can download the same query
  as a formula-safe audited CSV; the server refuses row 10,001 instead of
  returning a partial file.
- Client activity is a separate bounded fixed report over current active
  customers. Company rows combine direct work with work on currently linked
  contacts and deduplicate each source event per client; individual rows remain
  contact-scoped. It shares viewer-aware touchpoint privacy, filters on current
  retained owner, prioritizes no-activity rows, returns at most 100 records under
  a five-second deadline, and never presents current derived health as a past
  health snapshot.
- PostgreSQL regression tests seed and analyze mixed multi-tenant event and
  audit history, then require tenant/date and owner/date report paths to use
  their reviewed organization-scoped indexes under normal planning. Activity
  reads exclude unrelated audit actions before aggregation.

## Deliberate boundaries

- A current-data draft PDF remains mutable, while finalized quote versions
  preserve the exact recipient, terms, validity, line-item/totals snapshot, PDF
  bytes, and SHA-256 digest. Connected-mailbox delivery may request the native
  recipient-link ceremony, which retains exact-name consent and a certificate;
  staff cannot forge completion. Revisioned preparation templates snapshot exact
  terms and delivery defaults, and a template or workspace policy can require a
  different active owner/admin to approve the retained PDF digest before any
  delivery. A separate staff decision binds signed evidence to a selected won
  outcome and handoff without granting that authority to the public signer.
  Accounting policy, jurisdiction review, approved mailbox evidence, and pilot
  evidence remain incomplete.
- Only the exposed deal-task rule subset executes. Stored broad workflow
  definitions, branching, approvals, scheduled actions, and other action types
  remain hidden foundations.
- Reminder delivery is in-app. Email reminder delivery is deferred until pilot
  evidence shows the in-app signal is useful and appropriately quiet.
- Review/renewal schedules are task-backed follow-up metadata, not billing,
  contract management, or proof of legal notice.
- Stage deletion, custom close-reason administration, cohort/velocity analytics,
  and immutable forecast snapshots remain deferred pending observed need.

## Pilot feedback record

The technical workflow review is complete, but usage-dependent decisions require
an approved pilot team and observed sessions. For each session, record the team,
date, representative deal, completion/failure point, workaround, severity,
frequency, and anonymized evidence. Review these questions before changing scope:

| Decision | Evidence to collect |
| --- | --- |
| Pipeline vocabulary and limits | Stages operators cannot represent, reordering mistakes, or demand for deletion/migration |
| Probability and forecast policy | Overrides performed outside the CRM, missing close dates, and cadence of forecast review |
| Automation usefulness | Rules disabled or worked around, duplicate/no-op expectations, wording, due-day, and owner-fallback failures |
| Reminder timing and noise | Dismissed or ignored notifications, late follow-up, and requests for email delivery |
| Report interpretation | UTC-day confusion, disputed owner/actor attribution, rate questions, and missing drill-down |
| Close review quality | Repeated use of `other`, unclear fixed reasons, missing context, and reopen/reclose confusion |
| Quote-to-handoff gap | Signature completion failures, external signing tools still used, policy/identity gaps, and information re-entered before close or after a win |
| Client review cadence | Missed reviews, duplicate task expectations, ownership changes, dashboard horizon, and demand for calendar linkage |

Prioritize a change only from repeated evidence or one severe workflow failure;
retain a session note when the current behavior needs no change. Continue the
planned customer-operations convergence rather than adding speculative sales
breadth.
