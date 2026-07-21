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

Finalization does not send email, prove receipt/approval, expose a customer page, expire access, or collect a legal signature. Manual proposal tracking remains separate. Those are later Phase 4 outcomes.
