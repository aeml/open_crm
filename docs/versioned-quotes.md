# Versioned Quotes

Open CRM has two deliberately different deal documents:

- **Current-data draft PDF** renders the live deal and saved line items at download time. It can change after any commercial edit and is not customer evidence.
- **Finalized quote version** stores an immutable commercial snapshot and the exact PDF bytes produced at finalization.

## Finalization contract

A writer supplies a recipient name/email, validity date, terms, and a 16–200 character `Idempotency-Key`. At least one saved line item is required. The validity date must be today through 366 days from today.

One PostgreSQL transaction:

1. serializes the actor/key and locks the active tenant-scoped deal;
2. returns an existing version for the same normalized request, or rejects changed-payload key reuse with `409`;
3. allocates the next version for that deal;
4. snapshots organization, deal, account/contact labels, recipient, preparer, validity, terms, currency, line items, and totals;
5. stores the exact PDF (maximum 2 MiB) and its SHA-256 digest; and
6. writes deal activity and a redacted audit event.

Raw idempotency keys are never stored. A finalized quote has no edit/delete endpoint. The authenticated download requires the session organization, deal, and quote IDs to match and returns `Cache-Control: private, no-store` plus `X-Open-CRM-Content-SHA256`.

## Operator reconciliation

1. Compare the UI quote number, recipient, total, validity, creator, and digest with the downloaded response header.
2. Re-download the same version after a live deal or line-item edit. Its digest and bytes must remain unchanged.
3. If finalization times out, retry the identical request with the identical idempotency key. Do not generate a new key until the outcome is known. A changed request requires a new key.
4. Do not update `deal_quotes`, `deal_quote_line_items`, PDF bytes, hashes, version numbers, activity, or audit evidence with ad hoc SQL. Correct live data and finalize a new version.
5. Portable workspace export contains quote snapshots, line snapshots, and PDF bytes while excluding internal idempotency/request hashes.

## Delivery and receipt contract

Finalization alone does not send anything. A writer may deliberately deliver one immutable version through their connected SMTP, Google, or Microsoft mailbox. Quote delivery requires both a valid `CREDENTIAL_ENCRYPTION_KEY` and the public browser `WEB_BASE_URL`.

Before the mailbox provider is called, Open CRM persists a tenant/quote/actor-scoped intent with the exact sender and recipient, subject/message snapshot, a stable RFC `Message-ID`, a digest-only idempotency key, and a digest-only customer access token. The customer link expires after the quote validity date plus 30 days, with a minimum lifetime of 24 hours. Only one unresolved delivery may exist per quote.

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

A writer may select **Request electronic signature** while delivering a finalized version. An already-expired quote is rejected before an intent or provider effect exists; finalize a new version rather than sending an unusable signing link. The delivery transaction creates one native request bound by database constraints to that exact quote, deal, recipient, PDF filename, and delivery. Provider acceptance changes the request from `draft` to `sent`; a definite provider failure, a confirmed-not-sent resolution, or sender deactivation before the provider call voids it. An ambiguous send leaves the delivery recoverable and does not activate or duplicate the ceremony. Only one native `draft`, `sent`, or `signed` request may exist for a quote.

The recipient-specific email link is the authentication method. Signing is available only while both the public delivery link and the quote-validity day remain open. The recipient must type the expected name exactly after whitespace normalization and explicitly accept the immutable consent statement. Sign and decline require a 16–200 character idempotency key; only SHA-256 key/request digests are stored. An exact replay returns the original terminal result, changed reuse conflicts, and a new key cannot alter a terminal request. Staff can void an unsigned sent request but cannot create detached requests or mark one signed/declined. Historical manual tracking rows remain visible as read-only non-evidence.

A successful signature atomically retains typed name, expected recipient email, exact consent text/time, `recipient_email_link` authentication, quote PDF digest, signed time, and a generated certificate PDF with its own SHA-256. Customer and tenant-scoped staff downloads return the same retained certificate bytes. Signed/declined activity and audit evidence contains record IDs and digests, never the bearer token, IP address, or browser fingerprint. Shared PostgreSQL budgets protect preview/PDF/certificate reads and receipt/sign/decline writes; aggregate awaiting/expired/signed/declined/voided metrics expose no tenant, recipient, or token labels.

Portable workspace export includes consent and certificate evidence but removes completion replay hashes and delivery secrets. Reusable terms/templates, approval, quote-level FX disclosure, active expiration/reissue workflow, automatic signed-quote-to-won conversion, jurisdiction-specific legal policy, approved live-mailbox evidence, and pilot validation remain Phase 4 outcomes. The first-party ceremony is executable production-equivalent behavior, not a claim that every agreement is legally enforceable in every jurisdiction.
