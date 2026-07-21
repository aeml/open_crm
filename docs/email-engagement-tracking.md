# Email Engagement Tracking

This document is the product and data contract for approximate engagement
signals on one-to-one customer email sent from a CRM record. It does not apply
to email sequences or unfinished marketing-campaign foundations.

## Sender choice and scope

Tracking is off by default on every contact, company, and deal email composer.
It is enabled for one send only when the sender checks the option confirming
that their organization is authorized to collect open and link-click signals.
Templates and snippets do not enable it, and the form resets to off after a
successful send. Open CRM does not infer recipient consent or claim that the
sender confirmation satisfies any particular law, contract, or jurisdiction.
Operators must establish the appropriate policy for their recipients before
using the option.

## Data and meaning

An opted-in send stores a random 256-bit open token, a random 256-bit token and
validated HTTP(S) destination for each rewritten link, the confirming user and
time, and a collection expiry 90 days later. During that window Open CRM stores
only aggregate open and click counts plus first/last observation timestamps. It
does not persist the requesting client address, user agent, or referrer for
these endpoints.

The observations are directional hints, not proof a person read or acted on a
message. Mail security scanners can follow links, privacy proxies can fetch
pixels, clients can suppress images, and forwarding can attribute another
person's activity to the original message. The UI describes the signals as
approximate and never equates provider acceptance, an open, or a click with
delivery, identity, or consent.

Avoid placing credentials or other secrets in destination URLs. A destination
is retained with its opaque click token for as long as the email-message record
exists so an already-delivered link continues to work after collection ends.
Portable workspace exports remove internal tracking tokens but may include the
business URL as part of the tenant's email record.

## Expiry and deletion

At the exact expiry, the API stops exposing observations and public endpoints
stop incrementing them. The open endpoint continues returning the same neutral
pixel so token state is not disclosed. The click endpoint continues its
validated redirect without recording another event.

A bounded lifecycle pass runs immediately on API startup and hourly, scrubbing
the open token, message and per-link counts, and all first/last observation
timestamps. It retains only the click mapping required for old links to work and
records when the scrub completed. Passes are idempotent, retryable, safe across
multiple API instances, observable through protected aggregate metrics, and
covered by retention alerts. Migration 91 expires all pre-policy tracked
messages immediately because the old schema has no explicit sender
acknowledgement; the scheduler then scrubs their observations.

Public open and click endpoints also use separate shared PostgreSQL-backed
per-client limits of 300 requests per minute, fail closed when that shared store
is unavailable, return no-referrer/no-index responses, and never accept a
browser-supplied redirect destination. These controls protect application
capacity but are not a substitute for an approved edge/WAF against volumetric
traffic.
