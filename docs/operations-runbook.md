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
reviewed event mix, per-recipient concentration, and retention outcomes without
tenant, recipient, or record labels. Password-recovery gauges report current
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
provider failures, password-recovery and system-email feedback health, notification collection/retention/elevated recipient volume,
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

1. Open **Settings > Data Imports** and inspect the batch counts. Completed
   batches need no replay; an idempotent repeat returns the existing result.
2. If a batch remains `processing` after the request ended, select the exact
   original CSV and use **Resume with selected file**. The stored source digest,
   mapping, and idempotency key reject a different file and continue after the
   last committed 50-row checkpoint.
3. Download the error CSV for skipped rows. It contains row numbers and issues,
   not retained source values; use the operator's original file to correct and
   submit those rows as a new batch.
4. To reverse a bad batch, use **Roll back import**. Rollback archives only
   records unchanged since import. Changed/already archived records are reported
   as skipped and must be reviewed rather than overwritten.
5. Correlate completion/rollback audit events and request IDs if counts disagree.
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

### Proposal tracking and current-PDF reconciliation

1. The deal's **Line items** are saved CRM data. A catalog selection copies its
   name, SKU, type, unit, price, and currency into the line item, so later
   catalog edits do not rewrite an existing proposal. Saving the complete list
   replaces the prior list, calculates subtotal/discount/tax/total, updates the
   deal value, and adds `deal.line_items_updated` activity in one transaction.
2. **Download current quote PDF** renders the deal and saved line items at
   request time. It is not an immutable quote version, attachment, approval, or
   delivery receipt. Regenerating after a deal, relationship, stage, or line-item
   edit may produce different content and filename. Save and deliver a reviewed
   copy outside Open CRM when a fixed customer document is required.
3. **Proposal tracking** records a recipient, filename reference, and a manual
   draft/sent/signed/declined/voided status. Creating or updating it does not
   send a message, contact a provider, expose a signer page, or prove a legal
   signature. Operators should change status only after confirming the matching
   external event; `sentAt`, outcome timestamps, and deal activity help
   reconcile who recorded what.
4. If totals appear wrong, inspect quantity, unit price, discount, tax rate,
   currency, and saved activity before editing. A failed cross-tenant or invalid
   catalog reference changes nothing. Correct through the deal UI and download
   a new current PDF; do not edit line items, proposal tracking rows, timestamps,
   or activity with ad hoc SQL. Versioned delivery, approvals, provider
   webhooks, and an audit certificate remain the Phase 4 quote/signature family.

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
   or deal archived creates exactly one literal follow-up task due in 0–365
   whole days. Other stored workflow definitions are deliberately hidden and do
   not gain partial execution merely because their schema exists.
2. Deal event, task, activity, run record, and audit event commit together. A
   timed-out direct request can be retried normally; a repeated same-stage move
   is a no-op, and stable activity/bulk event keys prevent a completed event
   from creating the same rule task twice.
3. The task goes to the active deal owner. If that membership is inactive at
   event time, it goes to the active teammate who caused the event. Inspect
   **Recent task automation runs**, the task's `task.automated` activity, and
   the `workflow_automation.executed` audit event when reconciling an outcome.
   A `skipped` run means a matching legacy rule shape was unsupported and made
   no task; edit or disable it through a reviewed API repair rather than assuming
   it ran.
4. To stop future work, deactivate the rule. Deactivation does not remove tasks
   already created; edit, complete, archive, or reassign those through normal
   task controls so the operational history remains honest. Restoring a directly
   or bulk-archived deal likewise does not delete its archive follow-up task.
   Review that task explicitly during rollback; never delete run/audit rows or
   task history with ad hoc SQL.

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

1. Open **Reports > Data quality**. These are live read-only queries, not the
   custom report definitions farther down the page. Every member can review the
   queues; no record is changed automatically.
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

Authentication, workspace bootstrap, password setup, public lead submissions,
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

Monitor `open_crm_rate_limit_decisions_total{scope,outcome}`. The reference
rules alert on any sustained `error` decision and elevated `rejected` traffic.
For a store-error alert, first correlate `open_crm_database_up`, `/readyz`, and
database saturation/connection logs; restoring PostgreSQL restores the abuse
boundary without clearing live windows. For elevated rejections, identify the
bounded scope and compare route/status request rates. Do not query or export
individual client digests as user identifiers, and do not delete active buckets
to bypass a limit. If legitimate provider webhook traffic exceeds its budget,
validate provider source and signature evidence before changing the static
policy in code and rerunning the concurrency tests.

These application budgets coordinate replicas but are not a volumetric DDoS
boundary or a reputation system. An approved production edge/WAF plus lead-form
bot challenge and explicit consent remain required before promoting public lead
generation beyond foundation maturity.

## Deploy

Production deploy workflows are reusable workflows called only by
`.github/workflows/ci.yml` after backend, frontend, real-PostgreSQL browser, and
encrypted backup/restore jobs pass on `main`. A failed test, vet, format, lint,
audit, build, migration-integrity, browser, or recovery check prevents both
deploy jobs from starting. The backend deploy also verifies the public
`/healthz` and `/readyz` endpoints with bounded transport retries and exact
release matching; the frontend deploy verifies the published Pages URL. If the
GitHub-hosted runner cannot reach the origin through its Cloudflare region, the
backend job allows a four-minute public recovery window, emits a visible warning,
and repeats the same bounded public-hostname health and exact-release checks from
the production host. The window covers observed Cloudflare `522` recovery after
a healthy container replacement without accepting local-only readiness. A
reachable but wrong release never falls back and always fails the deployment.

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
`/readyz` reports the exact expected `X-Open-CRM-Release` header. Atomic state
and per-release manifests are retained under `var/deploy/`.

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
disposable CI acceptance test proves both failed-readiness recovery and a
manual rollback without reversing database migrations.

Check service state:

```sh
docker compose -f docker-compose.deploy.yml --env-file .env.production ps
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 api
docker compose -f docker-compose.deploy.yml --env-file .env.production logs --tail=200 migrate
cat var/deploy/last-deploy.json
cat var/deploy/current-release
cat var/deploy/previous-release
```

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

Open CRM runs calendar and task reminders, automatic mailbox sync, and sequence sends on
the tenant-scoped PostgreSQL queue. Claims use expiring leases and
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
Retention is allowlisted to the seven currently reviewed production job types:
`billing.reconcile`, `billing.usage.snapshot`, `calendar.reminder`,
`email_sequence.send`, `mailbox.sync`, `task.reminder`, and
`workspace.export.generate`. A new worker type retains full history until its
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
An owner/admin can **Approve & resume** the unchanged revision. Approval and
pause actions appear in Audit Trail.

Do not activate or resume a definition by updating `email_sequences` directly:
the API binds status, approver, approval time, and revision together, while the
worker independently verifies the same policy at its effect boundary.

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
- The production API container healthcheck uses `/healthz` so Docker can restart unhealthy API containers.
