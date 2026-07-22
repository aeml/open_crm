# Audit Event Policy

Last reconciled: 2026-07-22

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
  email-snippet, and email-sequence definition lifecycle and approval, plus
  executable automation configuration and transactional workflow-approval
  request, decision, unavailability, and cancellation evidence;
- archive recovery, imports and rollback, bulk changes and rollback, duplicate
  merge, client review scheduling, and lead-submission quarantine/recovery;
- billing subscription/reconciliation outcomes, identity- and customer-email
  feedback, accepted one-to-one record email, uncertain email/quote resolution,
  and background-job replay;
- immutable quote finalization, approval, delivery, receipt, signature,
  decline, reissue, conversion, and client handoff;
- workspace, filtered CRM, and audit export request/readiness/download evidence; and
- saved-report definition changes and successful saved-report CSV downloads.

Ordinary contact, company, deal, task, note, and preference edits use their
tenant-scoped activity or domain history unless the operation crosses one of
the classes above. Notification acknowledgement and read-only queries are not
administrative audit events. Adding a registered HTTP route changes the global
security-surface digest, while adding an audit producer source changes the
inventory below; both gates require an explicit review instead of silently
expanding the boundary.

Producer source count: `50`

Producer source digest: `25eb889e4de796e4d7517a3a04d8d5847bd4276521af6bc3927eedf5dff23082`

The producer digest covers production Go files that insert `audit_events`
directly or construct the shared audit record input. It is a change detector,
not proof that a schema or passing unit test completes a user outcome; relevant
handler and PostgreSQL acceptance still remain mandatory.

The 2026-07-22 source-boundary review moved the existing invitation, role, and
password-setup producers from the mixed authentication/user handler into the
focused tenant user-lifecycle handler. The mutation classes, metadata, secret
boundary, retention, and export behavior are unchanged; only the reviewed
producer source path changed.

The workflow-approval review adds three focused producers for request or
unavailable-reviewer capture, terminal decision, and definition/member-driven
cancellation. Each event commits with its tenant-scoped approval/run/action
transition. Metadata contains only definition/run/deal/task identifiers,
finite role/decision state, bounded decision or cancellation notes, and task
counts/IDs. It excludes the idempotency key and request fingerprints, which
remain digest-only in the approval ledger and are removed from portable export.

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
