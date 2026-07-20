# Phase 3 Decision and External-Evidence Register

Last reconciled: 2026-07-20

This register centralizes the items that cannot be completed safely through
repository changes alone. It does not weaken the completion criteria in
`project-convergence-goal.md`, and none of these rows is treated as complete
without retained evidence. Secrets, production mutations, purchases, provider
enablement, licensing changes, and consequential commercial policy remain
outside the standing local-code authorization.

The recommendation is deliberately conservative: keep self-hosted behavior
unrestricted and keep the optional hosted offer narrower than the product
until policy and provider evidence are explicit.

## Product and commercial decisions

| ID | Decision required | Options | Current recommendation | What it unblocks |
| --- | --- | --- | --- | --- |
| `P3-D1` | Hosted feature/tier contract | (A) sell only the currently enforced seat/contact/deal capacities; (B) define and enforce a reviewed feature matrix across UI/API/workers; (C) keep hosted mode non-commercial while piloting self-hosted | **A for the first hosted pilot.** Do not advertise API, SSO, automation, reporting, or other foundations as tier benefits. Add one feature gate only after its complete runtime is promoted. | Final hosted catalog copy, consistent entitlements, and commercial acceptance. |
| `P3-D2` | Period meters and overage behavior | (A) no metered billing; fixed capacity only; (B) hard period quotas; (C) measured overages reported to Stripe | **A until usage definitions, billing periods, late events, corrections, and customer-visible evidence are approved.** Existing snapshots remain evidence, not billable meters. | Meter enforcement and any usage-based price claim. |
| `P3-D3` | Upgrade, downgrade, proration, cancellation, resubscription, and dunning contract | (A) Stripe Checkout/Portal is authoritative and Open CRM makes no separate proration promise; (B) implement an application-owned policy; (C) manual support process | **A for the pilot**, with exact Portal settings and Stripe retry rules reviewed and captured before enablement. Keep current signed webhook/reconciliation state authoritative and do not infer money state from browser redirects. | Customer-facing lifecycle copy, deterministic acceptance scenarios, and support runbook. |
| `P3-D4` | Tenant deletion and retention | (A) export, immediate hard delete; (B) export, time-bounded recoverable cancellation, then verified purge; (C) indefinite retention after cancellation | **B**, but the grace period, legal holds, backup expiry, audit/provider-ledger exceptions, and owner confirmation must be approved before implementation. Until then, cancellation is read-only/recoverable and no hard-delete claim is made. | Tenant deletion API/worker, backup purge behavior, privacy copy, and deletion drill. |

## Credentialed provider evidence

| ID | Required authority/input | Safe test to run after approval | Evidence required for closure |
| --- | --- | --- | --- |
| `P3-E1` | Dedicated Stripe test account credentials, price IDs, webhook secret, and permission to create/cancel test customers/subscriptions | Run the existing app-level lifecycle through real Stripe test mode: Checkout, signed duplicate/tampered webhooks, trial, payment failure/recovery, scheduled/final cancellation, reconciliation, invoices, Portal recovery | Redacted event/customer IDs, timestamps, exact release, webhook/reconciliation logs, screenshots, and cleanup confirmation. |
| `P3-E2` | Approved Postmark sandbox/server token, sender/domain, message stream, callback credentials, and permission to send to controlled recipients | Verify signup, invitation, and password reset delivery plus bounce and spam-complaint callbacks | Redacted message IDs, callback/application state, alert evidence, exact release, and suppression/recovery results. |
| `P3-E3` | Approved Google Workspace and Microsoft 365 test mailboxes plus OAuth app credentials/consent | For each provider, connect, refresh, send, inspect Sent and raw delivered MIME, prove both RFC 8058 headers survive and are covered by the provider-added DKIM signature, verify scanner GET does not suppress and exact repeated POST does, sync an unrelated inbound message plus an exact reply, induce a controlled terminal bounce and complaint where the provider supports it, prove only exact header/provider-thread/DSN/ARF correlation exits or suppresses a sequence, and do not expose token/mail content | Redacted provider request/message/thread IDs, RFC/DKIM/header and DSN/ARF preservation result, one-click response/suppression evidence, granted scopes, refresh/sync/send/customer-feedback metrics, exact release, suppression, reconnect, and revocation behavior. |

Credentials must be supplied through the deployment secret mechanism, never
committed or pasted into retained logs. Provider tests use controlled test data
and stop before any paid or production enablement not separately approved.

## Operational evidence and infrastructure

| ID | Required authority/input | Options and recommendation | Evidence required for closure |
| --- | --- | --- | --- |
| `P3-O1` | Approved off-host backup destination and credentials | S3-compatible object lock/versioning or an equivalently independent encrypted repository. **Recommend a separate account/region with lifecycle policy and restore-only credentials for drills.** | Automated timer evidence, remote object/checksum, alert on failure/staleness, and a timed clean-host restore drill. |
| `P3-O2` | Approved metrics store and Alertmanager notification destination | Existing Prometheus rules can target email/chat/on-call. **Recommend the team's actually monitored pilot channel plus a secondary escalation.** | Production scrape success, synthetic alert receipt/resolve, provider/queue/backup incident drills, redaction review, and retention record. |
| `P3-O3` | DNS/edge control and approval for a WAF/bot challenge | Managed edge challenge, self-hosted reverse-proxy challenge, or accept application-only budgets. **Recommend an edge challenge for public lead writes while retaining the PostgreSQL limiter as defense in depth.** | Allowed/rejected browser cases, callback exclusions, fail behavior, accessibility/privacy review, and alert evidence. |
| `P3-O4` | Approved production-like load host/window and permission for disruptive recovery | Dedicated staging is preferred; a controlled production maintenance window is the fallback. **Recommend staging that matches CPU/RAM/PostgreSQL/proxy topology, followed by a bounded production smoke only.** | Same-release performance gate, deploy rollback, process/DB interruption recovery, tenant-isolation results, timings, and unresolved findings. |

## How this register is used

When one row becomes authorized, record the exact environment, release SHA,
operator, start/end time, redacted evidence location, cleanup, and result in the
relevant runbook/roadmap slice. A failed exercise remains useful evidence but
does not close the row. While any row is waiting, implementation continues on
other unblocked convergence work; the register is not a reason to stop after an
audit or documentation pass.
