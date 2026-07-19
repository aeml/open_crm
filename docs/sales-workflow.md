# Sales Workflow

Last reviewed: 2026-07-19

This is the supported pilot sales path. It joins the production-capable pieces
that are otherwise described individually in the capability matrix; it does not
promote the hidden general workflow builder or manual proposal tracking into a
delivery or e-signature claim.

## Pilot path

1. An admin configures a pipeline, open-stage probabilities, and one or more
   bounded deal-to-task rules under **Pipelines** and **Automations**.
2. A teammate creates a deal, links its client and primary contact, selects the
   pipeline stage and owner, and supplies value and expected-close context.
3. A matching rule creates exactly one assigned follow-up task in the same
   transaction. The task appears on the deal and in the exact overdue,
   next-24-hours, later, or no-date task bucket. Due-soon and overdue in-app
   reminders are durable, preference-aware, idempotent, and replay-safe.
4. The team records notes, completed tasks, communication touchpoints, line
   items, and stage movement. Forecasts use the configured probability saved on
   each current stage and disclose excluded currency/date cases.
5. Moving a deal into won or lost requires a fixed outcome-specific reason and
   accepts optional notes. Reopening clears the live close context but retains
   the immutable event-time outcome history.
6. **Reports** reconciles deal creation and movement, won/lost outcomes and
   reasons, notes, task work, teammate ownership/actors, and recent deal events
   over a bounded UTC date range. Deal exports retain the current close context.

The clean PostgreSQL-backed Chromium journey performs this sequence as one
workflow, including a pipeline rename after deal creation, forecast continuity,
automated-task creation, reminder visibility, report reconciliation, close
review, and cross-tenant denial.

## Reporting semantics and scale boundary

- Deal metrics use the owner snapshotted on each deal-stage event. Note and task
  metrics use the teammate who performed the activity. Disabled teammates remain
  available for historical filtering; a foreign-tenant teammate is rejected.
- Stage conversion is event-based, not a deal-cohort funnel. Reclosing a reopened
  deal creates another real outcome.
- Stage and close-reason tracking start times are disclosed independently, so
  pre-ledger history is never presented as complete.
- Report ranges are inclusive UTC dates, limited to 366 days, recent event output
  is capped at 50 rows, and custom report definitions are not executed by this
  surface.
- PostgreSQL regression tests seed and analyze mixed multi-tenant event and
  audit history, then require tenant/date and owner/date report paths to use
  their reviewed organization-scoped indexes under normal planning. Activity
  reads exclude unrelated audit actions before aggregation.

## Deliberate boundaries

- Proposal tracking is a manual CRM status and the PDF reflects current deal
  data. Open CRM does not yet deliver an immutable quote or collect a legal
  signature.
- Only the exposed deal-task rule subset executes. Stored broad workflow
  definitions, branching, approvals, scheduled actions, and other action types
  remain hidden foundations.
- Reminder delivery is in-app. Email reminder delivery is deferred until pilot
  evidence shows the in-app signal is useful and appropriately quiet.
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
| Proposal-to-handoff gap | Manual status errors, external delivery/signing tools used, and information re-entered after a win |

Prioritize a change only from repeated evidence or one severe workflow failure;
retain a session note when the current behavior needs no change. Until that
evidence exists, the next convergence work is the already-planned won-deal
client handoff rather than speculative sales breadth.
