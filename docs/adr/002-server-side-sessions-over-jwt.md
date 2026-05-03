# ADR-002: Server-side sessions over JWT

Status: accepted
Date: 2024-01-01

## Context

Open CRM is a server-rendered-data web application accessed via a browser. Authentication state needs to persist across page loads. Two dominant patterns exist: server-side session tokens stored in a database, and self-contained JWTs sent as Bearer tokens or cookies.

The application has one Postgres instance that is already the source of truth for all other state.

## Options considered

**JWT (JSON Web Tokens)** — stateless tokens signed with a secret or keypair. No database read required to validate on each request. Commonly stored in localStorage or as a cookie. Requires token rotation, refresh token flows, revocation lists if early invalidation is needed, and careful storage decisions. Logout is complex: the token remains valid until expiry unless a blocklist is maintained, which reintroduces server-side state.

**Opaque server-side sessions in Postgres** — a random token is issued at login. The server stores a hashed version in a `sessions` table. Each request looks up the session by token hash. Session state (user ID, organization ID, expiry, last seen) lives in the database. Logout is a single delete. Invalidating all sessions for a user (on role change, disable, password reset) is a single query.

## Decision

Use opaque server-side sessions stored in Postgres, delivered via `HttpOnly; Secure; SameSite=Lax` cookies.

Reasons:
- Logout and session invalidation are simple and immediate: delete the row.
- Session state (last seen, organization context) is queryable for admin and audit purposes.
- No token rotation or refresh endpoint needed for MVP.
- `SameSite=Lax` plus CSRF double-submit for state-changing requests is straightforward and well understood.
- The application already has Postgres; adding a session table adds no new infrastructure dependency.
- JWT statelessness is valuable for multi-region or multi-instance deployments. This application deploys to a single host. The benefit does not apply.
- JWT revocation (logout, role change, disable user) requires a server-side blocklist anyway, which erases the statelessness argument for most CRM use cases.

## Consequences

- Every authenticated request does one Postgres session lookup; acceptable at this scale.
- Multi-instance deployment is possible with a shared Postgres session table; no sticky sessions required.
- If a distributed cache or stateless API later becomes genuinely necessary, the session interface can be wrapped.
- Session cleanup (expired rows) runs as a periodic maintenance query; documented in the operations runbook.
