# ADR-003: Plain SQL over ORM

Status: accepted
Date: 2024-01-01

## Context

Open CRM's backend stores all state in Postgres. Database access can be written as plain SQL (with a driver or connection pool), via a query builder (sqlx, squirrel), or via a full ORM (GORM, Ent). Each level of abstraction trades explicitness for convenience.

## Options considered

**GORM** — the most popular Go ORM. Provides struct-driven migrations, associations, hooks, and a chainable query API. Hides SQL behind a method DSL; generated queries are not always obvious. Complex queries often require dropping back to raw SQL anyway. Adds significant package weight and a non-trivial learning curve for its query model.

**Ent** — code-generation-based ORM from Meta. Schema-first with strong typing. Generates substantial boilerplate. Well-suited for graph-like entity relationships. Adds a code generation step and a new way of expressing queries that diverges from SQL thinking.

**sqlx** — thin wrapper over `database/sql` that adds struct scanning. SQL is still written by hand; sqlx handles destination mapping. Reasonable middle ground but adds a dependency.

**pgx/v5 with plain SQL** — `pgx` is the standard high-performance Postgres driver and pool for Go. All SQL is written explicitly. Repository functions are ordinary Go functions that accept a pool and return typed results. `pgx.CollectRows` and `pgx.RowToStructByName` handle scanning. Queries are readable, reviewable, and directly runnable in psql.

## Decision

Use `pgx/v5` with plain SQL. No ORM or query builder.

Reasons:
- Every SQL statement is visible in the source file where it executes; no hidden query generation.
- Postgres-specific features (ON CONFLICT, RETURNING, window functions, ILIKE, partial indexes) are expressed directly without fighting an ORM abstraction.
- Reviewing query behavior during code review is straightforward.
- The CRM data model is relational and well-normalized; ORM association magic is not needed.
- No code generation step in the build pipeline.
- `pgx/v5` handles connection pooling, prepared statements, and type mapping well on its own.
- Queries that change as features evolve are edited in one place with no cascading DSL changes.

## Consequences

- Repository functions are slightly more verbose than ORM equivalents; this is acceptable.
- Multi-step writes use explicit `pgx.Tx` transactions, which are clear and testable.
- If a query performance issue appears, the query is immediately visible and tunable.
- Schema migrations are hand-written SQL files, reviewed as code, and tracked in `schema_migrations`.
- Adding a new module means writing a small number of explicit SQL queries, not learning a new DSL.
