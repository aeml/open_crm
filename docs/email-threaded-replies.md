# Connected-mailbox threaded replies

## Product and sender policy

Open CRM replies only through the acting teammate's own connected mailbox. A
teammate may collaborate on shared mail, but Open CRM never impersonates the
mailbox that originally received it. Private source mail remains private;
shared source mail produces a shared reply. Owners and admins can inspect and
resolve uncertain shared/private reply outcomes, but only the original sender
can perform a retry through that sender's mailbox.

The Mailbox and Team Inbox show one chronological conversation using normalized
RFC `Message-ID`, `In-Reply-To`, and `References` headers plus provider thread
identifiers when available. New inbound mail inherits a root only from the same
workspace and mailbox. Missing or ambiguous correlation does not authorize a
reply or cross a mailbox/tenant boundary. Replies deliberately leave engagement
tracking off.

## External-effect boundary

`POST /api/email-threads/{messageID}/reply` requires a 16–200 character
`Idempotency-Key`. Before contacting SMTP, Gmail, or Microsoft, Open CRM stores
an immutable request hash, sender/recipient/content snapshot, stable RFC message
ID, source/root, privacy, and actor. The state machine is:

- `prepared`: durable intent exists; no provider attempt is claimed.
- `sending`: one sender has claimed the provider attempt.
- `accepted`: the provider returned acceptance and the outbound conversation
  message committed in the same database transaction as finalization.
- `failed`: a definite pre-acceptance failure or an operator-confirmed not-sent
  outcome; replay of the same key cannot silently send again.
- `uncertain`: the provider may have accepted the message, or the process was
  interrupted after claiming it. No automatic retry is allowed.

The API rechecks current sender identity and recipient suppression immediately
after claiming and before the provider call. A one-minute scheduler moves send
claims older than five minutes to `uncertain` without calling a provider. The
sender must check the Sent folder and explicitly retry or mark not sent; an
owner/admin may confirm sent or mark not sent. Every uncertain resolution is
audited without retaining message content in the audit event.

Only one `prepared`, `sending`, or `uncertain` reply per actor and conversation
may exist. The database enforces this across processes, and the composer stays
closed until that actor resolves the existing outcome, so changing a browser
request key cannot bypass duplicate-risk confirmation.

Portable workspace bundles include shared reply content and lifecycle state but
remove request-key hashes and RFC/provider correlation identifiers. Private
reply intents remain excluded with private mailbox messages.

## Evidence and remaining promotion work

Handler/UI/MIME tests cover roles, private/shared access, idempotency headers,
suppression, exact reply headers, uncertainty, and explicit recovery. A
disposable-PostgreSQL acceptance test covers tenant isolation, direct-insert
thread roots, service-boundary roles, replay conflicts, claim/finalize,
interruption recovery, operator permissions, and inbound correlation. Protected
aggregate metrics and alerts cover scheduler health, stale claims, and
unresolved outcomes.

This remains below `production-capable` until approved Gmail, Microsoft, and
SMTP sandbox runs retain raw delivered header/thread evidence, forced ambiguous
outcome recovery evidence, suppression behavior, and pilot review of the
send-as/privacy wording. Provider acceptance is not proof of final delivery;
the separate customer-email feedback contract still governs later bounce and
complaint evidence.
