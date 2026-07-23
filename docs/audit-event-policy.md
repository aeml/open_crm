# Audit Event Policy

Last reconciled: 2026-07-23

Open CRM's admin audit trail is durable evidence for high-impact workspace,
identity, configuration, recovery, provider, and commercial operations. It is
not a duplicate of every ordinary CRM field change: record timelines,
versioned commercial documents, import/bulk ledgers, and provider ledgers remain
the authoritative detailed history for their own domains.

## Mutation inventory

The audited mutation classes are:

- workspace provisioning, identity verification, invitation, role, member
  lifecycle, password recovery, and active-session revocation;
- organization profile, pipeline, custom-field, quote-template, email-template,
  email-snippet, email-sequence, lead-form, landing-page, and website-widget definition lifecycle and
  approval, plus
  executable automation configuration, transactional workflow-approval
  request, decision, unavailability, and cancellation evidence, and causal
  automation-loop prevention;
- archive recovery, imports and rollback, bulk changes and rollback, duplicate
  merge, client review scheduling, and lead-submission quarantine/recovery;
- billing subscription/reconciliation outcomes, identity- and customer-email
  feedback, accepted one-to-one record email, uncertain email/quote resolution,
  and background-job replay;
- immutable quote finalization, approval, delivery, receipt, signature,
  decline, reissue, conversion, and client handoff;
- workspace, filtered CRM, and audit export request/readiness/download evidence; and
- saved-report definition changes, shared report-dashboard configuration,
  successful saved-report CSV downloads, and scheduled-report configuration,
  queue, generation, per-recipient delivery/recovery, and completion evidence.

Ordinary contact, company, deal, task, note, and preference edits use their
tenant-scoped activity or domain history unless the operation crosses one of
the classes above. Notification acknowledgement and read-only queries are not
administrative audit events. Adding a registered HTTP route changes the global
security-surface digest, while adding an audit producer source changes the
inventory below; both gates require an explicit review instead of silently
expanding the boundary.

Producer source count: `61`

Producer source digest: `6568afbd72a1a34d46e7d39fe36a0f82a50e3144530b90a45a4807f0f94a6f16`

The producer digest covers production Go files that insert `audit_events`
directly or construct the shared audit record input. It is a change detector,
not proof that a schema or passing unit test completes a user outcome; relevant
handler and PostgreSQL acceptance still remain mandatory.

The 2026-07-22 source-boundary review moved the existing invitation, role, and
password-setup producers from the mixed authentication/user handler into the
focused tenant user-lifecycle handler. The mutation classes, metadata, secret
boundary, retention, and export behavior are unchanged; only the reviewed
producer source path changed.

The 2026-07-23 role-transition review replaces the handler's best-effort role
event with a focused transactional producer. It revalidates the acting
owner/admin inside the same serializable tenant transaction, writes the new
role and exactly one append-only event together, and writes neither when audit
insertion fails. The event retains the target email, previous/current finite
role, and current finite membership status; it contains no session, invitation,
credential, provider, or request-correlation material. Exact unchanged retries
create no event. The current membership row and full transition event are both
included in the portable workspace export.

The workflow-approval review adds three focused producers for request or
unavailable-reviewer capture, terminal decision, and definition/member-driven
cancellation. Each event commits with its tenant-scoped approval/run/action
transition. Metadata contains only definition/run/deal/task identifiers,
finite role/decision state, bounded decision or cancellation notes, and task
counts/IDs. It excludes the idempotency key and request fingerprints, which
remain digest-only in the approval ledger and are removed from portable export.

The causal-loop review adds one focused producer for a retained run stopped by
automation re-entry, the maximum causal depth, or the causal-tree fan-out
ceiling. It commits with the skipped
same-tenant run and records only automation/run/deal/action identifiers, finite
depth, and one reviewed reason. It contains no notification body, recipient,
customer value, trigger payload, or idempotency material.

The deal-sequence-enrollment review adds one focused producer for the exact
`deal_add_to_sequence_v1` outcome. A created enrollment or explicit retained
active/paused enrollment commits its `workflow_automation.executed` event with
the deal mutation, run, action result, activity, enrollment, and first durable
job or commits none of them. Metadata is limited to tenant-safe automation,
deal, sequence, enrollment, and contact identifiers, the finite trigger event,
and one creation boolean. It excludes contact and mailbox addresses, sequence
subject/body, provider identifiers, credentials, message content, job payload,
and request/idempotency material. The delivery ledger remains the detailed
provider evidence and the portable workspace export retains the content-free
audit row.

Custom-field create/update/archive events commit in the same bounded-retry
serializable tenant transaction as the exact-revision definition mutation. The
acting owner/admin is revalidated under the membership row lock, and a failed
audit insert rolls back the definition and revision. Metadata contains only
record type, stable field key, immutable data type, and resulting positive
revision. It excludes the mutable label, select options, required/list-display
choices, record values, filters, imports, and every customer identity.

The lead-form mapping review adds transactional create/update producers and a
custom-field-driven mapping-revision producer. The acting owner/admin is
revalidated inside the bounded-retry serializable definition transaction; a
failed audit insert rolls back the definition or custom-field change. Metadata
contains only form identity, current/previous revision, active state, bounded
field and mapping counts, and the stable custom-field key that invalidated a
mapping. It contains no submitted value, consent statement, contact identity, public or
challenge token, request digest, source URL, attribution, or provider data.
The portable audit trail retains these configuration events; detailed form and
submission evidence remains in its own portable datasets.

Landing-page and website-widget create/update events commit in the same
serializable tenant transaction as the definition. Metadata contains only the
current and previous revision, active state, and same-tenant lead-form ID; it
excludes page/widget copy, public IDs and slugs, embed code, consent text,
visitor values, request material, and provider data. A failed audit insert
rolls back the definition, while stale revisions and unauthorized or foreign
actors/forms write neither mutation nor event. These producers remain in the
already-reviewed `definition_audit.go` source, so the 60-file source-set digest
does not change.

Email-sequence create, update, delete, exact-revision approval, and effective
pause events commit in the same tenant transaction as the definition change.
Idempotent repeated approval or pause writes no duplicate event. Metadata is
limited to revision, resulting status, and step count; cadence subject/body and
recipient/provider material remain outside the audit row.

Email-template and snippet create, exact-revision update, and exact-revision
delete events likewise commit with the tenant definition. Their audit metadata
contains only the resulting revision; summaries may name the definition but
never retain its subject, body, merge-field content, recipient material, or
provider data.

Shared report-dashboard changes commit `report_dashboard.updated` in the same
tenant transaction as the exact-revision configuration. Metadata contains only
the resulting revision and bounded widget count. Definition names, source
tables, filters, result values, actor input, and request fingerprints remain
outside the event. A semantic no-op writes neither a new revision nor an audit
row, and a failed audit insert rolls back the complete configuration change.

Scheduled-report configuration commits `report_schedule.created` or
`report_schedule.updated` with the exact tenant/revision/recipient change;
failure to insert the audit row rolls back the schedule. Due discovery commits
`report_schedule.queued` with the occurrence and source-guarded job. Generation,
provider acceptance, ambiguity, exhausted failure, explicit admin resolution,
and terminal run state use the finite `report_schedule.*` events in the same
transaction as their durable state transition. Every actively scheduled
definition edit commits its definition audit and a reason-coded schedule
revision audit together; an inactive or non-executable result also pauses the
schedule. An audit failure rolls back both. Metadata is limited to tenant-
safe definition/schedule/run/user identifiers, bounded report name, cadence and
bounded counts, finite state/recovery choices, artifact byte/row counts, and
SHA-256. Recipient
addresses, CSV bytes and values, provider message identifiers, provider or
internal error text, and request keys remain only in their purpose-specific
operational ledgers and are excluded from the portable audit trail.

Import submission records `import.queued` in the same transaction as the
tenant-scoped batch, retained source, and `import.execute` job. Its metadata is
limited to the reviewed row count and source-retention hours; the filename,
mapping, source digest, CSV bytes, row values, idempotency key, and job payload
remain in their bounded operational ledgers rather than the audit row.

Durable filtered CRM export submission likewise commits `crm.export_queued`
with its request row and queue item. Audit metadata records only the resource
and retention window. Readiness records the resource, row count, immutable
digest, and expiry; download records only the digest. Filters, idempotency keys,
artifact bytes, and exported customer values remain outside the audit row.

## Data and secret boundary

Every row is tenant-scoped and records a bounded event/entity type, summary,
optional actor/entity identity, typed JSON metadata, and database timestamp.
The API supports string, boolean, numeric, and nested metadata emitted by the
reviewed producers. It never assumes all metadata values are strings.

The shared writer removes top-level keys naming passwords, tokens, secrets,
credentials, authorization material, or cookies. A database constraint applies
the same rule to direct transactional producers so a new path cannot bypass the
boundary. Summaries and metadata may contain normal business identifiers such
as record IDs, user email addresses, provider event IDs, immutable digests, or
decision notes, but must not contain raw credentials, bearer links, session
material, mailbox content, or provider payloads.

One-to-one record-email acceptance records only the record, durable delivery,
and resulting email-message IDs. Explicit uncertain resolution records the
delivery ID and the finite `confirmed_sent`, `retry`, or `not_sent` decision.
Neither event contains sender/recipient addresses, subject/body, RFC/provider
identifiers, tracking tokens/links, idempotency keys, or request hashes; the
tenant-scoped delivery and email ledgers remain the detailed evidence.

## Retention and deletion

Audit rows are append-only and retained for the workspace lifetime. PostgreSQL
rejects direct row updates, deletes, and table truncation while the owning workspace exists. There
is no age-based audit purge job and no operator API for rewriting history.

The foreign-key cascade is deliberately retained for the future approved
tenant-deletion transaction. Until `P3-D4` defines retention, legal holds,
backup expiry, and provider-ledger exceptions, Open CRM does not claim that a
canceled hosted workspace is deleted. Implementing that lifecycle must export
and verify the audit package before the workspace cascade and must retain the
separately approved deletion certificate outside the deleted tenant.

## Review and export

Owners and admins can review the newest events and filter by exact event type.
The same filter is available as a chronological CSV convenience export with an
explicit 10,000-row ceiling; Open CRM returns `EXPORT_TOO_LARGE` instead of a
partial file. Text cells that spreadsheet software could interpret as formulas
are neutralized. Every successful audit CSV download creates its own audit
event before bytes are returned.

The complete portable workspace ZIP remains the authoritative untruncated
tenant package. Its repeatable-read `audit_events.ndjson` dataset includes the
full retained audit history and typed metadata, and its manifest reports the
row count. Export files contain business and identity evidence and must be
stored and shared as sensitive customer data.
