# Architecture Decision Records

Short records of sticky decisions made while building Open CRM. Each ADR captures context, the options considered, the choice made, and consequences. If a decision stops being true, the ADR gets a status update and a new ADR explains the change.

## Format

```
# ADR-NNN: Title
Status: accepted | superseded by ADR-NNN | deprecated
Date: YYYY-MM-DD

## Context
What was the situation and why did it need a decision.

## Options considered
What alternatives were on the table.

## Decision
What was chosen and why.

## Consequences
What becomes easier or harder as a result.
```

## Index

| # | Title | Status |
|---|-------|--------|
| [001](001-stdlib-http-over-framework.md) | Go stdlib net/http over third-party framework | accepted |
| [002](002-server-side-sessions-over-jwt.md) | Server-side sessions over JWT | accepted |
| [003](003-plain-sql-over-orm.md) | Plain SQL over ORM | accepted |
| [004](004-javascript-over-typescript.md) | JavaScript over TypeScript on the frontend | accepted |
| [005](005-modular-monolith-over-services.md) | Modular monolith over microservices | accepted |
