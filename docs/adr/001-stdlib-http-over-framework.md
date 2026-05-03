# ADR-001: Go stdlib net/http over third-party framework

Status: accepted
Date: 2024-01-01

## Context

Open CRM needs an HTTP API layer. The Go ecosystem has several well-known HTTP frameworks (Gin, Echo, Fiber, Chi) that provide routing, middleware chains, and response helpers. The Go standard library has had a capable `net/http` and `ServeMux` since 1.0, with pattern-based routing (including method and path parameter support) added in Go 1.22.

The project started on Go 1.23.

## Options considered

**Gin / Echo / Fiber** — popular frameworks with rich middleware ecosystems, convenient context types, and large communities. Add a compile dependency, a custom handler signature, and a non-stdlib request context model. Framework-specific abstractions tend to become load-bearing.

**Chi** — lightweight router that stays on `net/http` semantics. Adds a dependency but avoids the full framework surface.

**Go stdlib `net/http` + `ServeMux`** — no external dependency. Handler signature is the standard `http.Handler`. Pattern routing covers method-prefixed and wildcard patterns. Response helpers, middleware, and error types are written once in `internal/platform/web/` and owned entirely.

## Decision

Use Go stdlib `net/http` with `ServeMux` and hand-written helpers in `internal/platform/web/`.

Reasons:
- No extra dependency to audit, update, or explain.
- Handler signature stays `http.Handler`; no framework lock-in.
- Go 1.22+ `ServeMux` supports method-based and path-parameter routing that covers every route in this application.
- Any middleware written against `http.Handler` composes cleanly.
- The full routing table is visible in one file (`internal/app/`); no magic registration.
- Helpers (`RespondJSON`, `DecodeJSON`, `RequestID`, etc.) are short, tested, and owned.

## Consequences

- New engineers familiar only with Gin/Echo need a few minutes to adjust but find no surprises.
- Adding any stdlib-compatible middleware (rate limiting, CORS, etc.) works without adapters.
- If a genuinely complex routing need appears (e.g., OpenAPI middleware), a thin router wrapper can be introduced without replacing every handler.
- There is no built-in request validation or binding framework; validation lives in service layer with explicit checked structs.
