# Versioned Quotes

Open CRM has two deliberately different deal documents:

- **Current-data draft PDF** renders the live deal and saved line items at download time. It can change after any commercial edit and is not customer evidence.
- **Finalized quote version** stores an immutable commercial snapshot and the exact PDF bytes produced at finalization.

## Finalization contract

A writer supplies a recipient name/email, validity date, terms, and a 16–200
character `Idempotency-Key`. They may also bind the request to one exact active
template ID/revision and may request independent approval. At least one saved
line item is required. The validity date must be today through 366 days from
today.

One PostgreSQL transaction:

1. serializes the actor/key and locks the active tenant-scoped deal;
2. returns an existing version for the same normalized request, or rejects changed-payload key reuse with `409`;
3. allocates the next version for that deal;
4. snapshots organization, deal, account/contact labels, recipient, preparer,
   validity, terms, currency, line items, totals, the exact template reference,
   rendered delivery defaults, signature default, and approval requirement;
5. stores the exact PDF (maximum 2 MiB) and its SHA-256 digest; and
6. writes deal activity and a redacted audit event.

Raw idempotency keys are never stored. A finalized quote has no edit/delete endpoint. The authenticated download requires the session organization, deal, and quote IDs to match and returns `Cache-Control: private, no-store` plus `X-Open-CRM-Content-SHA256`.

## Preparation templates and independent approval contract

Owners/admins manage tenant-scoped quote preparation under **Settings > Quote
Templates**. A template has bounded terms, default validity, delivery
subject/message merge fields, a signature default, and an optional approval
requirement. Names are unique among active templates. Editing uses a required
current revision and creates the next revision; archiving removes the template
from future selection without changing a retained quote snapshot. Stale edits,
archived revisions, a foreign template, or finalization terms that differ from
the selected revision fail closed.

Workspace policy, the selected template, or the writer can require approval.
Enabling a required policy and finalizing a review-required quote both require
a different active owner/admin. Finalization creates one pending decision bound
to the requester, quote, and stored PDF SHA-256. The requester and quote creator
cannot decide it. A different active owner/admin reviews the retained PDF and
chooses one terminal `approved` or `rejected` result; rejection requires a
bounded note. Decision idempotency is digest-only: exact replay returns the
same evidence, while changed key/body reuse and attempts to alter a terminal
decision conflict.

Pending and rejected versions cannot be delivered. Delivery preparation and
the mailbox-provider claim both require retained approved evidence for the
exact PDF digest, so a provider effect cannot bypass review. Approval is an
internal release control only: it is not customer acceptance, signature,
accounting authorization, legal advice, or a deal close. Correct a rejected
quote in live deal data and finalize a new immutable version; never mutate the
rejected PDF or decision. Reissuing an expired version retains its exact source
template revision and creates a fresh approval request when review was required.

The pending queue, deal history, activity/audit records, notifications, and
aggregate status/oldest-pending metrics expose the workflow without tenant,
recipient, or document labels in telemetry. Portable workspace export retains
templates, workspace policy, quote template snapshots, and approval evidence
while excluding decision idempotency/request hashes.

## Operator reconciliation

1. Compare the UI quote number, recipient, total, validity, creator, and digest with the downloaded response header.
2. Re-download the same version after a live deal or line-item edit. Its digest and bytes must remain unchanged.
3. If finalization times out, retry the identical request with the identical idempotency key. Do not generate a new key until the outcome is known. A changed request requires a new key.
4. Do not update `deal_quotes`, `deal_quote_line_items`, PDF bytes, hashes, version numbers, activity, or audit evidence with ad hoc SQL. Correct live data and finalize a new version.
5. Portable workspace export contains quote snapshots, line snapshots, and PDF bytes while excluding internal idempotency/request hashes.

## Currency-disclosure contract

Every newly finalized or reissued quote retains the workspace base currency
used for reporting. When the quote currency matches it, the snapshot records an
identity rate. Otherwise, finalization locks and selects the newest
tenant/base/quote-currency rate whose `effective_date` is on or before the
document's UTC date. It stores that rate, effective date, source, and rounded
base-currency total with the immutable quote. A future rate or a rate belonging
to another workspace/base pair is never substituted.

If no valid effective rate exists, finalization or reissue returns `422
QUOTE_FX_RATE_REQUIRED` without creating a quote, activity, audit event, or
provider effect. Configure the rate under **Settings > Business Profile**, then
retry the identical request with the same idempotency key. Do not invent a
rate, backdate a quote, or patch the quote tables.

The retained PDF, staff version row, customer page, audit metadata, and portable
export expose the same disclosure. The base-currency number is explicitly a
reporting equivalent: the customer's amount due remains the quote currency and
total. Later edits to the workspace base currency or rate table never rewrite an
existing PDF or snapshot. Reissuing an expired version creates a new document
and therefore selects the rate effective for the replacement date while
preserving the source version's earlier disclosure. Versions created before
this control are labeled as legacy with no FX snapshot; their immutable PDFs
are not rewritten or backfilled with an estimate.

## Expiration and reissue contract

A quote is active through the end of its UTC `valid_until` date. After that
date every new delivery—review-only or signature-bearing—is rejected before an
intent or provider call. Staff see the version as **Expired** rather than a
generic finalized record.

A writer may deliberately reissue an expired version with a future validity
date and a 16–200 character `Idempotency-Key`. Reissue is allowed only while
the deal remains open, the source has no signed native evidence, no delivery is
`prepared`, `sending`, or `uncertain`, and no replacement already exists. One
transaction locks the deal, source quote, and signature state; copies the
source recipient, terms, commercial labels, line items, currency, totals, and
exact template/preparation snapshot;
renders a new PDF with current open-deal stage/close-date context and the
current preparer; stores a new version/digest; binds the source and replacement
with a tenant-and-deal constrained lineage; voids an expired pending native
signature request; creates a fresh independent review when the source required
approval; and writes activity/audit evidence. Exact concurrent retry
returns the same replacement. Changed key reuse, a second replacement,
cross-tenant lineage, signed evidence, or unresolved delivery fails closed.

The source version becomes **Replaced**, but its PDF, digest, deliveries,
receipt, signature/certificate evidence, and timestamps never change. A later
expired replacement may itself be reissued, forming an explicit one-to-one
chain. Commercial changes are not a reissue: edit the live deal and finalize a
deliberately new version instead. Portable workspace export retains the
lineage and all immutable versions while excluding replay hashes.

## Delivery and receipt contract

Finalization alone does not send anything. A writer may deliberately deliver one immutable version through their connected SMTP, Google, or Microsoft mailbox. Quote delivery requires both a valid `CREDENTIAL_ENCRYPTION_KEY` and the public browser `WEB_BASE_URL`.

Before the mailbox provider is called, Open CRM persists a
tenant/quote/actor-scoped intent with the exact sender and recipient, the
retained template subject/message defaults unless deliberately overridden, a
stable RFC `Message-ID`, a digest-only idempotency key, and a digest-only
customer access token. The customer link expires after the quote validity date
plus 30 days, with a minimum lifetime of 24 hours. Only one unresolved delivery
may exist per quote. A review-required quote must have retained approved
evidence for its exact PDF digest at both preparation and provider claim.

Delivery states have strict meanings:

- `prepared` and `sending` are durable provider-boundary states.
- `sent` means the connected provider accepted the message and Open CRM transactionally recorded the shared outbound email, activity, and audit evidence. It does not prove inbox placement.
- `failed` means the provider effect was definitely not accepted, a current
  suppression/mailbox/sender precondition prevented the provider call, or an
  operator marked it not sent. The API returns that durable state immediately
  and retains only a bounded, user-safe explanation rather than a raw provider
  or infrastructure error.
- `uncertain` means the connection or CRM finalization failed after the provider effect may have happened. Open CRM never automatically retries this state.

The original sender checks the exact RFC `Message-ID` in their Sent folder before choosing **Confirm in Sent folder**, **Retry after checking**, or **Mark not sent**. Owners/admins may confirm or reject another sender's uncertain delivery, but only the original sender may cause another provider attempt. Disabling/revoking a sender fails their prepared intent and quarantines an already claimed send as uncertain; preparation and claims lock and revalidate active membership first so no late intent can commit after deactivation. Startup and minutely recovery move `sending` claims older than five minutes to `uncertain`; aggregate metrics and alerts expose unavailable, stale, failed, and unresolved recovery without tenant, recipient, or token labels.

The public customer page and PDF are bearer-token resources with PostgreSQL-backed read/write budgets, `no-store`, and `no-referrer` responses. Preview access and PDF download counts are approximate because security scanners and reloads can follow links. **Confirm receipt** is an explicit, idempotent customer action with activity/audit evidence, but it is not approval, acceptance, consent to contract, or a legal signature. The raw access token and secure URL are not retained in the CRM email body or portable export.

Portable workspace export includes delivery status, sender/recipient snapshots, message copy, access/download timestamps and counts, explicit receipt time, and recovery error evidence. It excludes the access-token digest, idempotency/request hashes, RFC/provider correlation identifiers, and raw customer link.

## Native signature contract

A writer may select **Request electronic signature** while delivering a finalized version. An already-expired quote is rejected before an intent or provider effect exists; use the explicit reissue flow when its commercial content remains correct. The delivery transaction creates one native request bound by database constraints to that exact quote, deal, recipient, PDF filename, and delivery. Provider acceptance changes the request from `draft` to `sent`; a definite provider failure, a confirmed-not-sent resolution, or sender deactivation before the provider call voids it. An ambiguous send leaves the delivery recoverable and does not activate or duplicate the ceremony. Only one native `draft`, `sent`, or `signed` request may exist for a quote.

The recipient-specific email link is the authentication method. Signing is available only while both the public delivery link and the quote-validity day remain open. The recipient must type the expected name exactly after whitespace normalization and explicitly accept the immutable consent statement. Sign and decline require a 16–200 character idempotency key; only SHA-256 key/request digests are stored. An exact replay returns the original terminal result, changed reuse conflicts, and a new key cannot alter a terminal request. Staff can void an unsigned sent request but cannot create detached requests or mark one signed/declined. Historical manual tracking rows remain visible as read-only non-evidence.

A successful signature atomically retains typed name, expected recipient email, exact consent text/time, `recipient_email_link` authentication, quote PDF digest, signed time, and a generated certificate PDF with its own SHA-256. Customer and tenant-scoped staff downloads return the same retained certificate bytes. Signed/declined activity and audit evidence contains record IDs and digests, never the bearer token, IP address, or browser fingerprint. Shared PostgreSQL budgets protect preview/PDF/certificate reads and receipt/sign/decline writes; aggregate awaiting/expired/signed/declined/voided metrics expose no tenant, recipient, or token labels.

## Signed quote conversion contract

A public signature never guesses a pipeline or closes a deal. While the deal is still open, a writer deliberately chooses **Convert signed quote to won**, selects one current same-workspace won stage, and completes the required won close review. The deal must already have the account relationship required by the ordinary won transition.

One transaction locks the native signed request and deal, then binds the retained certificate to the selected stage and its immutable name snapshot, close reason and notes, closing actor/time, stage activity and event, matching task automation, and client handoff. A 16–200 character digest-only idempotency key makes an exact retry harmless. Changed reuse conflicts; a different key cannot convert terminal evidence again. Reopening the deal later clears its live close context through the normal stage control but intentionally retains the original quote-conversion evidence. Replaying the original conversion after that correction returns the current deal and does not silently re-close it.

Aggregate metrics distinguish signed requests awaiting staff conversion from
converted evidence. Portable workspace export includes consent, certificate,
conversion outcome, reissue lineage, quote preparation/approval, and FX
evidence but removes completion, conversion, finalization, reissue, and
approval replay hashes plus delivery secrets. Jurisdiction-specific legal and
accounting policy, approved live-mailbox evidence, and pilot validation remain
Phase 4 outcomes. The first-party ceremony is executable production-equivalent
behavior, not a claim that every agreement is legally enforceable in every
jurisdiction.
