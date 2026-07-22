# Operations Runbook

This runbook covers the production Docker Compose deployment used by `scripts/remote-deploy.sh`.

## Request Tracing

- Every API response includes `X-Request-Id`.
- API request logs include `request_id`, `method`, bounded route pattern,
  `status`, `duration_ms`, and response `bytes`.
- In production (`GO_ENV=production`), API logs are JSON so request IDs can be searched directly in log tooling.
- Raw URLs, query strings, client addresses, recipients, subjects, phone numbers,
  meeting titles, and provider IDs are intentionally excluded from global and
  provider success logs. Never add tenant, record, message, or credential data
  as a metric label.

## Monitoring And Alerts

`GET /metrics` exposes dependency-free Prometheus text metrics for bounded HTTP
route/status/latency, PostgreSQL readiness, aggregate (not tenant-labeled)
durable queue state/lag, worker outcomes, Postmark/SMTP/Gmail/Microsoft send and
OAuth-refresh outcomes, and verified
backup/restore evidence. It also reports aggregate notification backlog, age,
reviewed event mix, per-recipient concentration, notification retention, and
email-engagement cleanup outcomes plus one-to-one record-email and
connected-mailbox reply claims, uncertain outcomes, and recovery-pass health
without tenant, recipient, message, or record labels. Public-lead counters report only bounded challenge,
accepted, replayed, rejected, and internal-error outcomes. Lead-review gauges
report aggregate unreviewed, legitimate, and spam states plus the age of the
oldest unreviewed submission without tenant, form, contact, or payload labels.
Password-recovery gauges report current
non-expired links plus stale-pending and latest-failed delivery counts without
user, address, or tenant labels. System-email feedback gauges report aggregate
Postmark bounces, complaints, and authenticated-but-unapplied events in the
last 24 hours; they likewise contain no user, recipient, provider ID, or tenant
labels. The route is hidden with `404` unless
`METRICS_BEARER_TOKEN` contains at least 32 characters; invalid credentials
receive `401`. The deployment Compose port is loopback-bound, and the token is
still required because a reverse proxy could otherwise expose the route.

Generate and store one token outside the checkout, then copy the same value to
the protected deployment environment and the monitoring system's credentials
file:

```sh
install -d -m 700 ~/.config/open-crm
openssl rand -hex 32 > ~/.config/open-crm/metrics-token
chmod 600 ~/.config/open-crm/metrics-token
# Add: METRICS_BEARER_TOKEN=<the generated value> to .env.production.
```

After restarting the API, verify a scrape without putting the token in curl's
process arguments:

```bash
metrics_token="$(< ~/.config/open-crm/metrics-token)"
curl --fail --silent --show-error \
  --config <(printf 'header = "Authorization: Bearer %s"\n' "$metrics_token") \
  http://127.0.0.1:18089/metrics | head
unset metrics_token
```

Merge `ops/monitoring/prometheus-scrape.example.yml` into an existing
Prometheus configuration and install
`ops/monitoring/prometheus-alerts.yml` as its rule file. Update the host/port if
`API_PORT` differs. Validate before reload:

```sh
promtool check rules /etc/prometheus/rules/open-crm-alerts.yml
promtool check config /etc/prometheus/prometheus.yml
```

The reference rules alert on metrics collection or database failure, sustained
5xx ratio/p95 latency, queue collection/lag/dead letters/worker errors,
provider failures, password-recovery and system-email feedback health,
notification collection/retention/elevated recipient volume, email-engagement
retention, one-to-one record-email recovery, connected-mailbox reply recovery,
public-lead internal errors and
elevated rejections, unavailable lead-review health and submissions left
unreviewed for more than one day,
backup evidence/failure/freshness, and restore-drill failure/freshness. They do
not choose an Alertmanager destination. Before a
pilot, the operator must route critical and warning alerts to an approved
on-call destination, send a synthetic test alert, and record its receipt. Do
not place destination credentials in this repository.

### Initial pilot SLOs

Repository regression thresholds and fixture scope are defined separately in
[`performance-and-failure-budgets.md`](performance-and-failure-budgets.md); do
not reinterpret CI timings as production SLO evidence.

These are starting operational targets, not claims backed by pilot history:

- API readiness availability: at least 99.5% over a rolling 30 days, excluding
  announced maintenance.
- API 5xx ratio: below 1% over 30 days; p95 server duration below 1 second for
  ordinary JSON routes. The early-warning rules intentionally trigger at 5%
  and 2 seconds over shorter windows to avoid low-traffic noise.
- Runnable background-job lag: below 5 minutes, with no dead job unreviewed for
  more than 15 minutes during supported hours.
- Verified encrypted backup recovery point: less than 30 hours old; successful
  isolated restore drill: less than 8 days old.

Measure these during the pilot and tighten or revise them from actual traffic;
do not silently relabel a missed target as success.

### Database or API incident

1. Confirm `/healthz`, `/readyz`, `open_crm_database_up`, request error ratio,
   and p95 latency. A healthy process with failed readiness points to PostgreSQL
   or its connection path.
2. Correlate the alert window with request IDs and bounded routes in API logs.
   Do not paste `.env.production`, database URLs, or provider errors containing
   secrets into an incident channel.
3. Inspect Compose service state/logs and PostgreSQL disk/connections before
   restarting. Preserve evidence if the failure repeats.
4. Use **Deploy Recovery** below for a release regression and **Deliberate
   Production Restore** only after explicit incident authorization.

### Provider incident

1. Identify `provider` and `operation` from
   `open_crm_provider_operations_total{outcome="error"}`; inspect HTTP errors and
   background-job outcomes in the same window.
2. Confirm provider configuration/status outside Open CRM without exposing
   credentials. Correct configuration or wait for provider recovery.
3. Review dead/retryable work in **Settings > Operations**. Never replay an
   uncertain SMTP delivery until the recipient/provider evidence is checked as
   described below.
4. Confirm the counter stops increasing and the next controlled provider
   operation succeeds before resolving the alert.

### Per-user Gmail and Microsoft OAuth delivery

Set the Google or Microsoft client ID/secret only in the protected deployment
environment, and register the exact callback
`${API_BASE_URL}/api/me/email-sync/oauth/{provider}/callback`. Google connections
request delegated `gmail.readonly` and `gmail.send`; Microsoft connections
request delegated `Mail.Read` and `Mail.Send` plus `offline_access`. These are
the least-privilege read/send contracts used by Open CRM. The authoritative
provider references are the [Gmail send method](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send)
and [Microsoft Graph sendMail method](https://learn.microsoft.com/en-us/graph/api/user-sendmail?view=graph-rest-1.0).

Migration 86 deliberately leaves earlier read-only OAuth connections intact,
but their stored grant has no send evidence. **Settings > My Email** labels them
for reconnection; do not edit `oauth_scopes` or encrypted token columns in SQL.
After reconnecting, confirm the page says **Connected for sending and sync**,
send one controlled record email, run one manual sync, and then observe a
scheduled `mailbox.sync` cycle. Retain the provider's sent item, received test
reply, Open CRM mailbox entry, and bounded `provider=google|microsoft`
`operation=send|oauth_refresh` metric evidence without copying tokens or message
content into the runbook.

Token refresh is serialized per workspace/user in PostgreSQL and committed
before a send begins. Gmail returns a message resource; Microsoft `202` means
the request was accepted but does not prove final delivery. Open CRM never
automatically retries a provider send after the request starts. Treat a timeout
or connection loss as ambiguous: inspect the provider Sent folder and recipient
before approving any sequence retry in **Settings > Operations**. A
`EMAIL_OAUTH_RECONNECT_REQUIRED` response means the grant is missing send
permission or predates migration 86; reconnect it instead of replaying.

### Password recovery

Password reset is public because a user may have no valid session. It remains
available when a hosted workspace is read-only. A valid completion changes the
user's global password, consumes the one-hour link, invalidates every session,
and writes an audit event into every workspace membership.

1. Confirm `open_crm_password_resets_available` is `1`. Review
   `open_crm_password_reset_delivery_stale_pending`,
   `open_crm_password_reset_delivery_failed_24h`, the bounded
   `auth.request-password-reset` rate-limit decisions, and Postmark provider
   errors in the same window. Metrics deliberately contain no email, user, or
   tenant labels.
2. For a failed latest delivery, correct the system-email provider and have the
   user submit the same public form again. Failed delivery bypasses the
   five-minute recipient cooldown and rotates the token. For a successful or
   pending request, wait five minutes before another delivery; never bypass the
   cooldown or manufacture a raw token in SQL.
3. Treat delivery errors as potentially ambiguous. An earlier link may still
   arrive, but token rotation makes it invalid; only the newest non-expired link
   can complete. Replays and expired links fail without changing credentials.
4. `EMAIL_PROVIDER=fake` does not deliver mail. A local reset link appears only
   when `GO_ENV` is explicitly `development` or `test`. It is always omitted in
   production and for Postmark, including when an account exists. Never enable
   a development runtime mode on an internet-facing deployment.
5. After a controlled reset, verify the old password is rejected, a new login
   succeeds, an existing second-device `/auth/me` returns `401`, and the admin
   audit view contains `user.password_reset`. Do not collect or ask the user to
   share the raw reset link.

### Invitation delivery, expiry, and revocation

User invitations use a one-time link with a seven-day expiry. The raw token is
never stored and production/Postmark API responses never return the setup link.

1. In **Settings > Users**, inspect the invitation state, delivery state, and
   expiry. A pending, expired, failed, or bounced invitation may be resent;
   resend rotates both the setup token and its separate provider-correlation
   key, so every earlier link and callback becomes stale.
2. If delivery reports a failure or bounce, verify the address, correct the
   system-email provider if necessary, and use
   **Resend invitation**. The new token remains valid after a failed attempt so
   an ambiguous provider outcome cannot strand a delivered link; another retry
   safely rotates it again. Never manufacture or retrieve a token with SQL.
3. A spam complaint globally suppresses future workspace-verification,
   invitation, and password-reset mail to that user. Do not bypass the
   suppression or resend. Confirm the intended address/recipient, revoke the
   invitation, and investigate provider/domain-authentication evidence.
4. Use **Revoke invitation** when the recipient should no longer activate.
   Review and confirm the explicit warning. Revocation disables the membership,
   clears the token, invalidates sessions, quiesces effects, preserves history,
   and is idempotent. It remains available while hosted writes are suspended.
5. To invite a revoked user again, reactivate the membership and then resend.
   Reactivation alone does not create a usable setup link. Confirm final setup
   succeeds only with the newest link and the audit view records
   `user.invitation_resent` and `user.invitation_revoked` without secrets.
6. Local fake-provider links are returned only in explicit development/test
   mode. Never enable that mode on an internet-facing deployment. Escalate
   repeated delivery failures with request IDs and provider timestamps, without
   asking the recipient to share their link.

### System email feedback

Postmark does not sign webhooks with HMAC. Open CRM therefore exposes its
feedback callback only when a dedicated Basic Auth username and a password of
at least 32 characters are configured. Generate distinct credentials, store
them only in the protected deployment environment and Postmark server webhook
configuration, and set:

```text
POSTMARK_WEBHOOK_USERNAME=<dedicated username>
POSTMARK_WEBHOOK_PASSWORD=<random 32-or-more-character secret>
```

Create both **Bounce** and **SpamComplaint** webhooks for the configured
`POSTMARK_MESSAGE_STREAM` at
`https://<username>:<password>@<API_BASE_URL-host>/api/email/webhooks/postmark`.
Leave Postmark's message-content inclusion disabled. When either credential is
absent or the password is too short, the route deliberately returns `404`.
Wrong credentials receive `403`; never weaken or log them while diagnosing a
callback.

Provider contract references: [webhook overview](https://postmarkapp.com/developer/webhooks/webhooks-overview),
[bounce payload](https://postmarkapp.com/developer/webhooks/bounce-webhook), and
[spam-complaint payload](https://postmarkapp.com/developer/webhooks/spam-complaint-webhook).

Each verification, invitation, and reset delivery carries a version marker,
purpose, internal IDs, and a separate random delivery key in Postmark metadata.
Only the key digest is stored. A callback must match the normalized recipient,
active one-time flow, user, tenant where applicable, and current key before it
can change delivery state. Events from another application on a shared stream
are acknowledged but ignored. Exact retries are idempotent; reuse of a provider
event ID with changed bytes receives `403` so Postmark stops retrying.

1. Confirm `open_crm_system_email_feedback_available` is `1`, then compare
   `open_crm_system_email_bounces_24h`,
   `open_crm_system_email_complaints_24h`, and
   `open_crm_system_email_feedback_unapplied_24h` with bounded request/provider
   telemetry. The metrics contain no recipient or tenant labels.
2. For a bounce, inspect the appropriate user-facing/admin recovery state.
   Verification and invitation can rotate to a fresh attempt; password reset
   becomes immediately retryable through the generic public form.
3. Treat every complaint as a compliance incident. The exact attempt is marked,
   future system email to that user is suppressed, and tenant audit evidence is
   written. Do not clear suppression or create a replacement token manually.
4. An unapplied event is durable evidence that authenticated metadata was stale
   or mismatched. A small count can follow legitimate token rotation; a growing
   count requires checking stream selection, provider webhook configuration,
   application release, and timestamps without querying recipient data from the
   feedback ledger.
5. Feedback receipts retain only provider/type/event/message IDs, a payload
   digest, purpose, matched internal IDs, bounded bounce attributes, and times.
   They never retain recipient, subject, body, raw delivery key, or provider
   payload. Multi-instance-safe cleanup removes receipts older than 400 days in
   bounded hourly batches; do not delete recent/unapplied evidence to clear an
   alert.

### Suspected account or session compromise

Active-sign-in recovery remains available when a hosted workspace is read-only.
Open CRM stores only the workspace plus created, last-active, and expiry times
for this view; it does not collect IP addresses or browser fingerprints, so do
not claim that the UI identifies a device or location.

1. Have the user open **My Profile > Active sign-ins**. Confirm the row labeled
   **This sign-in** matches the session they intend to keep by its workspace and
   timing. The current row is deliberately protected; **Log out** is the only
   normal way to end it.
2. For one suspicious row, choose **Sign out**, review the inline confirmation,
   and confirm. For broad concern, choose **Sign out all other sessions**. A
   repeated all-other request is safe and reports zero after recovery is done.
3. Confirm the ended browser receives `401` from `/auth/me`. Review
   `user.session_revoked` or `user.other_sessions_revoked` in each affected
   workspace audit view. Audit metadata records only the revocation count where
   applicable, never a raw session token.
4. If the password may be known, complete password recovery as well. Login and
   reset serialize on the user credential row, and reset deletes sessions using
   a statement-current view; a concurrent old-password login therefore either
   loses authentication or is included in the reset invalidation.
5. Escalate unexpected recreation after the password change as an account or
   system-email incident. Preserve request IDs and audit timestamps; do not ask
   for cookies, raw reset links, passwords, or browser storage.

### Stripe hosted-billing setup and recovery

The fake billing provider is the safe self-host/development default. Enabling
Stripe makes external payment and subscription state authoritative, so do it
only with explicit approval and an account whose products, tax, portal, and
operating policy have been reviewed.

Before configuring hosted billing, note the runtime boundary: only
`BILLING_PROVIDER=stripe` enables managed trials, plan limits, suspension, and
dunning enforcement. The default fake/self-hosted mode reports `unmanaged`,
shows local usage without ceilings, and never restricts private writes, public
lead capture, or tenant workers because of stored hosted lifecycle fields.

1. In one Stripe mode (test first, live only after approval), create recurring
   Prices for each offered plan. Configure the customer portal for the approved
   payment-method, invoice, plan-change, and cancellation behavior. Open CRM
   does not silently choose proration or resubscription policy.
2. Register the public endpoint
   `${API_BASE_URL}/api/billing/webhooks/stripe` and subscribe it to
   `checkout.session.completed`, `customer.subscription.created`,
   `customer.subscription.updated`, `customer.subscription.deleted`,
   `invoice.finalized`, `invoice.updated`, `invoice.paid`,
   `invoice.payment_succeeded`, and `invoice.payment_failed`. Copy that
   endpoint's signing secret—not an API key—into the deployment secret store.
3. Set `BILLING_PROVIDER=stripe`, `STRIPE_SECRET_KEY`,
   `STRIPE_WEBHOOK_SECRET`, the offered `STRIPE_PRICE_*` values, and the exact
   public `WEB_BASE_URL`. No browser publishable key is used: Checkout and the
   portal are server-created hosted sessions. Never mix test and live keys,
   secrets, Prices, or events; the API rejects a mode mismatch.
4. Deploy through the ordinary CI-gated workflow. In a disposable workspace,
   open **Settings > Plan & Billing**, complete hosted Checkout, and wait for
   the signed subscription event before expecting access to change. The return
   URL is informational and never activates a plan. Confirm the recurring price
   in Stripe Checkout; Open CRM's in-code amount is only a catalog hint and is
   not presented as the provider charge. Confirm the portal opens,
   then exercise a failed payment, recovery, scheduled cancellation, and final
   cancellation using approved Stripe test controls. Confirm the owner/admin
   **Invoice and payment history** remains readable during suspension, reports
   provider amounts and the next retry when present, and opens only the
   provider-reconciled HTTPS invoice/PDF destinations.

   CI also runs a credential-free provider-contract acceptance against a fresh
   PostgreSQL schema. `TestHostedBillingSandboxJourneyAgainstPostgres` sends
   Stripe-shaped HTTP through the production adapter and the real app routes,
   signs raw webhook bodies, and runs provider reconciliation through the leased
   job queue. It proves Checkout and webhook replay, activation, invoice refresh,
   suspension-safe tenant invoice visibility, past-due grace, unpaid
   suspension/portal recovery, reactivation, and both
   cancellation stages without contacting Stripe. Reproduce that gate with:

   ```bash
   cd apps/api
   OPEN_CRM_TEST_DATABASE_URL='postgres://...' go test ./internal/app \
     -run '^TestHostedBillingSandboxJourneyAgainstPostgres$' -count=1
   ```

   CI then runs a separate Chromium journey against another empty PostgreSQL
   database. It provisions and verifies the workspace through the UI, reaches
   the server-created Checkout and Portal destinations, applies signed raw-body
   events, proves the redirect cannot activate a plan, exercises grace,
   suspension, recovery, scheduled/final cancellation, verifies direct-write
   denial, and downloads a suspension-safe export. Reproduce it with:

   ```bash
   cd apps/web
   OPEN_CRM_E2E_DATABASE_URL='postgres://.../open_crm_e2e_hosted?sslmode=disable' \
     OPEN_CRM_E2E_BILLING_PROVIDER=stripe npm run test:e2e:hosted
   ```

   The harness supplies `OPEN_CRM_TEST_STRIPE_API_BASE_URL` only to its
   `GO_ENV=test` API process. API startup rejects that override in every other
   environment so a production Stripe secret cannot be redirected by this
   test seam.

   This contract gate does not authorize provider activation and does not replace
   the approved credentialed Stripe test-mode deployment smoke in this step.
5. During suspension, verify `/auth/me` reports `workspaceAccess.state` as
   `read_only`, the persistent banner appears, and normal mutation controls are
   absent or disabled. An authorized direct CRM mutation must still return
   `402 SUBSCRIPTION_INACTIVE`, while a viewer receives `403` before billing is
   disclosed. Navigation, reads, CSV exports, portable workspace export, Plan &
   Billing, own profile and preferences, notification acknowledgement, and
   Operations replay remain available; the public lead form returns only `503
   FORM_UNAVAILABLE`.
   Non-billing tenant jobs should move to `retryable` with a `deferred` worker
   outcome, `subscription inactive`, a later `run_at`, and no net increase in
   `attempts`; the `billing.reconcile` job must continue. After payment recovery,
   reload or refresh the session, confirm the snapshot is `writable`, and verify
   the original jobs resume automatically.
   A `503 BILLING_CHECK_UNAVAILABLE` means policy or usage could not be read;
   treat it as a database/control-plane incident rather than bypassing it.
6. Correlate provider counters (`checkout_session`, `portal_session`,
   `webhook_verify`, and `subscription_reconcile`), bounded webhook-route
   request metrics, tenant audit events, and the durable `billing_checkout_requests`,
   `billing_webhook_events`, `billing_invoices`, and `background_jobs` ledgers.
   The owner/admin invoice view reads at most the newest 25 local ledger rows;
   it never calls Stripe on page load, never invents a retry deadline, and
   strips non-HTTPS or userinfo-bearing document URLs before they reach the UI.
   The tenant row records the last reconciliation attempt, successful provider
   observation, and bounded error. A failed receipt is
   retryable under the same Stripe event ID and payload; a duplicate processed
   event is a safe no-op. A changed payload for the same event ID and
   cross-tenant customer/subscription references fail closed.
   Open **Settings > Plan & Billing** to reconcile the tenant's measured-usage
   evidence on demand. The `billing.usage.snapshot` scheduler also discovers a
   bounded batch every 15 minutes and retains at most one successful observation
   per tenant per UTC day through the leased queue, so evidence does not depend
   on a page visit. It is deliberately exempt from suspension, just like provider
   reconciliation and offboarding export. Stripe workspaces use the signed/reconciled
   `subscription_current_period_start` and end; other workspaces use the current
   UTC month. `billing_usage_snapshots` retains the latest observation for that
   tenant/period. The displayed sources are active memberships, non-archived
   contacts/deals, sent outbound `email_messages.created_at`, successful
   `workflow_automation_runs.completed_at`, successful
   `background_jobs.completed_at`, and estimated `pg_column_size` row bytes
   across current-schema base tables with `organization_id` (excluding the
   snapshot and ephemeral capacity-reservation tables). Row bytes do not include indexes or external object
   storage. Do not invoice from these figures or add manual SQL counters: the
   message/automation/job/storage values are reconciliation evidence until their
   hosted quota contracts are approved. Existing seat/contact/deal limits use
   transactional, expiring reservations across direct writes, imports, public
   lead capture, reactivation, and archive recovery; do not edit or preallocate
   those internal claims manually. Internal browser
   REST traffic is not labeled API usage; no external API meter exists because
   no versioned external API exists.
   A failed daily observation is labeled **Billing usage snapshot** in
   **Settings > Operations**. Correct the database/source-table cause before
   replay; do not bypass the bounded identifier/source guard or edit the
   snapshot/job status manually.
7. On an incident, preserve the Stripe event ID and Open CRM request ID, correct
   the configuration or data-reference cause, and use Stripe's signed event
   redelivery. Do not edit organization plans/statuses or mark receipt rows
   processed with SQL. A 15-minute discovery loop queues Stripe workspaces once
   six-hour provider evidence is stale. The retrieval-start watermark prevents
   its current subscription and recent 25 invoices from overwriting a newer
   webhook. A transient failure retries automatically. After correcting a dead job's
   provider connectivity or tenant-reference cause, open **Settings >
   Operations**, filter **Billing reconciliation**, inspect the bounded error,
   and replay it. Cross-tenant metadata/customer/subscription disagreement must
   be corrected at the provider before replay; never bypass the check in SQL.

Stripe API behavior used by this boundary is documented in the official
[Checkout](https://docs.stripe.com/api/checkout/sessions/create),
[customer portal](https://docs.stripe.com/api/customer_portal/sessions/create),
[webhook](https://docs.stripe.com/webhooks), and
[subscription webhook](https://docs.stripe.com/billing/subscriptions/webhooks),
[subscription retrieval](https://docs.stripe.com/api/subscriptions/retrieve),
[invoice listing](https://docs.stripe.com/api/invoices/list?api-version=2024-06-20),
and [currency minor-unit rules](https://docs.stripe.com/currencies)
references.

### Import interruption or recovery

1. Open **Settings > Data Imports** and inspect both batch and worker status.
   Submission returns after the validated batch, short-lived source,
   `import.queued` audit event, and `import.execute` job commit atomically. A
   `pending`, `running`, or `retryable` worker is still active; the screen polls
   until the batch reaches a terminal outcome.
2. A worker retry resumes after the last committed 50-row checkpoint without
   another upload. If the job becomes `dead`, open **Settings > Operations**,
   filter to **CRM imports**, inspect the bounded error, correct capacity,
   membership, or database health, then replay the same job only while the
   source-retention deadline remains. Do not submit the same file with a new key
   merely to bypass an uncertain job.
3. The initiating admin must remain active. Deactivation cancels a pending job
   transactionally and marks the batch failed; submit a new reviewed request
   under an active admin, or roll back any already committed rows. A running
   checkpoint revalidates membership before each transaction and fails closed.
4. The live PostgreSQL row gives source bytes a seven-day recovery deadline.
   The worker refuses expired bytes, completion clears them immediately, and
   startup/hourly cleanup clears expired rows. Raw source is excluded from logs,
   audit metadata, error CSV, and portable workspace exports; encrypted database
   copies follow the separately approved backup-retention policy. Once expired,
   submit the original file with a new key after reviewing the mapping again.
5. Download the error CSV for skipped rows. It contains row numbers and issues,
   not source values; use the operator's original file to correct and submit
   those rows as a new batch.
6. To reverse a bad or partially failed batch, use **Roll back import**. Rollback archives only
   records unchanged since import. Changed/already archived records are reported
   as skipped and must be reviewed rather than overwritten.
7. Correlate queue/completion/rollback audit events and request IDs if counts disagree.
   Do not delete batch rows or CRM records with manual SQL during normal recovery.

### Core CSV export ceiling

1. Contacts, Clients, Deals, and Tasks export the active records matching the
   visible supported filters. Task due filters use the same exact saved-time
   boundaries as the UI: overdue, next rolling 24 hours, later, or no due date.
   Contact/client files include every active custom
   definition as a stable `custom:<key>` column. Only owners and admins can
   export.
2. The synchronous download is deliberately bounded at 10,000 matching rows.
   The service queries row 10,001 and returns `422 EXPORT_TOO_LARGE` instead of
   writing a partial file. Never treat that response as a completed export.
3. For an operational subset, apply a search, pipeline/stage/owner, task, or
   custom-field filter and export the smaller result; retain the filter and time
   with the file. Filters are not a substitute for a complete tenant package
   unless they form reviewed, non-overlapping sets.
4. A tenant requiring more than 10,000 rows in one record type should use the
   portable workspace export below. Do not raise the in-memory ceiling, run ad
   hoc production SQL, or assume several informal filtered downloads prove
   completeness.

### Portable workspace export and recovery

1. An owner or admin opens **Settings > Plan & Billing** and selects **Create
   workspace export**. This path remains available when hosted billing places
   the workspace in read-only mode. The request, its durable
   `workspace.export.generate` job, and audit evidence commit together under a
   stable idempotency key. The server permits only one pending/processing export
   per workspace, regardless of browser tabs or direct API clients.
2. The worker reads one repeatable-read, read-only PostgreSQL snapshot and
   writes a ZIP containing `manifest.json`, `README.txt`, and one NDJSON file
   per exported dataset. The manifest records row counts, creation/expiry time,
   and package format. The ready record exposes the whole-file SHA-256 and byte
   size; preserve those values with the downloaded file and verify the digest
   before relying on a transferred copy.
3. The package includes current and archived tenant business records,
   configuration, membership/user profile data, activity, audit history, and
   provider-safe operational metadata. It deliberately excludes passwords,
   sessions, verification/setup/tracking tokens, provider credentials and sync
   cursors, Stripe-sensitive references and URLs, internal billing/job ledgers,
   private mailbox content, prior export artifacts, and raw attachment bodies.
   Shared communication data is included. External file references may remain,
   but the product does not currently store attachment bodies to embed.
4. Self-service generation refuses any row above 10 MiB, more than 200 MiB
   uncompressed data, or more than 50 MiB compressed output. It also fails
   closed when a migration adds an organization-scoped table without an
   explicit exported-or-excluded classification. For a failed or dead job,
   open **Settings > Operations**, filter **Workspace export**, inspect the
   bounded error, correct the size/schema/query cause, and replay the job. Do
   not mark the export ready or copy database rows by hand.
5. At most the three newest ready artifacts remain downloadable per workspace.
   Ready artifacts otherwise expire after seven days. The hourly cleanup and ordinary
   export listing both clear expired ZIP bytes while retaining status,
   checksum, counts, audit, and expiry metadata. Generate a new package rather
   than attempting to restore an expired artifact. Cross-tenant or unauthorized
   downloads return a non-disclosing not-found/forbidden response and must not
   be bypassed.
6. A portable package is an interchange/offboarding artifact, not an automatic
   Open CRM database restore. Keep normal encrypted PostgreSQL backups for
   disaster recovery. If an approved pilot exceeds the self-service size
   ceiling, design and test a separately authorized operator-streamed export;
   never increase memory ceilings or use an unreviewed production SQL dump.

### Audit retention, review, and export

1. Owners and admins review **Settings > Audit Trail**. The list and CSV use the
   organization from the server-side session and an exact optional event-type
   filter; there is no caller-selectable tenant. A successful CSV download
   writes `audit.export_downloaded` before the file is returned.
2. Audit rows are append-only and retained for the workspace lifetime. Database
   updates, direct deletes, and table truncation fail intentionally. Do not rewrite or purge
   individual audit rows to correct wording, reduce storage, or conceal an
   incident; add a new corrective business event where the product supports it
   and preserve the original evidence. Audit rows may disappear only as part of
   deleting the parent workspace under the separately approved tenant-deletion
   policy.
3. The filtered CSV is an operator convenience, ordered oldest first and
   neutralized against spreadsheet formulas. It reads at most 10,001 candidates
   and returns `422 EXPORT_TOO_LARGE` rather than a partial file above 10,000
   matches. Narrow the exact event-type filter or use the complete portable
   workspace ZIP, whose `data/audit_events.ndjson` and manifest count are the
   authoritative full-workspace package.
4. PostgreSQL rejects a metadata value that is not an object or whose top-level
   keys resemble passwords, tokens, secrets, credentials, authorization, or
   cookies. If a producer begins failing at
   `audit_events_metadata_keys_safe`, fix the producer to store non-secret
   business evidence and rotate any credential that may have reached logs or an
   earlier system. Never weaken the constraint or put secrets under a renamed
   key. Nested values remain allowed for typed provider evidence, but producers
   should keep metadata bounded and purpose-specific.
5. The executable producer inventory in `internal/app/audit_inventory_test.go`
   fails when audit-writing source files are added, removed, or moved. Review the
   mutation class, tenant boundary, metadata, retention, and export effect before
   updating its count/digest and `docs/audit-event-policy.md`; a mechanical
   digest update is not a policy review.

### Pipeline configuration and recovery

1. Only owners and admins can change pipeline configuration under **Settings >
   Pipelines**. Capture the current pipeline and stage order before a broad
   process change; every successful create, update, and reorder also emits an
   audit event.
2. Renaming a stage is safe for existing deals because they retain the same
   stage ID. Reordering likewise changes display position only. Verify the
   updated label/order in Deals after the change rather than editing database
   rows.
3. Changing a stage between open, won, and lost is rejected with
   `409 STAGE_IN_USE` if any active or archived deal uses it. Move active deals
   deliberately and account for archived history before retrying. Do not use
   SQL to bypass this protection: outcome changes affect status and forecast
   interpretation.
4. Stage deletion is intentionally unavailable. If an obsolete unused stage
   should disappear, record that limitation and retain it until a reviewed
   archive/replacement workflow exists. Restore a mistaken label or order from
   the captured configuration and confirm the corresponding audit trail.

### Forecast interpretation and recovery

1. Open-stage probability is an organization-owned assumption from 0% to 100%
   under **Settings > Pipelines**. Won and lost stages are fixed at 100% and 0%.
   Changing probability immediately changes the live weighted forecast without
   moving attached deals; use the pipeline audit event to recover the previous
   value after a mistaken edit.
2. Dashboard periods require an inclusive start and end date, in order, and may
   span at most one year. Open deals with no expected close date remain included
   so incomplete data cannot disappear from the forecast. Won deals with no
   expected close date use their last update date. The stage-assumption list is
   the reconciliation source for probability, unweighted value, and weighted
   value.
3. The owner breakdown includes an explicit **Unassigned** row. Assigning a deal
   moves it between owner rows but does not change the team total. Quotas apply
   only to active members for the exact selected period.
4. Values are converted to the organization base currency using the latest
   configured rate. Any missing currency is listed in the UI and omitted from
   value totals; add or correct the rate and reload rather than substituting an
   undocumented estimate. Use matching expected-close filters on Deals and its
   CSV export to reconcile the dated records behind a period.
5. The complete dashboard is one repeatable-read PostgreSQL snapshot with a
   five-second execution limit. `504 DASHBOARD_TIMEOUT` means no partial panel
   was returned; retry once, then inspect database saturation and the
   `idx_activities_dashboard_recent` / `idx_contacts_dashboard_recent` plans.
   A forced pre-commit timeout rolls back the quota and snapshot together. Any
   failed request should still be reloaded before retrying so an operator never
   guesses whether a failure occurred at the commit boundary.

### Sales activity reporting and reconciliation

1. Open **Reports > Sales activity**. Every active member, including a viewer,
   may run it. From/to are inclusive UTC calendar dates and may span no more
   than 366 days. **Teammate** includes disabled members so historical work does
   not disappear when access is removed.
2. Deals created, moved, won, and lost use the deal owner saved on each event.
   Notes and tasks use the teammate who performed that activity. The UI states
   this mixed but deliberate meaning beside the teammate rows; do not compare it
   with current assignment or treat it as employee-adoption measurement.
3. Win rate is won outcomes divided by won plus lost outcomes in the window.
   Each is a real transition into an outcome; a deal reopened and closed again
   contributes another outcome. A stage's forward-exit rate is forward moves within the same pipeline, plus
   exits to a won stage, divided by every exit from that stage in the window.
   It is event-based and is not a cohort funnel or stage-to-stage velocity.
4. Deal create/change writes the ordinary activity and an event-time snapshot
   in one transaction. Pipeline, stage, outcome, deal-name, and owner edits do
   not rewrite an older event. Use **Recent deal events** and its deal link when
   reconciling a count; a repeated same-stage request creates no event.
5. **Partial event history** means the requested window starts before the shown
   tracking time for a workspace that already existed when the ledger shipped.
   Older deal events are deliberately not inferred from mutable current records.
   A newly provisioned workspace is fully covered from its creation, even when
   the selected calendar window begins earlier because no workspace records
   could predate it. Shorten a partial window to the coverage boundary or
   disclose the limitation; never backfill or edit `deal_stage_events` manually.
   Record the filters, coverage time, generated time, and request ID when
   escalating a mismatch.

### Pipeline cohort conversion and velocity reconciliation

1. Open **Reports > Pipeline conversion & velocity**. This is the fixed,
   executable cohort report at `/api/reports/pipeline-funnel`; it does not make
   the separately stored custom-funnel metadata production-capable. Every active
   member, including a viewer, may run the fixed report.
2. Select one current pipeline and one exact entry stage, inclusive UTC cohort
   creation dates, an optional teammate saved on the creation event, and a
   separate inclusive **as of** date. From through as-of may span no more than
   366 days, and as-of cannot be in the future. A malformed or foreign pipeline,
   stage, or teammate is a non-disclosing `400`; do not substitute IDs from a
   different workspace.
3. Cohort membership is fixed by the durable deal-creation event. The report
   shows each current configured stage label and order, but reach, exit, and
   forward-or-won math uses retained event-time stage positions. A skipped stage
   is not inferred, re-entry counts as another visit/exit, and moving to another
   pipeline is an exit but not forward progress. Median reach, completed-visit,
   and current-win values are elapsed 24-hour days, not calendar-day estimates.
4. **Partial event history** has the same coverage boundary as Sales activity.
   An as-of date before the requested cohort has fully matured is deliberate and
   is disclosed by the report. Record the pipeline/stage IDs, cohort dates,
   as-of date, teammate, coverage/generated times, release, and request ID when
   reconciling a mismatch; do not rewrite `deal_stage_events` or infer missing
   pre-ledger history from mutable deals.
5. A `504 REPORT_TIMEOUT` means the bounded five-second query deadline elapsed.
   Check database saturation, the request ID, and the sales event index plans,
   then retry the same bounded query after recovery. Empty cohorts are valid;
   compare their exact filters before treating them as a fault. The CI-gated
   PostgreSQL performance test keeps a tenant-isolated 500-deal cohort under a
   two-second ceiling.
6. The report is current analysis, not an immutable snapshot, scheduled report,
   or export promise. Preserve the selected filters outside Open CRM when an
   approved operating review requires a retained comparison; add durable
   snapshots only through a separately reviewed product slice.

### Contact/client touchpoints and follow-up reconciliation

1. Open a Contact or Client and inspect **Follow-up** for its latest touch and
   five most recent entries, or open **Reports > Follow-up queue** to find the
   first 25 records older than 14, 30, 60, or 90 days. Every member, including
   viewers, may read these surfaces. The owner filter includes disabled members
   so retained work does not disappear when access is removed.
2. A qualifying touch is a note, a durable task-completion event, a completed
   call, a sent/received SMS, a scheduled meeting, or a sent/received email the
   viewer may see. Ordinary record create/update/archive events, failed calls or
   messages, cancelled meetings, reminders, open-task due dates, and future
   meeting times do not move the clock. Scheduling the meeting is the touch, so
   its creation time—not its future start time—is shown.
3. A record with no qualifying touch uses its creation time. This prevents a new
   lead from appearing stale immediately while keeping the absence of real
   contact explicit. Client history includes direct Client work and work on
   currently linked Contacts; **via** / **Source** links identify the person that
   produced a rolled-up touch. Unlinking a person removes that person's work
   from the current client rollup without deleting either record's own history.
4. Private inbound email and private meetings are evaluated for the current
   viewer, so two authorized teammates may legitimately see a different latest
   touch or stale result. Shared email/meetings remain visible to all members;
   mailbox/calendar owners and meeting creators retain their private view. Do
   not disclose or copy a private source merely to reconcile another viewer's
   queue.
5. A CRM-sent email normally creates both a durable message and a fallback note.
   When their same-record, same-actor timestamps are within 30 seconds, the
   report counts the message once; if durable message logging failed, the note
   remains an `email.sent` touch. Expand **How touchpoints are calculated** and
   record the entity type/ID, viewer, threshold, source/action/time, generated
   time, and request ID before escalating. Never repair a derived touch by
   editing activity or message history with ad hoc SQL.

### Client health reconciliation

1. Open **Clients > Client health** and select organization or individual,
   **All**, **Needs attention**, **Watch**, or **Healthy**, and the 14-, 30-,
   60-, or 90-day stale window. Results are limited to active customer
   companies and active individual clients. The optional owner includes a
   disabled teammate with retained records; a foreign owner is rejected.
2. **Needs attention** means at least one overdue open task or a latest
   viewer-visible qualifying touch older than the selected threshold. **Watch**
   means follow-up is current and an open task is due within seven days.
   **Healthy** means neither. An open task without a due date is counted but
   cannot change the state. The queue and detail summary show the exact reasons
   and task counts used.
3. Organization health includes work linked directly to the client and to its
   currently linked contacts. Individual-client health is contact-scoped.
   Archived/completed tasks and archived records are excluded. Unlinking a
   contact removes its current rollup without deleting history.
4. Private email and meetings follow the touchpoint visibility rules above, so
   authorized viewers can legitimately receive different health results for
   the same client. Do not copy or expose private content to force agreement.
   Open CRM has no issue record, so health deliberately makes no open-issue
   claim.
5. Reconcile a surprising row through its displayed touch source and task list,
   then record entity type/ID, viewer, filters, reasons, counts, generated time,
   and request ID. Correct the source task, relationship, or follow-up record
   through normal application workflows; the derived health value is not a
   mutable status and must not be patched with ad hoc SQL.

### Client-period activity reconciliation

1. Open **Reports > Client activity**. Every active member, including a viewer,
   may select organization customers or individual clients, an inclusive UTC
   from/to window of at most 366 days, current retained owner, and all/with/
   without activity. The default is the latest 30 UTC calendar days. Results
   stop at 100 rows and put no-activity clients first; narrow owner or activity
   when the screen says more rows match.
2. Organization counts combine direct client work with work on contacts that
   are currently linked to that company. Individual counts stay contact-scoped.
   The touchpoint source/deduplication and private email/meeting visibility rules
   above apply unchanged, so authorized viewers can legitimately see different
   totals. The owner filter is the client's current retained owner, not a
   reconstructed event-time owner.
3. **Touches** counts qualifying notes, durable task completions, successful
   calls/SMS, scheduled meetings, and visible sent/received email in the period.
   **Notes**, **Tasks completed**, and **Active days** are exact subsets/context;
   the latest-period link identifies the source record. Creation, edits,
   failures, reminders, future due dates, and health derivations do not count.
4. The report totals honor client type, date window, viewer privacy, and current
   owner but intentionally remain unchanged when switching between all/with/
   without activity; the match count and rows honor that last filter. A
   `REPORT_TIMEOUT` means the five-second query deadline was reached. Narrow the
   period or owner, retain the request ID and filters, and investigate database
   health/query plans before retrying; do not increase the deadline ad hoc.
5. This is a current-client period rollup, not a historical-health ledger.
   Never infer a prior Healthy/Watch/Needs-attention state from today's derived
   state or patch counts with SQL. Correct the source record or current link in
   normal product workflows, and add snapshots only through a separately
   reviewed persisted contract if pilot decisions require real health changes.

### Client review and renewal task recovery

1. An active customer company or individual client may have one review or
   renewal schedule. Its current obligation is an ordinary assigned task, and
   Dashboard groups it as overdue, due within 30 days, or later. The schedule is
   follow-up metadata only; it is not subscription billing, a contract renewal,
   or evidence that a legal notice was delivered.
2. Editing the current task's due time or assignee updates the schedule in the
   same transaction. Completing a one-time task marks the schedule completed;
   reopening it restores the obligation. Completing a recurring task creates
   exactly one next task from the original 1-, 3-, 6-, or 12-month cadence and
   skips missed periods until the due time is future. Replaying the old
   completion must not create another task.
3. Direct archive and bulk mutation of a managed task are rejected. Client
   archive, demotion from customer, and duplicate merge are likewise rejected
   while a schedule is active. Use **Clear schedule** on the client first; this
   archives an open generated task, removes its pending reminder state, and
   records client activity/audit evidence. Then retry the intended operation.
4. If the dashboard, client, and task disagree, record the tenant-safe client
   type/ID, schedule task link, due time, assignee, cadence, task status, request
   ID, and related activity/audit/job evidence. Retry the normal update after
   checking the assignee remains active. Do not edit `client_review_schedules`,
   tasks, reminders, or jobs directly in production.

### Product catalog capacity and recovery

1. **Settings > Product Catalog** shows exact 50-row pages across active and
   inactive definitions. Search is a literal case-insensitive name/SKU match;
   `%` and `_` are ordinary characters. Use the status filter to review inactive
   history instead of assuming the first page is the complete catalog.
2. At most 100 definitions may be active in one workspace. The API serializes
   this ceiling across instances and returns `409 CATALOG_ACTIVE_LIMIT` when a
   create or reactivation would exceed it. Archive an obsolete active definition
   through the normal UI, then retry. Archiving does not rewrite existing deal
   lines, finalized PDFs, or certificates; inactive definitions remain visible
   to management but are not offered for new quote lines.
3. A writer disabled or downgraded during a mutation receives `403`; a foreign
   or missing item remains `404`. Duplicate non-empty SKUs return `409`. Confirm
   current membership, tenant, and the existing SKU before retrying. Do not
   bypass the active ceiling or change catalog rows with production SQL.

### Draft, finalized quote, and signature reconciliation

1. The deal's **Line items** are saved CRM data. A catalog selection copies its
   name, SKU, type, unit, price, and currency into the line item, so later
   catalog edits do not rewrite an existing proposal. Saving the complete list
   replaces the prior list, calculates subtotal/discount/tax/total, updates the
   deal value, and adds `deal.line_items_updated` activity in one transaction.
2. **Download current-data draft PDF** renders the live deal and saved line
   items at request time. Regenerating after a deal, relationship, stage, or
   line-item edit may produce different content and filename. It is not customer
   evidence.
3. **Finalize quote version** requires saved line items plus recipient, validity,
   and terms. A writer may use custom terms or choose one exact active revision
   from **Settings > Quote Templates**. A selected template locks its terms in
   the UI and snapshots the template identity, revision, rendered delivery
   subject/message, and signature default; later template edits never rewrite
   an existing quote. One transaction allocates the deal version and preserves
   the exact identity, line-item, totals, terms, preparation defaults, PDF bytes,
   SHA-256, approval requirement, and workspace-base currency disclosure with
   activity/audit evidence. A foreign-currency quote
   uses the newest tenant rate effective on or before the document's UTC date;
   future and other-tenant rates are ignored. `422 QUOTE_FX_RATE_REQUIRED`
   means no effective local rate exists: add it under **Settings > Business
   Profile**, then retry the identical request with the same idempotency key.
   Unsaved line changes block the UI action. Downloads
   are tenant/deal/quote scoped and private/no-store. Follow the retry, digest,
   and correction procedure in `docs/versioned-quotes.md`; never edit a stored
   version in place.
4. **Request electronic signature** is available only while delivering a
   finalized, unexpired version. An expired version is rejected before a
   delivery intent or provider call; finalize a new version rather than sending
   an unusable link. A valid request creates one native record bound to that quote,
   recipient, immutable PDF, and delivery. Mailbox-provider acceptance activates
   the customer ceremony. Staff cannot create detached requests or mark one
   signed/declined; they may only **Void unsigned request** while it is sent.
   Historical manual proposal rows remain visible as read-only non-evidence.
5. If live totals appear wrong, inspect quantity, unit price, discount, tax rate,
   currency, and saved activity before editing. A failed cross-tenant or invalid
   catalog reference changes nothing. Correct through the deal UI and download
   a new draft or finalized version; do not edit line items, quote/proposal rows,
   stored bytes, timestamps, or activity with ad hoc SQL. The PDF/customer page
   labels converted totals as reporting equivalents; the amount due remains the
   quote currency. Later rate edits do not change retained versions, and a
   reissue selects the rate effective for its new document date. Pre-control
   versions remain labeled legacy rather than receiving an estimated backfill.
   Template and independent-approval evidence follow the procedure below.

### Quote preparation and independent approval

1. Owners/admins manage reusable preparation under **Settings > Quote
   Templates**. Names are unique among active templates. Terms, a 1–366-day
   validity, delivery subject/message, supported merge fields, signature
   default, and per-template approval policy are bounded. Editing creates a new
   revision; archiving removes the template from future finalization without
   changing any retained quote reference. A stale edit or archive returns
   `409 QUOTE_TEMPLATE_CHANGED`; reload before retrying rather than overwriting
   another administrator's revision.
2. **Require approval for every quote** applies workspace-wide. Enabling it, or
   saving a template that requires approval, needs at least one other active
   owner/admin. Finalization rechecks the same independent-review condition so
   a later membership change fails before a quote is created. Add/reactivate an
   appropriate reviewer or make review optional; do not patch policy rows to
   bypass `422 QUOTE_APPROVER_REQUIRED`.
3. A required quote is created as **Pending independent review** and cannot be
   delivered. The pending queue shows the quote number, deal, recipient, amount,
   requester, time, and exact PDF SHA-256. A different active owner/admin must
   download/review the retained PDF and choose **Approve exact PDF** or **Reject
   with note**. The requester and quote creator cannot decide it. Approval
   permits delivery but is not customer acceptance, signature, accounting
   authorization, legal advice, or a deal close.
4. Decisions are terminal and bind the approver, time, optional approval note or
   required rejection note, quote identity, and retained PDF digest. The client
   reuses the same 16–200-character idempotency key after a timeout. An exact
   replay returns the evidence; a changed key/body or attempt to alter a
   terminal decision returns `409 IDEMPOTENCY_CONFLICT`. A rejected version
   stays immutable and non-deliverable; correct live data and finalize a new
   version. An expired reissue retains the source template revision and creates
   a fresh approval request when the source required review.
5. Delivery preparation and provider claim both recheck approval. A prepared
   effect therefore cannot cross the mailbox boundary when retained approved
   evidence for the exact PDF digest is absent. `409 QUOTE_APPROVAL_REQUIRED`
   means the retained decision is still pending; `409
   QUOTE_APPROVAL_REJECTED` means a new corrected version is required. Never
   update `deal_quote_approvals`, template revisions, quote hashes, or delivery
   rows directly.
6. Monitor `open_crm_quote_approval_pending`,
   `open_crm_quote_approval_approved`, `open_crm_quote_approval_rejected`, and
   `open_crm_quote_approval_oldest_pending_age_seconds`. The stale warning fires
   when the oldest pending item exceeds 24 hours for 15 minutes. Confirm the
   requester still needs the quote, the reviewer is active and independent,
   and the PDF digest matches before deciding. Resolve the alert only after the
   queue has been deliberately reviewed. Portable workspace export includes
   templates, policy, and business decision evidence but excludes decision
   idempotency/request hashes.

### Finalized quote delivery and receipt recovery

Finalized quote delivery uses the sender's connected SMTP, Google, or Microsoft
mailbox. The API must have a valid `CREDENTIAL_ENCRYPTION_KEY` and public
`WEB_BASE_URL`; the latter becomes the customer link origin. A send snapshots
the exact mailbox sender, quote recipient, subject, body, stable RFC
`Message-ID`, access expiry, and digest-only access/idempotency material before
crossing the provider boundary. Never copy the customer token or link into an
incident ticket or application log.

For a review-required quote, delivery preparation and the provider claim both
require retained `approved` evidence for the exact PDF digest. Pending or
rejected versions never cross the mailbox boundary.

1. Interpret delivery **Sent** only as provider acceptance plus committed CRM
   email, activity, and audit evidence. It does not prove inbox placement.
   Link-access and PDF-download counts can include security scanners and
   reloads. **Receipt confirmed** acknowledges delivery only. A separate
   signature request becomes **Signed** only after the recipient link submits
   the exact expected name and retained consent statement; that terminal effect
   retains its own activity, audit, and certificate evidence.
2. Startup and minutely recovery move a `sending` claim older than five minutes
   to `uncertain` without contacting the provider again. Monitor
   `open_crm_quote_deliveries_available`,
   `open_crm_quote_delivery_sending`,
   `open_crm_quote_delivery_stale_sending`,
   `open_crm_quote_delivery_uncertain`, and the recovery last-run metrics. They
   contain no workspace, sender, recipient, quote, or token labels.
   Also monitor `open_crm_quote_signature_awaiting_response`,
   `open_crm_quote_signature_expired`, `open_crm_quote_signature_signed`,
   `open_crm_quote_signature_declined`, and
   `open_crm_quote_signature_voided`, plus
   `open_crm_quote_signature_awaiting_conversion` and
   `open_crm_quote_signature_converted`. An expired unsigned request alerts
   after 15 minutes so the sender can review and deliberately reissue if
   appropriate. A signed request on an open, unarchived deal awaiting conversion
   alerts after 30 minutes; it requires a staff business decision, not an
   automatic stage change. A signed request on a deal deliberately closed or
   archived another way remains retained evidence but is not an actionable
   conversion alert.
3. For **Needs resolution**, the original sender searches the exact quote
   recipient/subject/time and RFC `Message-ID` in the connected mailbox Sent
   folder. If present, choose **Confirm in Sent folder**. If definitely absent,
   choose **Retry after checking** or **Mark not sent**. A retry is a new
   provider attempt using the same durable delivery and stable message ID, so
   approve it only after the Sent-folder check. Owners/admins may confirm or
   reject another sender's delivery but cannot retry as that sender.
4. An expired quote cannot start any new review or signature delivery. If its
   commercial content remains correct, use **Reissue this commercial version**
   with a new future validity date. This creates a new PDF/version/digest and
   preserves an explicit source/replacement lineage. It also voids an expired
   pending signature request in the same transaction. Reissue is blocked for a
   signed quote, a quote with `prepared`, `sending`, or `uncertain` delivery,
   a closed/archived deal, or a source already replaced. Resolve ambiguous
   delivery first; revise live deal data and finalize normally when commercial
   content changed. Never extend `valid_until` or repair lineage with SQL.
5. A definitely failed signature delivery voids its attached draft request and
   permits a new delivery/request. Marking an uncertain delivery **not sent**
   does the same; confirming it sent activates the existing request. Never
   create or repair a request independently of its delivery. Suppression and
   current sender identity are rechecked at the provider boundary. Do not work
   around a suppression, sender mismatch, unresolved delivery, expired public
   link, or customer decision by editing the database; correct the source state
   and use the normal UI. Use the request ID and aggregate/provider telemetry
   rather than expecting raw infrastructure errors in the quote record.
6. Disabling or revoking the sender atomically fails any `prepared` delivery
   before a provider call and voids its draft signature request. It moves an
   already claimed `sending` delivery to `uncertain` without guessing whether
   the message arrived. A disabled sender cannot claim or retry it. An
   owner/admin must use the same Sent-folder evidence and resolution procedure
   above; never reactivate a teammate merely to bypass an unresolved effect.
7. Customer preview, PDF, and signed-certificate routes are private/no-store
   bearer links with shared PostgreSQL budgets of 120 reads/client/minute;
   receipt, sign, and decline each have 20 writes/client/minute. An invalid token
   is non-disclosing `404`; an expired link or signing deadline is `410`. If a
   customer needs access after expiry, review the quote and create a new
   delivery/request rather than extending stored timestamps.
8. For a signed request, compare the customer and staff certificate response
   `X-Open-CRM-Content-SHA256`, then confirm the certificate includes the quote
   PDF digest, expected/typed names, consent, signing time, and authentication
   method. Do not regenerate or replace retained certificate bytes. Enforceability
   is agreement- and jurisdiction-dependent; escalate legal-policy questions
   instead of editing evidence.
9. For **Convert signed quote to won**, first confirm the certificate digest and
   customer identity evidence, then choose the actual same-workspace won stage,
   required reason, and concise close notes. The action atomically commits the
   stage event, outcome, automation, client handoff, and immutable conversion
   snapshots. If the request times out, retry the identical request; its digest-
   only idempotency evidence prevents a second effect. A changed request
   conflicts, and a new key cannot reuse already converted evidence. The public
   signer never chooses the stage and never closes the deal automatically.
10. To correct the outcome later, use the ordinary stage control to reopen the
   deal. This clears current close context but retains the signed-quote
   conversion record. Retrying the original conversion returns the current deal
   and does not undo that deliberate correction. Never clear conversion columns,
   replace its activity, or regenerate certificate evidence with SQL.
11. Resolve alerts only after recovery is succeeding, no stale sends remain,
   every uncertain item has explicit operator evidence, and expired unsigned
   and signed-unconverted requests have been reviewed. Portable workspace export
   includes delivery, receipt, consent, certificate, and conversion evidence
   while excluding bearer tokens, replay hashes, and provider/RFC correlation
   identifiers.

See `docs/versioned-quotes.md` for the immutable snapshot, idempotency, and
customer-evidence boundary. Never repair `deal_quote_deliveries`,
`deal_signature_requests`, linked email rows, activity, audit, PDF/certificate
bytes, tokens, or counters with ad hoc SQL.

### Deal close review and outcome reconciliation

1. A deal's `open`, `won`, or `lost` outcome is derived only from its current
   pipeline stage. General deal edits and bulk changes do not change outcomes.
   Move the deal to a won or lost stage and choose the required reason; optional
   notes should record concise decision context, not secrets or regulated data.
2. A successful close commits the stage, derived status, reason label/code,
   notes, closing actor/time, activity, stage-event snapshot, and matching task
   automation in one transaction. The live deal and its latest close event
   should therefore agree. Sales reporting counts real transitions; reopening
   and closing again creates another outcome rather than rewriting history.
   When the close came from a native signed quote, the same transaction also
   binds the certificate/request to that activity and client handoff. Its
   retained stage and close snapshots remain historical evidence if the live
   deal is later reopened.
3. To correct a mistaken close, move the deal back to an open stage. This clears
   current close context while preserving the original event. Then move it to
   the correct closed stage with the corrected reason/notes. Reports retain both
   events intentionally; explain the correction in close notes instead of
   editing `deal_stage_events` or close timestamps with ad hoc SQL.
4. Existing outcomes from before migration 68 show **Not captured before
   close-reason tracking**. Do not invent a historical reason. Use the separate
   close-reason coverage timestamp in Sales activity when reconciling totals,
   and use the deal CSV columns for reviewed external analysis. If a current
   post-migration close lacks context, capture the deal ID, stage, actor,
   activity, event, request ID, and release before escalating; do not patch the
   database directly.
5. Close reasons are a fixed pilot vocabulary. Record pilot feedback when no
   option fits and use `Other` plus notes. Do not add organization-configurable
   reasons until observed usage justifies the migration, reporting, and rename/
   retirement semantics.

### Deal task automation and recovery

1. Owners and admins manage the pilot-safe subset under **Settings >
   Automations**: deal created, a real stage change (optionally to one stage),
   or deal archived creates an ordered playbook of 1–5 literal follow-up tasks.
   Every task has an independent 0–365-whole-day due offset measured from the
   deal event for an immediate playbook. A deal rule can instead require one
   retained human decision before creating any task; its due offsets begin at
   the approval decision. None is a delayed or background action. A rule can optionally
   require one event-time deal condition:
   value amount greater/less than or set, currency equal/not-equal/set, owner
   equal/not-equal/set, or status equal/not-equal to open, won, or lost. An
   unassigned deal does not satisfy **owner is set**.
   Other stored workflow definitions are deliberately hidden and do not gain
   partial execution merely because their schema exists.
2. Deal event, every task/activity, one run record, one immutable ordered
   action-outcome row per task, and one audit event commit together. A failure
   rolls back the entire playbook. A timed-out direct
   request can be retried normally; a repeated same-stage move is a no-op, and
   stable activity/bulk event keys prevent a completed event from creating the
   same playbook twice. Action evidence preserves the captured label, status,
   attempts, due time, and tenant-validated task ID in action order; each
   `task.automated` activity records its one-based action index plus total task
   count. The same transaction loads the tenant-scoped deal/stage snapshot and
   evaluates any condition before creating effects. A reviewed non-match keeps
   one ordered `skipped` outcome per action and only the event-time field
   referenced by that rule.
3. Every task goes to the active deal owner. If that membership is inactive at
   event time, the playbook goes to the active teammate who caused the event.
   Inspect **Recent task automation runs**, expand **Inspect action outcomes**,
   follow each **Open created task** link, and compare the task's
   `task.automated` activity with the `workflow_automation.executed` audit event
   when reconciling an outcome. The stored label is execution-time evidence and
   does not change when an admin later edits the rule.
   A `skipped` run with **condition did not match** is an expected no-task
   outcome and shows the referenced event-time field. **unsupported rule shape**
   means the rule was not in the reviewed executable contract and made no task.
   In particular, stored conditions without the explicit `deal_snapshot_v1`
   marker remain inert. Multi-task plans additionally require the exact
   `deal_task_plan_v1` marker; existing one-task rules remain executable for
   compatibility, while legacy multi-action and unknown-contract definitions
   remain inert. Edit or disable an unsupported rule through the normal admin
   UI or a reviewed API repair rather than assuming it ran. Never add either
   marker directly in SQL to activate a legacy definition.
4. An approval-gated rule names exactly one reviewer class: workspace owner,
   workspace owner/admin, or current deal owner. The triggering transaction
   captures every action, creates the pending decision and reviewer
   notifications, and exposes a `waiting_approval` run with zero tasks. Under
   **Pending workflow approvals**, an eligible teammate can approve or enter a
   required rejection note. Approval commits every captured task, reminder,
   assignment notification, action outcome, deal activity, requester
   notification, and audit event together. Rejection marks the gate decided,
   cancels the captured task actions, and creates no task. The decision always
   uses the captured plan, never a later definition body.
5. A decision rechecks the actor's current role or current deal ownership and
   both actor/requester active memberships while holding the approval/run/deal
   locks. A retry with the same 16–200-character idempotency key and exact
   decision/note returns the original result; changed reuse is a conflict.
   Editing or deactivating the definition, or disabling its initiating member,
   cancels pending work and dismisses reviewer notifications. If no active
   teammate matches the configured reviewer class at event time, the run fails
   visibly and creates no task. Monitor
   `open_crm_workflow_runs{status="waiting_approval"}` and
   `open_crm_workflow_oldest_approval_age_seconds`.
   `OpenCRMWorkflowApprovalStale` means the oldest pending decision exceeded 24
   hours; verify current role/ownership and membership before deciding or
   deliberately cancelling through a definition/member change. Never edit the
   approval, run, captured action, notification, activity, or audit rows in SQL.
6. To stop future work, deactivate the rule. Deactivation does not remove tasks
   already created; edit, complete, archive, or reassign those through normal
   task controls so the operational history remains honest. Restoring a directly
   or bulk-archived deal likewise does not delete its archive follow-up task.
   Review that task explicitly during rollback; never delete run/audit rows or
   task history with ad hoc SQL.

### Durable lead follow-up automation

1. Owners and admins manage this bounded outcome under **Settings >
   Automations**. A rule chooses one exact active lead form (or every active
   form), zero or one attribution condition (`leadSource`, `utmSource`,
   `utmMedium`, `utmCampaign`, or `sourceUrl`), one exact active teammate, a
   literal task title/description, a 0–365 whole-day creation delay, and a
   separate 0–365-day due offset measured from task creation. Other
   target/action/timing/approval shapes remain hidden and do not execute.
2. An accepted public submission snapshots the exact authorized definition and
   enqueues one `workflow.lead_follow_up` job plus its initial immutable action
   outcome in the same transaction as its contact, activity, submission,
   consent evidence, and challenge consumption.
   The run's immutable `scheduled_at` and job `run_at` are identical. Editing
   the rule afterward cannot change queued work. Deactivating it stops queued
   work deliberately; it does not remove tasks already committed. Rules created
   before scheduling support remain immediate and retain their original due
   offset until an admin edits them.
3. The worker rehydrates the retained submission, form, and contact; rechecks
   tenant ownership, rule activation, the optional condition, and active
   assignee membership; and commits the task, due/overdue reminder state,
   assignment notification, `task.automated` activity,
   `workflow_automation.executed` audit event, terminal action outcome, and
   terminal run together.
   Managed hosted suspension uses the ordinary durable billing deferral and
   preserves attempts; self-hosted workspaces are never subject to that policy.
4. Inspect **Recent task automation runs**, expand **Inspect action outcome**,
   and use **Settings > Operations** before escalating. The action shows the
   captured title, schedule, attempt count, lifecycle state, terminal reason,
   and same-workspace task link when a task exists. A future `queued` run is
   healthy until its displayed schedule;
   the browser waits until that boundary instead of polling every second.
   Once due, `queued` or `running` should be transient. `succeeded` names the
   created task. `skipped` means the captured condition did not match or the
   retained contact/source was no longer eligible. `cancelled` means
   an operator deactivated the rule or quarantined its source submission before
   execution. `failed` is terminal and
   most commonly means the captured assignee is no longer active; choose an
   active assignee for future submissions and create/reassign the current
   follow-up through normal task controls.
5. Queue claims and task creation are replay-safe. If a worker loses its
   acknowledgement after commit, the repeated job returns the same task from
   the terminal run instead of creating another. The run panel reconciles its
   exact durable job and shows the operation status, attempt count, and last
   error. A dead operation is shown as a failed run and stops contributing to
   active-run health. After fixing a transient dependency, an owner/admin can
   follow **Review and replay in Operations**, filter to **Lead follow-up
   automations**, and replay the dead job there. That control invokes the
   tenant- and dead-state-gated audited queue replay. A business
   failure whose durable job completed is terminal and intentionally has no
   replay control. Never replay healthy work before its planned time. Do not rewrite
   `background_jobs`, `workflow_automation_runs`,
   `workflow_automation_action_outcomes`, tasks, activities, or audit rows with
   manual SQL.
6. Monitor `open_crm_workflow_runs{status="queued"}`,
   `open_crm_workflow_runs{status="running"}`,
   `open_crm_workflow_runs_failed_24h`,
   `open_crm_workflow_runs_skipped_24h`, and
   `open_crm_workflow_oldest_active_age_seconds`,
   `open_crm_workflow_runs{status="waiting_approval"}`, and
   `open_crm_workflow_oldest_approval_age_seconds`. The
   metric excludes future schedules. `OpenCRMWorkflowRunStalled` indicates a
   due active run older than the normal recovery window; correlate its retained
   schedule and durable job before replay.
   `OpenCRMWorkflowRunFailed` indicates terminal failures that require an
   operator review even when the queue itself is healthy.
   `OpenCRMWorkflowApprovalStale` is a human queue, not a worker retry signal;
   verify eligibility and decide or deliberately cancel it instead of replaying
   background work.

### Lead submission spam review and recovery

1. Owners/admins open **Settings > Lead Forms**. **Lead submission review**
   defaults to the newest 50 unreviewed inquiries and can filter by exact form
   or `unreviewed`, `legitimate`, or `spam` state. **Load older submissions**
   follows the opaque creation-time/ID cursor in pages of 50 and disappears at
   the terminal page. A new capture appears on refresh rather than inside an
   older traversal. Reviewing an item refreshes from the newest page so a live
   status filter and its exact counts reconcile together. It shows retained business
   fields, attribution, contact state, and bounded follow-up
   counts. This queue contains customer-provided data and is deliberately not
   visible to members or viewers.
2. Before using follow-up automation as an operating SLA, choose a creation
   delay that leaves the team a realistic review window. **Mark spam** requires
   confirmation and accepts an optional internal note. One transaction records
   activity/audit evidence, archives the exact contact created by that
   submission, cancels every still-queued lead-follow-up run and action outcome,
   and completes its unclaimed durable job as a safe no-op. A worker already holding the run lock
   finishes first; if it committed a task, that task and run remain explicit
   history and the review result reports the completed effect.
3. Do not restore a spam contact from **Archived Records**. That screen names
   the quarantine and refuses the generic restore so review and automation state
   cannot diverge. Use **Recover as legitimate** in Lead Forms. Recovery restores
   only a contact whose exact archive timestamp belongs to that spam decision,
   checks hosted contact capacity, records a new review version, and creates one
   successor for the latest spam-cancelled run per rule. It never rewrites a
   cancelled run or duplicates a task already completed. The successor honors
   the original future schedule, or becomes runnable now when that boundary has
   passed. The run panel therefore retains the cancelled predecessor and queued
   successor as separate action evidence instead of rewriting history.
4. Exact retries use a tenant/submission-bound digest-only request ledger.
   Replays at the same review version return the retained effect counts; a
   historical delayed retry returns the current decision without reapplying its
   older effects. Changed key reuse fails with `IDEMPOTENCY_CONFLICT`. The ledger
   is internal security material and is excluded from workspace exports.
   `REVIEW_CONFLICT` means the
   contact or review changed between the capacity preflight and transaction;
   refresh before deciding again. `PLAN_LIMIT_REACHED` means the archived contact
   cannot currently be restored under the managed plan. Never edit submission,
   contact, run, action-outcome, job, activity, or audit rows directly to bypass
   these guards.
5. Monitor `open_crm_lead_reviews_available`,
   `open_crm_lead_reviews{state="unreviewed"}`, and
   `open_crm_lead_review_oldest_unreviewed_age_seconds`.
   `OpenCRMLeadReviewMetricsUnavailable` means the aggregate review read failed.
   `OpenCRMLeadReviewStale` means at least one inquiry has waited more than the
   provisional 24-hour review window; inspect the admin queue and staffing before
   changing the threshold. These aggregate metrics expose no tenant, form,
   contact, or submitted-value labels.
6. This workflow is human classification and recovery, not a reputation engine,
   volumetric edge control, or automatic qualification/routing system. Keep the
   scoring/routing foundation hidden until an approved edge/WAF, configurable
   mapping, simulation, observability, and acceptance evidence complete that
   separate outcome.

### Bulk change recovery

1. Open the affected Contacts, Clients, Deals, or Tasks list and expand
   **Recent bulk changes**. History is tenant scoped and remains available even
   when an archive removed every selected record from the active list.
   Legacy deal-status operations remain visible but cannot be applied or undone;
   migration 68 reconciled live status from the current stage, and all later
   outcome corrections must use the close-review stage transition above.
2. Confirm the operation type, affected count, actor, and time. An idempotent
   retry of the original request returns the same operation rather than applying
   it twice.
3. Select **Undo** and confirm. Rollback restores only records whose version still
   matches the bulk write. Later teammate edits are left intact and reported as
   skipped; `partially_rolled_back` is therefore a safe review state, not a reason
   to force database changes.
4. Correlate `bulk_operation.completed` / `bulk_operation.rolled_back` audit
   events with the per-record `*.bulk_*` activity entries if counts disagree.
   Resolve skipped records individually after reviewing their current values.
5. Do not update `bulk_operations`, `bulk_operation_rows`, or CRM records with
   manual SQL during normal recovery.

### Duplicate merge review and recovery

1. Before confirming a merge in **Settings > Data Quality**, verify the match
   reasons, linked-work counts, chosen survivor, and each differing field. A
   merge is permanent and has no automatic undo. The source record is archived,
   not deleted, and the confirmation repeats this consequence.
2. If the merge request times out, retry from the unchanged review. The UI
   reuses the same idempotency key and request body, so a completed merge is
   returned rather than applied twice. If either record changed after review,
   the API rejects the stale version; refresh and review the current values.
3. Inspect **Recent permanent merges**, the survivor's
   `duplicate.merged` activity, and the matching audit event before escalating.
   Import, bulk-operation, and audit ledgers intentionally keep their original
   record IDs; this is historical accuracy, not an orphaned relationship.
4. If an operator chose the wrong survivor or field, stop further edits to the
   survivor and record the affected merge, actor, and time. The archived source
   still retains its record row, but linked work and selected values have been
   consolidated. Recovery therefore requires deliberate record-by-record
   reconciliation, or an approved database restore into an isolated environment
   for comparison; do not unarchive or rewrite merge/history rows with ad hoc SQL.

### Archived-record recovery and retention

1. Open **Settings > Archived Records**. Every active member can inspect the
   tenant-scoped history; owners, admins, and members can restore, while viewers
   are intentionally read-only. Filter by record type or search a name, title,
   or exact record ID before changing anything.
2. Confirm the record label, type, owner, and archive time, then select
   **Restore**. A successful restore returns the same record ID to normal active
   views and records both a `*.restored` activity and `record.restored` audit
   event in the same transaction.
3. A `409` naming an archived dependency is recoverable: restore the linked
   company/contact before its deal, or the linked contact/company/deal before its
   independently archived task, then retry. Existing related notes, tasks,
   activity, and links are retained during archive; active related work is not
   cascaded into archive, and separately archived work must be restored itself.
4. A duplicate-merge source is visible for historical diagnosis but cannot be
   restored. Its relationships and chosen values belong to the survivor; follow
   the permanent merge recovery procedure above instead of modifying either row
   directly.
5. Core lists, exports, and report inputs omit the archived core record. Open CRM
   currently performs no automatic hard delete or time-based purge: archived
   rows and their history remain in PostgreSQL and encrypted backups until an
   explicitly approved tenant deletion/retention workflow runs after a reviewed
   portable export. Do not use ad hoc SQL to bypass these rules during normal
   recovery.

### Data-quality review

1. Open **Reports > Data quality**. These are fixed live read-only queries,
   separate from the saved table reports farther down the page. Every member
   can review the queues; no record is changed automatically.
2. Select a 14, 30, 60, or 90 day stale-deal window. The API accepts only 7–365
   days. Each queue states its rule, shows the exact current tenant count, and
   lists up to 25 affected records with the specific reason and a direct link.
3. Review missing owners, missing contact details, stale or incomplete open
   deals, and open tasks without due dates. The final queue follows the current
   business profile: service clients need a linked person, construction clients
   need a location, and product-sales organization accounts need an industry.
4. Open each linked record, correct it through the normal editor, then return to
   Reports or refresh for the next bounded batch. Archived records are excluded;
   restore one through **Settings > Archived Records** before quality review if
   it needs to re-enter normal operations.
5. If a count appears wrong, record the workspace, rule, selected stale window,
   and displayed generated time; capture the request ID from the API response or
   correlated request log. Compare only active records matching the displayed
   criterion; do not repair report counts or CRM rows with manual SQL.

### Saved table and grouped bar report execution and recovery

1. Open **Reports**. Owners, admins, and members can create or edit a saved
   table or grouped bar report; viewers can run existing active reports but
   cannot change their definitions. Production hides pre-contract grouped bars,
   line, funnel, pie, KPI, dashboard, sharing, and scheduled-delivery
   definitions.
2. Choose contacts, companies, deals, or tasks and add only the typed operators
   offered for each field. A table selects result fields and can use **No
   aggregation** or a supported summary. A grouped bar selects exactly one
   category and a record count, numeric sum, or numeric average. It stores no
   ignored row fields and always renders the exact values again in an accessible
   data table.
3. Select **Run report**. Each request uses the session workspace, omits archived
   rows, returns at most 100 rows per page and 100 pages, and stops after five
   seconds. Page counts are deliberately not presented as an exact total.
4. `REPORT_INACTIVE` means a writer must edit and activate the saved report.
   `REPORT_TIMEOUT` means narrow the filters or grouping before retrying; capture
   the request ID and route latency if the same bounded query repeats. A generic
   validation error means a retained definition no longer matches the executable
   typed field/visualization contract and should be corrected through the
   editor, not SQL. `REPORT_NOT_EXECUTABLE` identifies an unversioned historical
   bar or a retained line, funnel, pie, or KPI foundation that remains
   intentionally hidden. Recreate an old bar through the production builder if
   its grouping and aggregation are still wanted; do not promote it with SQL.
5. A missing report returns the same `404` for a nonexistent or foreign
   definition. Do not diagnose tenant ownership from that response. Use the
   authenticated request log and the digest-gated PostgreSQL evidence test when
   investigating an isolation concern.
6. Owners and admins can select **Download CSV**. The attachment runs the same
   saved tenant-scoped query with a five-second deadline, excludes archived
   rows, neutralizes formula-like spreadsheet cells, and records
   `report.export_downloaded`. It contains at most 10,000 data rows.
   `EXPORT_TOO_LARGE` means the server detected row 10,001 and returned no
   partial file or completed-download audit; narrow the saved filters before
   retrying. Members and viewers may run reports but cannot download this admin
   export. Treat a repeated timeout as a query/performance incident and retain
   the request ID; do not bypass either ceiling with direct SQL. The CI
   PostgreSQL pilot gate expands one workspace to 10,000 contacts and requires
   a 100-row table page plus the complete grouped-bar aggregation within two
   seconds and each export within five seconds, including foreign-workspace
   denial and overflow/audit checks. Repeated production latency near either
   budget is an incident even when the request still succeeds.

### Saved-view limits and stale-change recovery

1. Each teammate owns a separate contact, client, deal/job, and task catalog.
   Select **Load views** before managing a catalog; the client retrieves every
   bounded page, and the displayed count covers all view scopes for that record
   type. The supported creation ceiling is 100 stored views per teammate and
   entity. A workspace upgraded with more than 100 retains every existing view
   for apply/update/delete, but cannot create another until below the ceiling.
2. Names are at most 100 Unicode characters and a definition retains at most 25
   bounded filter pairs. Do not use saved views as event history or bulk data
   storage. A `SAVED_VIEW_LIMIT` response means delete an unused view; deletion
   immediately recovers one slot. Do not remove rows directly in PostgreSQL.
3. Update, make-default, and delete use the exact revision shown when the view
   was loaded. Making one view default also increments the displaced default's
   revision. `SAVED_VIEW_CHANGED` therefore means another tab or operation won:
   reload the catalog, confirm the current filters/default, and deliberately
   repeat the action. Never overwrite the revision or default flag with SQL.
4. The UI serializes loads and mutations, rejects obsolete record-type/scope
   responses, and validates the returned view identity. If it reports that a
   save/update/delete succeeded but reload failed, trust the successful write,
   preserve the request ID, and select **Load views** before another change.
   Repeating the original mutation without reload can correctly conflict.
5. Repeated adjacent-page latency near two seconds, inconsistent totals, a
   missing row within the advertised exact total, or failures to load legacy
   overflow are performance/data-integrity incidents. Record the teammate,
   entity type, total, page, exact release, and request ID. Use the freshly
   migrated saved-view PostgreSQL acceptance and management-index plan as the
   diagnostic baseline; do not bypass the user/tenant predicate.

### Custom-field change and recovery

1. Only owners and admins can change definitions in **Settings > Custom
   Fields**. Before creating a required field, tell record editors which value is
   expected. Existing records may remain blank, but the next create or edit of
   each record must satisfy every active required field.
2. Treat the displayed `custom:<key>` as a permanent integration contract. A
   field key and type cannot be changed; labels, order, list visibility,
   required state, and select options can. Removing a select option that is
   still stored is rejected, so update affected records before removing it.
3. Archiving asks for explicit confirmation and removes the definition from
   normal forms, lists, filters, saved-view selection, imports, and exports. The
   underlying contact/company JSON value and audit history remain in PostgreSQL
   and therefore in backups. There is no definition-restore UI in this release.
4. After an accidental archive, stop recreating fields or editing affected
   records. Record the organization, definition key, actor, and audit-event time.
   Compare values in an isolated restore if needed; any production definition
   recovery requires an approved, reviewed data repair. Do not edit JSONB values
   or definition rows ad hoc during normal operation.
5. For unexpected validation or filter results, confirm record type, active
   definition, exact operator/value, and request ID. Contact definitions never
   apply to organization-client queries, and a company-field filter intentionally
   excludes individual clients. Export the same filtered list to compare the
   stable custom column before escalating.

## Public Endpoint Abuse Controls

Authentication, workspace bootstrap, password setup, public lead challenges and submissions,
public landing/widget reads, Stripe/Postmark webhook delivery, unsubscribe
links, and email open/click tracking use separate fixed-window per-client
limits.
Rate-limited responses return `429`, a stable `RATE_LIMITED` error code, and
`Retry-After`. Forwarded client addresses are trusted only when the direct peer
is a loopback or private reverse proxy.
Workspace creation is capped at 3/client/hour; login, verification, resend, and
password setup are capped at 10/client/minute. Verification resend also has a
persisted one-minute recipient cooldown and always returns the same accepted
shape for missing, verified, throttled, and pending accounts. Provider delivery
failure leaves the tenant pending with no session or running trial; retry the
same signup payload/key after correcting the sender, or use resend once its
cooldown permits. Stripe delivery uses a separate 120/client/minute read-class
window plus its HMAC and body limit. Authenticated Postmark feedback uses its
own 120/client/minute window plus dedicated Basic Auth and a 64 KiB body limit;
invalid credentials are rejected before callback parsing. Never mark a user verified, create a
session, or change a subscription manually.

Migration `079_shared_public_rate_limits.sql` stores each window atomically in
PostgreSQL, so every API process and restart consumes the same budget. The
ledger stores only the static route scope and a one-way SHA-256 client digest,
never the raw forwarded or peer address. Counters clamp at `limit + 1`, expired
rows are deleted in bounded opportunistic batches, and a store error fails the
request closed with `503 RATE_LIMIT_UNAVAILABLE` and `Retry-After: 1`.

Public lead reads use their existing 120/client/minute scope. Challenge issuance
and submission each use a distinct 20/client/minute scope, so blind challenge
traffic cannot consume the same counter as a visitor's completed write. An
active form issues a random 256-bit token, stores only its SHA-256 digest and the
exact consent statement, makes it usable after two seconds, and expires it after
30 minutes. Issuance opportunistically removes expired challenges older than 24
hours in a bounded `SKIP LOCKED` batch. Submission requires the exact consent
checkbox and challenge-bound statement; contact, activity, submission consent
evidence, and challenge consumption commit together. A retry with the exact
normalized request returns the original result, while changed, premature,
expired, cross-form, and cross-tenant attempts receive stable non-effect errors.
New submissions leave the legacy `remote_addr` and `user_agent` columns empty;
historical values were not rewritten by the expand migration and remain subject
to the approved retention/deletion policy.

Hosted `/lp/...` pages and `/widget/...` surfaces are same-origin. An embed on a
different website must add that website's exact scheme/host/port to
`CORS_ALLOWED_ORIGINS`, because its credential-free preparation script must read
the challenge response. Verify the browser's challenge request, consent text,
two-second preparation delay, submission success, and expected Origin before
publishing. Do not use `*`, weaken the CSRF origin policy, or copy challenge
tokens into logs while troubleshooting.

Monitor `open_crm_rate_limit_decisions_total{scope,outcome}`. The reference
rules alert on any sustained `error` decision and elevated `rejected` traffic.
For a store-error alert, first correlate `open_crm_database_up`, `/readyz`, and
database saturation/connection logs; restoring PostgreSQL restores the abuse
boundary without clearing live windows. For elevated rejections, identify the
bounded scope and compare route/status request rates. Do not query or export
individual client digests as user identifiers, and do not delete active buckets
to bypass a limit. If legitimate provider webhook traffic exceeds its budget,
validate provider source and signature evidence before changing the static
policy in code and rerunning the concurrency tests. Also monitor
`open_crm_lead_submission_outcomes_total{outcome}`. Any `error` indicates an
internal/storage failure; correlate it with readiness, database locks, contact
capacity, and bounded route logs. Elevated `rejected` volume can be ordinary
invalid or automated traffic, so compare challenge issuance, acceptance, replay,
and rate-limit outcomes before blocking a source at the approved edge.

These application budgets coordinate replicas but are not a volumetric DDoS
boundary or a reputation system. The delayed one-time lead challenge raises the
cost of blind submissions but does not replace an approved production edge/WAF,
IP/domain reputation, or an accessible escalation challenge. Validate that edge
boundary before promoting public lead generation beyond foundation maturity.

## Deploy

Production deploy workflows are reusable workflows called only by
`.github/workflows/ci.yml` after backend, frontend, real-PostgreSQL browser, and
encrypted backup/restore jobs pass on `main`. A failed test, vet, format, lint,
audit, build, migration-integrity, browser, or recovery check prevents both
deploy jobs from starting. The backend deploy also verifies the public
`/healthz` and `/readyz` endpoints with bounded transport retries and exact
release matching; the frontend artifact publishes `open-crm-release.txt` and
the deploy verifies the exact commit marker over the published HTTPS URL. If the
GitHub-hosted runner cannot reach the origin through its Cloudflare region, the
backend job allows a four-minute public recovery window, emits a visible warning,
and repeats the same bounded public-hostname health and exact-release checks from
the production host. The window covers observed Cloudflare `522` recovery after
a healthy container replacement without accepting local-only readiness. A
reachable but wrong release never falls back and always fails the deployment.

The exact-release HTTPS probe does not prove that the edge rejects plaintext
HTTP. As observed on 2026-07-21, `http://crm.mendola.tech` still served the app,
GitHub Pages reported HTTPS enforcement disabled, and the Cloudflare-proxied
Pages origin did not present a certificate for the custom hostname. Treat this
as open item `P3-O5`; do not enable the GitHub Pages flag against that origin.
With approved Cloudflare access, first add a host-scoped permanent redirect from
HTTP to the identical HTTPS URL, then verify from outside the edge:

```sh
curl --fail --silent --show-error --dump-header - \
  http://crm.mendola.tech/settings/profile --output /dev/null
curl --fail --silent --show-error \
  "https://crm.mendola.tech/open-crm-release.txt?release=<git-commit-sha>"
```

The first response must be a single `301` or `308` with
`Location: https://crm.mendola.tech/settings/profile`; the second body must be
the exact intended full commit SHA. Recheck login and direct SPA routes for
loops. Add HSTS only after an observation window proves every required hostname
and callback works over HTTPS.

Do not invoke the reusable deploy workflows directly. For a manual redeploy,
rerun the successful CI workflow for the intended `main` commit so the same
quality gates remain attached to the release.

`.env.production` uses Docker Compose env-file syntax, not shell syntax. Values
such as `EMAIL_FROM_NAME=Open CRM` may contain unquoted spaces. Deployment,
rollback, backup, and restore scripts never execute that file; they load only
their explicit operational allowlist and pass the complete file to Compose.
Keep paths explicit rather than relying on shell expansion, and never add shell
commands or command substitutions to the file.

From the remote host:

```sh
cd ~/open_crm
scripts/remote-deploy.sh "$PWD" "<git-commit-sha>"
```

The workflow supplies the full Git commit SHA as the release ID. The deploy
script builds one immutable `open-crm-api:<release>` image for both migration
and API processes, starts PostgreSQL, applies compatible migrations, recreates
the API, and accepts the release only when the container is healthy and
`/readyz` reports the exact expected `X-Open-CRM-Release` header continuously
for 45 seconds. This stabilization window covers dependency or container-daemon
restarts immediately after the first successful probe. It can be configured
from 0 through 120 seconds with `OPEN_CRM_DEPLOY_STABILITY_SECONDS`; retain the
45-second production default and use a shorter value only in disposable
acceptance tests. Zero disables the observation window and is reserved for
focused script diagnostics. Atomic state and per-release manifests are retained
under `var/deploy/`.

Every migration from `056` onward must begin with one of these classifications:

```sql
-- open-crm-deploy: expand
-- open-crm-deploy: contract
```

Ordinary deploys allow only backward-compatible expand migrations. The guard
rejects destructive DDL, required new columns, and new constraints mislabeled
as expand. A contract migration is blocked unless
`ALLOW_CONTRACT_MIGRATIONS=true`; use that setting only for an approved
maintenance window after a fresh backup and restore drill. Contract deploys
disable automatic application rollback because the previous binary may not be
compatible with the changed schema. Remove the setting immediately afterward.

## Deploy Recovery

If a normal expand deployment fails its post-migration readiness check,
`remote-deploy.sh` automatically recreates the previously accepted immutable
image, verifies its release header and health, records `rolled_back` in
`var/deploy/last-deploy.json`, and exits nonzero so CI remains failed. The
disposable CI acceptance test proves failed-readiness recovery, a manual
rollback without reversing database migrations, and database-unavailable boot
recovery. A production-configured API exits before opening HTTP whenever its
database configuration is invalid or its initial connection fails. Compose's
`unless-stopped` restart policy then retries the process until PostgreSQL and
Docker DNS are usable; the `/readyz` container healthcheck prevents a partial
database-disabled application from being classified as healthy. Development
and test processes may omit `DATABASE_URL` for a health-only harness, but a
configured database connection failure is fatal in every environment.

On 2026-07-21, an external Docker daemon restart occurred immediately after a
successful deployment. PostgreSQL recovered, but the API had started while
Docker DNS was unavailable and remained alive with database services disabled;
`/healthz` passed while `/readyz` returned `503`. Restarting only the API after
PostgreSQL was healthy restored the exact release. The fail-fast startup,
readiness healthcheck, stabilization window, and disposable interruption test
above are the permanent controls for that incident class.

Check service state:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production ps
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 api
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 migrate
cat var/deploy/last-deploy.json
cat var/deploy/current-release
cat var/deploy/previous-release
```

After a host or daemon restart, PostgreSQL should become healthy and the API
should recover automatically. If `/healthz` is absent or `/readyz` remains
degraded after PostgreSQL is healthy, preserve the service logs, confirm Docker
DNS resolves `postgres` from the Compose network, and restart only the API:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production ps
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 postgres api
docker compose -f docker-compose.deploy.yml --env-file .env.production restart api
curl --fail --show-error --silent http://127.0.0.1:18089/readyz
```

Do not restart the production daemon merely to exercise this path. Use the
disposable CI acceptance or an approved maintenance window.

To deliberately restore the recorded previous application release after an
expand-only deploy, use the guarded helper. It refuses arbitrary image tags,
missing manifests, unavailable images, and rollback across a contract release:

```sh
scripts/rollback-release.sh "$PWD"
```

The helper does not reverse migrations; expand migrations are deliberately
compatible with the previous binary. If the current manifest has
`"rollbackSafe":false`, deploy a forward fix or perform the deliberate database
restore procedure after explicit incident authorization. Never set
`ALLOW_CONTRACT_MIGRATIONS=true` merely to bypass a failed ordinary deploy.

## Background Jobs And Dead-Letter Recovery

Open CRM runs calendar and task reminders, automatic mailbox sync, sequence
sends, billing reconciliation/usage snapshots, workspace exports, and lead
follow-up automation on the tenant-scoped PostgreSQL queue. Claims use expiring leases and
`FOR UPDATE SKIP LOCKED`, so multiple API instances can share work. Ordinary
failures retry with capped exponential backoff; exhausted or permanent failures
remain `dead` until an administrator reviews them.

Use **Settings > Operations** as an owner or admin to:

1. Review pending, running, retryable, and dead counts plus the oldest ready job.
2. Filter by job type or status and read the last failure.
3. Correct the underlying provider or configuration failure.
4. Replay a safe dead job. Replay resets the same job and idempotency key; it
   does not create an unrelated copy.

Every replay is tenant-scoped and written to the admin audit trail. Job payloads
contain internal identifiers, not mailbox credentials or message bodies.

Successful-job retention runs immediately at API start and hourly thereafter.
Each API instance uses `FOR UPDATE SKIP LOCKED` batches of at most 500 rows, so
multiple instances can clean safely without a global maintenance lock. Payload,
result, and error detail are compacted after 30 days; the successful row and its
idempotency key remain for 400 days before deletion. All current producers also
recheck durable source state (for example reminder, delivery, enrollment,
subscription, usage-snapshot, or export state), so work older than the queue's
400-day replay window cannot rely on the queue row as its only duplicate guard.
Retention is allowlisted to the nine currently reviewed production job types:
`billing.reconcile`, `billing.usage.snapshot`, `calendar.reminder`,
`email_sequence.send`, `import.execute`, `mailbox.sync`, `task.reminder`,
`workflow.lead_follow_up`, and `workspace.export.generate`. A new worker type retains full history until its
source-state guard is reviewed.
Pending, running, retryable, and dead jobs are never selected. Dead work stays
visible until an administrator resolves and replays it; an increasing dead count
is an incident, not data for automatic cleanup.

### Task reminder behavior

Each assigned, open task with a due time has a versioned reminder ledger. A
`task.reminder` job becomes runnable at the start of the rolling 24-hour window
and another at the exact due time. Task create/edit, automatic deal tasks, bulk
changes and rollback, archive/restore, completion/reopen, and member reassignment
refresh that generation in the same transaction as the task change.

Delivery revalidates the tenant, current version, due time, open/archive state,
active assignee, and the recipient's **My Profile > Notify me when an assigned
task is due soon or overdue** choice. A stale or opted-out job succeeds as a
recorded no-op; a delivered reminder produces one notification plus task
activity. Replaying the same successful job cannot create another notification.
If a task reminder is dead, correct the database/configuration cause, verify the
task is still open and assigned, then use the ordinary Operations replay. Do not
edit `task_reminders`, its version, or its job payload manually. Email task
reminders are intentionally not enabled.

### Deal assignment notification behavior

Creating a deal for another active teammate, changing its owner, bulk
reassignment and rollback, and member-deactivation reassignment create an
in-app `deal.assigned` notification in the same database transaction as the
owner change. The recipient's **My Profile > Notify me when a deal is assigned
to me** choice is checked at write time. Self-assignment, an unchanged owner,
an inactive recipient, and opt-out are intentional no-ops.

Each effective owner transition advances `deals.owner_assignment_version`; the
notification idempotency key includes that generation. Do not edit the version
or notification row manually. If the notification insert fails, the owner
mutation also fails and may be safely retried through the original UI/API action.
Assignment notifications are not background jobs and therefore do not appear in
the Operations replay queue.

### Notification retention and noise

Notification retention runs immediately when the API starts and hourly
thereafter. Acknowledged rows are retained for 90 days from `read_at`; unread
rows are retained for 365 days from `created_at`. Each pass removes at most 500
rows from each class with `FOR UPDATE SKIP LOCKED`, so multiple API instances
can clean concurrently and a large backlog cannot turn one pass into an
unbounded table lock. A failed later class does not conceal rows already
removed by the earlier class; the outcome and both deletion counts are emitted
for that pass. Do not delete notification rows manually to clear an alert.

The protected metrics endpoint exposes:

- `open_crm_notifications_available`, aggregate unread count and oldest-unread
  age;
- trailing-24-hour total volume, distinct organization-recipient pair count,
  and maximum volume for one pair;
- a finite event mix for `deal.assigned`, `meeting.reminder`,
  `record.activity`, `record.mentioned`, `task.assigned`, `task.due_soon`, and
  `task.overdue`; all unreviewed values are combined as `other`; and
- retention run outcomes, last-run timestamp/success, and read/unread deletion
  counters. Process counters reset on API restart; Prometheus `increase` handles
  this reset.

No metric identifies the affected tenant or user. If
`OpenCRMNotificationMetricsUnavailable` fires, check PostgreSQL readiness and
the API scrape logs first. If `OpenCRMNotificationRetentionErrors` fires, find
`notification retention failed` in structured API logs, repair the database or
migration issue, and let the next hourly pass retry. A missing initial pass or
more than two hours without a pass raises `OpenCRMNotificationRetentionStale`.
Confirm the last-run success gauge returns to `1`, its timestamp advances, and
the error counter stops increasing.

`OpenCRMNotificationRecipientVolumeHigh` is a provisional pilot noise warning,
not a customer-volume SLO. It fires when one organization-recipient pair has
more than 100 events in the trailing 24 hours. An authorized database operator
can identify the internal IDs and event mix without selecting notification
summaries or user PII:

```sql
SELECT organization_id, user_id, COUNT(*) AS events
FROM notifications
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY organization_id, user_id
ORDER BY events DESC
LIMIT 20;

SELECT event_type, COUNT(*) AS events
FROM notifications
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY event_type
ORDER BY events DESC;
```

Check for a replaying producer, an accidental broad follower fan-out, or a
new `other` event before changing thresholds. Record actual pilot volumes and
recipient feedback; revise the threshold only with that evidence. If a legal or
contractual retention requirement differs, change the policy and acceptance
tests deliberately before pilot use rather than relying on manual cleanup.

### Email engagement tracking privacy and retention

One-to-one email engagement tracking is off for every new composer and send by
default. A sender must select the 90-day option and confirm that the workspace
is authorized to collect the signals. Open CRM does not decide whether that
confirmation satisfies a particular recipient, contract, or jurisdiction; the
workspace operator owns that determination. Migration 91 immediately expires
legacy tracked messages because they have no explicit acknowledgement in the
database. See [email-engagement-tracking.md](email-engagement-tracking.md) for
the product and data contract.

During an active window, the public pixel and redirect record approximate
aggregate open/click counts and first/last timestamps. They do not store a
client address, user agent, or referrer. Mail scanners, privacy proxies, and
forwarding make these signals non-authoritative. At expiry, the API immediately
hides prior observations, open pixels become no-ops, and click links still
redirect without increasing a counter.

The API runs a pass immediately at startup and hourly thereafter. Each pass
locks at most 500 expired messages with `FOR UPDATE SKIP LOCKED`, clears the
message open token, all aggregate counts/timestamps, and per-link observation
counts/timestamps, then records a purge timestamp. Validated click tokens and
destinations remain so links in already-delivered email continue to work. A
failed pass is safe to retry, multiple API instances can clean concurrently,
and replay is idempotent. Do not edit tracking rows or tokens manually.

The protected metrics endpoint exposes run counters by `success`/`error`, the
cumulative purged-message count, and last-run timestamp/success. Process
counters reset on restart. `OpenCRMEmailTrackingRetentionErrors` means the most
recent pass failed; find `email tracking retention failed` in structured API
logs, repair the PostgreSQL or migration problem, and let the next hourly pass
retry. `OpenCRMEmailTrackingRetentionStale` means no pass completed within two
hours while PostgreSQL reports ready. Confirm the success gauge returns to `1`,
the timestamp advances, and the error counter stops increasing.

An authorized database operator can inspect aggregate lifecycle state without
selecting recipients, content, tokens, or target URLs:

```sql
SELECT CASE
         WHEN engagement_tracking_enabled = FALSE THEN 'not_enabled'
         WHEN engagement_tracking_purged_at IS NOT NULL THEN 'purged'
         WHEN engagement_tracking_expires_at <= NOW() THEN 'expired_pending_purge'
         ELSE 'active'
       END AS tracking_state,
       COUNT(*) AS messages
FROM email_messages
GROUP BY tracking_state
ORDER BY tracking_state;
```

If `expired_pending_purge` grows across two successful hourly passes, confirm
the retention index exists and inspect lock contention before changing the
bounded batch. A different retention period is a product/privacy policy change:
review it, update UI copy, tests, this runbook, and the focused policy document
together rather than changing timestamps in place.

### One-to-one record email delivery recovery

Email sent from a contact, company, or deal uses the acting teammate's own
connected SMTP, Gmail, or Microsoft mailbox. Open CRM stores the exact rendered
sender, recipient, subject/body, RFC `Message-ID`, optional tracking snapshot,
and actor-scoped request hash before contacting that provider. The send is
claimed once; the active membership, active record, exact contact/address,
sender identity, and suppression are checked again immediately before the effect,
and no ambiguous provider call is retried
automatically.

The same ledger carries explicit template tests with `purpose=test`. A preview
has no durable or provider effect and reports every remaining `{{token}}`; the
server rejects both customer and test sends while any token remains. A test is
addressed only to the acting user's current sign-in address, revalidated at
preparation and claim. It prefixes `[TEST]` plus a safety notice, forces
engagement tracking off, and does not run customer suppression/unsubscribe or
HTML rewriting. Its terminal email evidence is private and has no customer
entity link, note, or activity. Separate template-test audit events retain the
source record ID used for rendering without presenting the test as customer
history.

Provider acceptance becomes **Sent** only when the outbound email record,
record link, note, activity, delivery state, and audit event commit together.
It does not prove inbox placement. A definite rejection becomes **Failed** with
the failed email evidence retained. SMTP errors after message data begins,
OAuth outcomes that cannot prove rejection, provider acceptance followed by a
failed CRM commit, and claims interrupted for five minutes become
`uncertain`. The startup/minutely recovery pass only changes durable state; it
never calls a provider.

1. Confirm `open_crm_record_email_deliveries_available` is `1`. Inspect
   `open_crm_record_email_delivery_sending`,
   `open_crm_record_email_delivery_stale_sending`,
   `open_crm_record_email_delivery_uncertain`, and the recovery last-run
   timestamp/success. These metrics contain no workspace, record, sender,
   recipient, message, provider, or request-key labels.
2. If recovery fails or becomes stale, find
   `record email delivery recovery failed` in structured logs, repair
   PostgreSQL/migration health, and let the
   next one-minute pass retry. Never update the delivery ledger directly.
3. Open the exact contact/company/deal Email card. A template test is labeled
   separately and its recipient must be the sender's sign-in address. For an
   uncertain item, check the connected mailbox Sent folder using its exact
   recipient, subject, time, and stable RFC `Message-ID`. Choose **Confirm sent** only when present,
   **Mark not sent** only when definitely absent, or **Retry explicitly** only
   after accepting that an earlier provider acceptance could make it a
   duplicate. Only the original active sender may retry; owners/admins may
   confirm or reject another sender's item after the same evidence review.
4. Confirm-sent records the normal customer email/note/activity/audit
   transaction, or the private unlinked template-test email/audit transaction,
   without another provider effect. Mark-not-sent records a failed email and
   closes the intent under the same purpose rules. Explicit retry returns the same immutable intent to
   `prepared`, then rechecks active membership, the active record and exact
   contact/address, connected sender identity, suppression, and hosted writability
   before one new provider claim.
5. Deactivating or revoking a sender atomically fails their prepared record
   emails and moves already claimed sends to `uncertain`. Do not reactivate a
   teammate merely to bypass recovery; an owner/admin should resolve the
   evidence. A portable workspace export retains business delivery state while
   stripping idempotency/request hashes, provider correlation, and tracking
   tokens/links.
6. Resolve the alert only after stale claims are zero, every uncertain item has
   an evidence-backed terminal decision, the recovery timestamp advances, and
   a controlled new record email succeeds once on the exact deployed release.

An authorized database operator may inspect aggregate state only:

```sql
SELECT purpose, status, COUNT(*)
FROM record_email_deliveries
GROUP BY purpose, status
ORDER BY purpose, status;
```

Do not select addresses, content, message IDs, provider IDs, tracking material,
or request hashes for routine alert triage.

### Connected mailbox reply recovery

Threaded replies in **Mailbox** and **Team Inbox** always use the acting
teammate's own connected SMTP, Gmail, or Microsoft mailbox. Shared access never
grants permission to send as the mailbox that received the original message.
Private conversations remain visible to their mailbox owner and admins; only
the original sender may retry an uncertain reply. See
[email-threaded-replies.md](email-threaded-replies.md) for the state, privacy,
and idempotency contract.

The API writes a durable `prepared` intent before the provider boundary and
claims it as `sending` once. A pass runs immediately at startup and every minute
thereafter; it moves claims older than five minutes to `uncertain` without
calling the provider. SMTP errors after message data begins, OAuth requests
whose response cannot prove rejection, and provider-accepted messages whose CRM
finalization failed also become uncertain. Open CRM never automatically retries
these cases.

1. Confirm `open_crm_email_replies_available` is `1`. Inspect
   `open_crm_email_reply_sending`, `open_crm_email_reply_stale_sending`,
   `open_crm_email_reply_uncertain`, and the recovery last-run metrics. These
   values contain no workspace, mailbox, recipient, or message labels.
2. If recovery fails or becomes stale, find `email reply recovery failed` in
   structured logs, repair PostgreSQL/migration health, and let the next
   one-minute pass retry. Do not update the reply ledger directly.
3. For an uncertain item, the original sender checks the exact mailbox Sent
   folder and recipient/thread before selecting **Retry explicitly**, **Confirm
   sent**, or **Mark not sent**. Retry can duplicate a provider-accepted message
   and therefore requires a warning confirmation. Owners/admins may confirm sent
   or mark not sent after the same evidence review, but cannot retry as another
   person.
4. Confirm-sent records the durable outbound conversation entry without another
   provider effect. Mark-not-sent closes the intent as failed. Suppression and
   current connected-mailbox sender identity are checked again on an explicit
   retry; either failure stops before the provider call.
5. Resolve the alert only after no stale claims remain, every uncertain item has
   evidence-backed resolution, the recovery success timestamp advances, and a
   controlled new reply succeeds on the exact deployed release. Retain provider
   evidence without copying message content or credentials into logs/tickets.

An authorized database operator may inspect aggregate state only:

```sql
SELECT status, COUNT(*)
FROM email_reply_requests
GROUP BY status
ORDER BY status;
```

Do not select bodies, addresses, message IDs, provider IDs, or idempotency
digests for routine alert triage.

### Customer email delivery feedback

Connected IMAP, Gmail, and Microsoft mailboxes ingest standards-based delivery
status notifications (RFC 3464 DSN) and abuse feedback reports (RFC 5965 ARF)
from raw MIME. Open CRM records only terminal `failed`/`5.x` DSNs; delayed or
temporary `4.x` reports do not suppress a recipient. A report can change state
only when its original opaque RFC `Message-ID` resolves to exactly one earlier
outbound message in the same workspace and mailbox, its reported recipient does
not disagree, and the report arrived after provider acceptance. Missing,
ambiguous, foreign-tenant, wrong-mailbox, wrong-recipient, and unmatched reports
remain durable but unapplied. Provider acceptance remains unchanged; the later
`bounced` or `complaint` outcome is shown separately in **Settings > Email Log**
and **Settings > Email Sequences**.

1. Confirm `open_crm_customer_email_feedback_available` is `1`, then inspect
   `open_crm_customer_email_bounces_24h`,
   `open_crm_customer_email_complaints_24h`, and
   `open_crm_customer_email_feedback_unapplied_24h` are being scraped. These
   aggregates expose no tenant, mailbox, message, or recipient labels.
2. A correlated terminal bounce suppresses the exact recipient for future
   customer email. A complaint takes precedence over a prior bounce and must be
   treated as a compliance incident. Active or paused sequence enrollment stops
   with a suppression exit; no later step is scheduled. Do not clear suppression
   or resume the enrollment with SQL.
3. An unapplied event is intentionally fail-closed evidence. Check the connected
   mailbox, original provider/Sent item, report timestamp, reported recipient,
   and the exact release before deciding whether it is a legitimate stale report,
   a provider MIME variation, or attempted spoofing. Never manually attach it to
   a different outbound message.
4. DSN and ARF are message formats, not universal proof that the report sender is
   authentic. Exact unguessable message correlation materially narrows this risk,
   but operators must still use provider authentication/reputation evidence for
   complaint investigations. Retain a controlled bounce and complaint from each
   approved live provider before treating that provider path as validated.
5. Feedback evidence is retained for 400 days and removed in bounded hourly
   batches with the system-email feedback ledger. The retained customer-mail
   ledger contains correlation IDs and the reported recipient, but not message
   bodies or raw MIME. Portable workspace exports omit internal correlation IDs.

Standards references: [RFC 3464](https://www.rfc-editor.org/rfc/rfc3464.html)
and [RFC 5965](https://www.rfc-editor.org/rfc/rfc5965.html). Microsoft raw MIME
uses the documented Graph message `$value` representation; Gmail ingestion uses
the provider's raw message representation.

### Customer email unsubscribe and one-click validation

Customer email and sequence sends retain a visible unsubscribe link in both
text and HTML. When that public URL is absolute HTTPS, the shared SMTP/Gmail/
Microsoft MIME builder also emits `List-Unsubscribe: <https://...>` and
`List-Unsubscribe-Post: List-Unsubscribe=One-Click`. HTTP development URLs stay
body-only; production `API_BASE_URL` and reverse-proxy host/protocol forwarding
must therefore resolve to the intended public HTTPS API origin.

The public `GET /api/email-unsubscribe/{token}` is deliberately read-only. It
validates the HMAC-signed organization/recipient token and renders a generic
confirmation form, so mail-security scanners cannot opt a recipient out by
fetching a link. The same URL accepts a URL-encoded or multipart `POST` only
when its sole value is exactly `List-Unsubscribe=One-Click`; it returns `200`
without redirecting. Replays are safe. A bounce may be promoted to an explicit
unsubscribe, and a complaint always remains the strongest evidence; later
unsubscribe, manual, or bounce writes cannot downgrade it. Do not clear or
rewrite suppression evidence in SQL.

Before approving a mailbox provider for pilot use:

1. Send to a controlled recipient and inspect the raw delivered MIME for both
   headers and the HTTPS URL. Confirm a link-scanner-style `GET` leaves the
   recipient sendable until the confirmation `POST` occurs.
2. Submit both supported RFC 8058 forms, repeat the request, and verify one
   tenant-scoped suppression row plus future direct/sequence send rejection.
3. Inspect the provider-added `DKIM-Signature` and prove its signed-header list
   covers both `List-Unsubscribe` and `List-Unsubscribe-Post`. Open CRM creates
   the headers but cannot guarantee a downstream SMTP/API provider signs or
   preserves them; without this retained provider evidence, one-click UI at a
   receiver remains unvalidated.
4. Repeat through each approved SMTP, Google Workspace, and Microsoft 365 path,
   recording the exact release, provider/message identifiers, received raw
   headers, POST response, suppression result, and cleanup without retaining a
   real recipient or token in general logs.

Standards references: [RFC 8058](https://www.rfc-editor.org/rfc/rfc8058.html)
and [RFC 2369](https://www.rfc-editor.org/rfc/rfc2369.html).

### Managing email templates, snippets, and capacity

**Settings > Email Templates** loads independent exact 50-row name-ordered
pages for templates and snippets. Each search is literal and limited to the
definition name. Use the catalog-specific previous/next controls rather than
assuming the first page is complete. Record composers separately traverse and
verify every bounded page so existing definitions remain selectable; if a
catalog changes during that traversal, reopen the composer or retry after the
other writer finishes.

A workspace may store at most 100 templates and 100 snippets. The API
serializes the final slot across instances, so concurrent creates at a ceiling
produce one success and one `EMAIL_TEMPLATE_LIMIT` or `EMAIL_SNIPPET_LIMIT`.
Delete an obsolete definition through Settings to free its catalog's slot.
Deletion does not rewrite already prepared or sent email evidence, which
retains rendered content in its purpose-specific delivery/message ledger, and
the definition deletion remains in the audit trail.

Every edit and delete is bound to the revision displayed when the operator
selected the row. A `409 DEFINITION_CHANGED` means another writer changed or
deleted that definition; reload the catalog, review the new content, and apply
the intended action again. Never guess a revision, bypass a ceiling with SQL,
or rewrite template/snippet rows to repair a composer. If a legacy workspace
already exceeds a new ceiling, the complete loader preserves read/use access
but no new definition can be created until the applicable stored count falls
below 100.

### Managing sequence definitions and capacity

**Settings > Email Sequences** loads exact 50-row name-ordered pages and shows
the filtered total. Search is literal and status can be narrowed to draft,
active, or paused. Use **Previous page** and **Next page** rather than assuming
the first response is the complete retained catalog. New contact enrollments
and nurture configuration receive only the complete active set; drafts and
paused history never enter those selectors.

A workspace may activate at most 100 definitions. The API serializes the final
slot across instances, so two approvals racing for it yield one activation and
one `EMAIL_SEQUENCE_ACTIVE_LIMIT` response. Pause an obsolete active definition
before retrying the rejected approval. Existing legacy workspaces above the
ceiling can still read and use every already-active definition, but cannot add
another until their active count falls below 100. Do not delete retained
history or raise the ceiling directly in SQL; record the workload and approve a
new operating limit before changing the checked constant.

Create, update, delete, approval, and effective pause revalidate the acting
membership and append audit evidence in the same transaction. Update, delete,
and approval also bind to the revision displayed in the UI. A
`SEQUENCE_CHANGED` response means another operator changed the definition;
reload, review its complete steps, then repeat the intended action. Never retry
with a guessed revision. Repeated approval of the already-active exact revision
and repeated pause are safe and create no duplicate audit event.

### Approving and pausing sequence email

New and edited sequence definitions are drafts. An owner or admin must use
**Settings > Email Sequences > Approve & activate** before a contact can be
enrolled. Approval is bound to the displayed revision; editing a paused,
never-enrolled definition creates a new draft revision and clears approval.
Definitions with enrollment history are immutable so sent and scheduled
content cannot be silently rewritten; create a replacement sequence instead.

Any owner, admin, or member can choose **Pause sending** as a safety stop. A
pause prevents new provider attempts and causes already queued jobs to defer
without consuming attempts. A provider attempt that the worker claimed before
the pause transaction acquired its lock may still finish. Check the contact
history and the enrolling user's Sent folder before assuming it was stopped.
An owner/admin can **Approve & resume** the unchanged revision. Create, update,
delete, approval, and effective pause actions appear in Audit Trail without
retaining cadence subjects or bodies.

Do not activate or resume a definition by updating `email_sequences` directly:
the API binds status, approver, approval time, and revision together, while the
worker independently verifies the same policy at its effect boundary.

### Interpreting sequence outcomes

**Settings > Email Sequences** shows cumulative, tenant-scoped counts. Use
**View enrollments** on a sequence to reconcile those totals against individual
contacts, enrollers, timestamps, terminal reasons, and accepted/bounced/
complaint/suppressed/queued/review delivery evidence. History is newest first,
loads 50 rows at a time, and offers **Load older enrollments** while a bounded
opaque continuation remains. A **Review delivery** link on an uncertain row
opens **Settings > Operations**; perform the Sent-folder procedure below before
choosing a resolution. **Accepted**
means the configured SMTP/Gmail/Graph adapter returned success or an operator
confirmed acceptance from the provider's Sent/log evidence; it does not prove
delivery to an inbox. **Replied** means Open CRM retained an inbound message from
the matched contact in the enrolling user's mailbox after an accepted sequence
delivery and correlated it to that delivery by an exact RFC reply reference or
one unambiguous provider thread. The stored enrollment links to that exact
message and received time.
**Bounced** and **complaints** are later machine-readable delivery outcomes;
they do not rewrite the accepted count. **Finished** means the cadence exhausted its steps, **suppressed** means policy
stopped it before another send, and **review** means a delivery is quarantined as
uncertain and needs the procedure below. Historical completions from before
migration 88 remain unclassified rather than being guessed; the API exposes
those counts even though the compact settings summary does not.

The CI-gated Chromium pilot journey exercises the local production-capable
baseline against a fresh PostgreSQL database and the SMTP provider sandbox. It
creates a draft, approves the exact revision, enrolls a contact, waits for the
durable worker, requires one captured SMTP message with the merged contact,
prepared `Message-ID`, multipart body, and unsubscribe fallback, then requires
the management total and its individual contact drill-down to show exactly one
accepted and one finished outcome.
It also scans that populated screen for WCAG A/AA violations. A failure in any
part of this path blocks deployment; do not replace the provider-boundary wait
with a seeded delivery or mark the outcome accepted directly in SQL.

This sandbox proves the same generic SMTP adapter and durable state transitions
without using customer infrastructure. It does not validate downstream inbox
placement, DKIM preservation, real Gmail/Microsoft refresh behavior, or real
DSN/ARF formats. Retain the controlled provider evidence described above before
calling the capability `validated with a real provider/pilot`.

Reply detection is deliberately conservative about tenant, active contact
email, enrolling mailbox, provider-accepted time, and message correlation.
Every new durable sequence send stores an opaque RFC `Message-ID` before the
provider boundary. Gmail also returns a provider message/thread receipt; IMAP
and Gmail raw MIME preserve `Message-ID`, `In-Reply-To`, and `References`, while
Microsoft sync retrieves raw MIME while retaining its conversation identifier. An inbound message can
exit a cadence only when `In-Reply-To` or `References` names the accepted
delivery, or when a non-empty provider thread identifies exactly one eligible
enrollment. If two eligible enrollments share a thread, neither is changed.
Unrelated later messages and historical deliveries without correlation evidence
remain unclassified; cancel manually only after inspecting the retained message.
A provider timestamp earlier than acceptance is never counted. Terminal DSN
bounces and ARF complaints use the same exact tenant/mailbox/message/recipient
boundary, suppress the recipient, and exit active or paused enrollments.
Suppression is terminal for the enrollment and no later cadence step is scheduled.

### Hosted sequence send safety limits

When `BILLING_PROVIDER=stripe`, every eligible sequence provider attempt must
atomically reserve both of these PostgreSQL-coordinated fixed-window budgets:

- `SEQUENCE_TENANT_SEND_LIMIT_24H` — defaults to 1,000 per workspace in a
  24-hour window; and
- `SEQUENCE_SENDER_SEND_LIMIT_1H` — defaults to 100 per enrolling mailbox in a
  one-hour window.

These are provisional reputation/abuse safety caps, not plan quotas. They apply
across all API instances and store only hashed workspace/sender keys. A fake
billing provider—the self-hosted default—does not enable the hosted cap. Hosted
values must be positive integers no greater than 1,000,000; an invalid value
stops API startup instead of silently disabling protection.

An exhausted budget leaves the delivery queued and defers the same background
job until the denied window resets. The deferral does not consume a worker
attempt or call the mailbox provider. It appears in **Settings > Operations**
with `hosted sequence send safety limit reached`, and contributes to the
bounded `email_sequence.send` / `deferred` worker metric. Suppressed deliveries
and deliveries already finalized at preflight do not consume a budget. A pause
that wins before the worker's approval check does not consume one either; if a
pause or concurrent resolution races after budget reservation but before
delivery claim, the provider is still not called but the conservative
reservation remains until its window expires.

Do not raise either value merely to clear a queue. First confirm the queue is
expected, review provider terms/domain reputation and recent complaint/bounce
evidence, then record the approved threshold and restart the API with the new
environment value. Lowering a value takes effect against the current window;
changing a window's numeric limit does not reset its stored expiry. Never edit
`public_rate_limit_buckets` to bypass the boundary.

### Uncertain sequence email

SMTP or a mailbox provider API can accept a message before a connection failure reaches Open CRM. Those
jobs are marked `dead` and their delivery is marked `uncertain`; automatic and
generic replay cannot send them a second time.

From **Settings > Operations**:

1. Check the enrolling user's Sent folder/provider log for the recipient,
   subject, and approximate attempt time shown by the job.
2. If the message is present, choose **Confirm already sent**. Open CRM advances
   the enrollment and schedules the next step without another provider call.
3. If the message is absent, choose **Retry email** and accept the duplicate-risk
   warning. This re-arms the same delivery/job for one operator-approved attempt.
4. Recheck the Operations and Audit Trail pages after the decision.

Both choices update the delivery ledger, enrollment/next-step state, and queue
state in one database transaction. Never repair an uncertain sequence by
editing `background_jobs` alone.

For read-only diagnosis from the database host:

```sh
source .env.production
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres \
  psql -U "${POSTGRES_USER:-open_crm}" -d "${POSTGRES_DB:-open_crm}" \
  -c "SELECT organization_id, job_type, status, attempts, max_attempts, run_at, lease_expires_at, left(last_error, 200) AS last_error FROM background_jobs WHERE status IN ('running', 'retryable', 'dead') ORDER BY updated_at DESC LIMIT 100;"
```

Do not mutate job or delivery tables manually while an API worker is running.

Rerun an expand migration after fixing a migration/dependency issue:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production run --rm migrate
docker compose -f docker-compose.deploy.yml --env-file .env.production up -d api
```

## Encrypted Off-Host PostgreSQL Backups

Open CRM creates a PostgreSQL custom-format dump, validates its catalog, records
its checksum and source revision, and sends it through pinned Restic `0.19.1` to
a client-side encrypted repository. A successful run applies retention and runs
`restic check`. The scripts reject local repositories outside the disposable
acceptance test; production must use an off-host Restic backend.

Provision an object-storage bucket/repository and credentials before enabling
the schedule. Keep the repository password and backend credentials outside the
checkout and readable only by the deployment user:

```sh
install -d -m 700 ~/.config/open-crm
openssl rand -base64 48 > ~/.config/open-crm/restic-password
chmod 600 ~/.config/open-crm/restic-password
touch ~/.config/open-crm/restic-backend.env
chmod 600 ~/.config/open-crm/restic-backend.env
```

Put only the provider variables required by the selected [Restic
backend](https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html)
in `restic-backend.env` (for example, scoped object-store access keys). Add this
configuration to `.env.production`; never commit the files or their contents:

```env
RESTIC_REPOSITORY=s3:https://s3.us-east-1.amazonaws.com/EXAMPLE/open-crm
RESTIC_PASSWORD_FILE=/home/DEPLOY_USER/.config/open-crm/restic-password
RESTIC_BACKEND_ENV_FILE=/home/DEPLOY_USER/.config/open-crm/restic-backend.env
RESTIC_IMAGE=restic/restic:0.19.1@sha256:136600b6ff6843d61d355f7f71f460a166429f35de6fd11b568fece3c9a4d510
BACKUP_HOST_TAG=open-crm-production
BACKUP_TAG=open-crm-postgres
BACKUP_KEEP_DAILY=7
BACKUP_KEEP_WEEKLY=5
BACKUP_KEEP_MONTHLY=12
```

Initialize the repository once, then run and inspect the first backup:

```sh
cd ~/open_crm
scripts/init-backup-repository.sh "$PWD"
scripts/backup-postgres.sh "$PWD"
python3 -m json.tool var/backup-status/last-backup.json
python3 -m json.tool var/backup-status/last-backup-attempt.json
```

`last-backup.json` is the last verified success and therefore the source for
freshness monitoring. `last-backup-attempt.json` records the latest success or
failure. Script output is structured as stable `backup_succeeded` or
`backup_failed` lines for journal/log alerts. Database dumps contain customer
data and authentication/session metadata; Restic encryption does not make a
copied plaintext dump safe.

### Schedule backups and drills

The repository includes systemd user-unit templates for a daily backup and
weekly isolated restore drill. They are intentionally not enabled by a deploy:
the operator must first configure and test a real off-host repository.

```sh
mkdir -p ~/.config/systemd/user
cp ops/systemd/open-crm-backup.{service,timer} ~/.config/systemd/user/
cp ops/systemd/open-crm-restore-drill.{service,timer} ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now open-crm-backup.timer open-crm-restore-drill.timer
systemctl --user list-timers 'open-crm-*'
```

The units assume the deployed checkout is `%h/open_crm` and that the deployment
user can reach Docker. Adjust both service files before installation if the
path differs. User timers require a persistent user manager; on hosts that do
not keep one after logout, an administrator must enable lingering or install
equivalent system units. Inspect failures with:

```sh
journalctl --user -u open-crm-backup.service -u open-crm-restore-drill.service --since '14 days ago'
systemctl --user status open-crm-backup.service open-crm-restore-drill.service
```

## Restore Drill

The drill downloads a selected snapshot, validates the recorded checksum,
restores into a new disposable PostgreSQL 16 container on the deployment
network, runs current forward migrations, performs schema/data sanity checks,
records duration and counts, and removes the disposable database. It never
changes the live database.

```sh
cd ~/open_crm
scripts/restore-drill.sh "$PWD"
python3 -m json.tool var/backup-status/last-restore-drill.json
python3 -m json.tool var/backup-status/last-restore-drill-attempt.json
```

Set `RESTORE_SNAPSHOT=<snapshot-id>` for a historical snapshot. A drill is not
successful merely because a snapshot exists: the checksum, `pg_restore`,
forward migration, and sanity queries must all pass. CI exercises the same
workflow against disposable PostgreSQL and a temporary encrypted repository.

## Deliberate Production Restore

This operation destroys the current live database contents. Confirm the target
host, incident authorization, snapshot ID, recovery point, and a fresh backup
before continuing. Keep the API stopped throughout the destructive portion.
First extract and verify a snapshot to a protected path without overwriting an
existing file:

```sh
cd ~/open_crm
install -d -m 700 var/restore
RESTORE_SNAPSHOT=SNAPSHOT_ID scripts/extract-backup.sh "$PWD" "$PWD/var/restore/open_crm.dump"
```

Run `scripts/restore-drill.sh` against that same snapshot before replacing live
data. Then, after explicit incident approval:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production stop api
source .env.production
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres dropdb -U "${POSTGRES_USER:-open_crm}" --if-exists "${POSTGRES_DB:-open_crm}"
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres createdb -U "${POSTGRES_USER:-open_crm}" "${POSTGRES_DB:-open_crm}"
docker compose -f docker-compose.deploy.yml --env-file .env.production exec -T postgres pg_restore -U "${POSTGRES_USER:-open_crm}" -d "${POSTGRES_DB:-open_crm}" --no-owner --no-acl < var/restore/open_crm.dump
docker compose -f docker-compose.deploy.yml --env-file .env.production run --rm migrate
docker compose -f docker-compose.deploy.yml --env-file .env.production up -d api
curl --fail --show-error --silent http://127.0.0.1:18089/readyz
```

Move the extracted plaintext dump to an approved encrypted incident store or
securely remove it according to the host policy after recovery evidence is
captured. Do not automate production database replacement.

## Health Checks

- `GET /healthz` confirms the API process is serving HTTP.
- `GET /readyz` confirms required dependencies are reachable.
- The production API container healthcheck uses `/readyz`, so Compose status
  never calls a database-disabled process healthy. Docker restart behavior is
  driven by process exit, which production startup now guarantees when its
  configured database cannot be reached.
