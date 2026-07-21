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

## Remaining signature boundary

Manual proposal tracking remains separate from immutable quote delivery. It does not collect consent or a legal signature. Reusable templates/terms, approval, quote-level FX disclosure, expiration workflow, signer identity/access, an actual signing ceremony or provider, signed idempotent webhooks, an audit certificate, and closed-deal conversion remain later Phase 4 outcomes.
