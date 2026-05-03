# ADR-005: Modular monolith over microservices

Status: accepted
Date: 2024-01-01

## Context

Open CRM needs a backend architecture. The team is small (one engineer), the product is early-stage, and the deployment target is a single SSH-accessible host. The choice is between a modular monolith (one process, clear internal boundaries) and microservices (multiple independently deployed processes, typically with a message bus or API mesh).

## Options considered

**Microservices** — separate deployable units per domain (auth service, contacts service, deals service, etc.). Independent scaling, independent deploy cadence, polyglot-friendly. Requires service discovery, network calls between services, distributed tracing, a message bus for event propagation, and per-service CI/CD pipelines. Operational overhead is substantial even with orchestration. Debugging across service boundaries is significantly harder than debugging a single process.

**Modular monolith** — one process with clear package/module boundaries enforced by Go's import graph and code review. Each domain module owns its handler, service, repository, types, and tests. Cross-module calls are ordinary Go function calls; no serialization, no network, no retry. One deploy unit. One database. One log stream.

## Decision

Use a modular monolith: one Go process, one React web app, one Postgres database.

Reasons:
- The product is not at a scale or team size where microservices add more value than overhead.
- Module boundaries inside a Go monolith are enforced by the import graph; circular dependencies are compile errors.
- Each domain module (`internal/modules/contacts/`, `internal/modules/deals/`, etc.) follows the same shape: handler, service, repository, types, tests. Adding a new module is obvious.
- Cross-module interaction (e.g., contacts writing activity records) is a direct Go function call with full type safety and zero network latency.
- Deployment is a single binary + static frontend + Postgres. The operational surface is a `docker-compose.deploy.yml` and an `rsync`+`ssh` deploy script.
- If a module genuinely needs independent scaling or isolation in the future, it can be extracted. The module boundary already defines the interface. This extraction is far cheaper than retrofitting services onto a big ball of mud.
- Microservices for a single-developer CRM is cargo cult architecture.

## Consequences

- The entire application state is in one Postgres database; acceptable and desirable for a CRM.
- Module isolation is a code convention plus Go import rules, not a process boundary; reviews must enforce it.
- A future extraction of a hot module (e.g., a background job worker) into a separate process is possible by exporting the module's interface.
- Horizontal scaling means running multiple API processes behind a load balancer sharing Postgres; session state is in the database so no sticky sessions are needed.
