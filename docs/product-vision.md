# Open CRM Product Vision

## Product thesis

Open CRM is a trustworthy, self-hostable revenue-and-client operations CRM for 5–50-person B2B service teams, with an optional managed SaaS offering.

It is not trying to reproduce every screen in an enterprise CRM suite. It should make one operating loop unusually coherent: turn an inbound lead into an owned relationship, follow it through communication and a commercial decision, then hand the resulting client to the team without losing context.

## Primary customer

The initial customer is a small B2B services company with a founder, sales or account lead, and a delivery team. It has outgrown spreadsheets and disconnected inboxes but does not have dedicated CRM administration or sales-operations staff.

The team needs:

- setup, migration, and daily operation without developer help;
- clear ownership and safe collaboration;
- portable customer data and a credible self-hosting path;
- email and follow-up centered on CRM records;
- a lightweight pipeline, quote, and client-handoff workflow;
- understandable automation and reporting;
- predictable pricing and tenant lifecycle behavior when using the managed service.

## Critical journey

A pilot customer must be able to complete this journey end to end:

1. Create and verify a workspace.
2. Invite a team, assign roles, and safely deactivate or reassign a member.
3. Import existing contacts and companies, map columns, resolve duplicates, and recover a bad import.
4. Capture or create a lead, attribute it, assign an owner, and schedule follow-up.
5. Send and receive record-linked email with appropriate privacy and suppression controls.
6. Progress an opportunity through an organization-defined pipeline.
7. Build, deliver, and sign a quote, then close the opportunity.
8. Hand the client and full history to the delivery/account team.
9. Automate a repeatable follow-up and inspect what ran or failed.
10. Build a useful report, export all tenant data, and recover from operational mistakes.

## Product principles

- Finish user outcomes before adding feature categories.
- Keep the modular monolith, PostgreSQL source of truth, explicit SQL, server-side sessions, and small dependency surface.
- Treat tenant isolation, permissions, portability, idempotency, and recovery as product behavior.
- Hide or label incomplete integrations; a fake provider is a development tool, not a shipped capability.
- Use progressive disclosure so breadth does not turn into navigation and settings clutter.
- Keep the self-hosted path operable without requiring the hosted billing stack.
- Let pilot evidence, not competitor checklists, determine expansion.

## Current distribution position

The repository is MIT licensed. During convergence, the working commercial model is managed hosting and support around the same open codebase; plan entitlements describe the hosted service and must not make self-hosted core data inaccessible. No proprietary/open-core source boundary has been approved. Any future licensing or source-availability change requires an explicit business and legal decision outside this engineering goal.

## Convergence scope

The convergence release includes:

- production trust and operability;
- the complete critical journey above;
- commercially real hosted signup, billing, metering, and tenant lifecycle;
- production-capable email/inbox/sequences, quotes/signature, workflow execution, and reporting.

AI, a help-desk suite, marketplace/custom objects, native mobile applications, real-time collaboration, and enterprise breadth are intentionally deferred until pilot evidence justifies them.

## Success evidence

The product is pilot-ready only when the critical journey passes against PostgreSQL in browser-level acceptance tests, external effects are idempotent and observable, backup restore is drilled, hosted lifecycle behavior is verified with provider sandboxes, and a pilot team can operate and export its workspace without developer intervention.
