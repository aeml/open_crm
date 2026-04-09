# Open CRM MVP Plan

> For Hermes: implement this as small vertical slices on `main`. Start with a modular monolith backend, a separate web app, Postgres in Docker Compose, typed contracts, TDD for core domain logic, and boring infrastructure that is easy to debug at 3am.

Goal: ship a clean, production-capable CRM MVP for managing organizations, users, contacts, companies, deals, notes, tasks, and activity history without painting the repo into a corner.

Architecture: use a simple monorepo with one backend service, one frontend app, and a small shared contracts package. Keep the backend as a modular monolith: clear domain modules, explicit service boundaries, single database, no fake microservices.

Tech stack:
- Package manager: pnpm workspaces
- Language: TypeScript everywhere
- Web: React + Vite + TanStack Router + TanStack Query + React Hook Form + Zod + Tailwind
- API: Fastify + Zod + Drizzle ORM + PostgreSQL
- Auth: server-side sessions stored in Postgres, HttpOnly cookies, Argon2id password hashing
- Database: PostgreSQL 16 via Docker Compose
- Testing: Vitest, Testing Library, Playwright for critical flows, supertest for API integration
- Tooling: ESLint, Prettier, Husky optional later, GitHub Actions CI

---

## 1. Product scope

This MVP should do a few things well:
- manage organizations/workspaces
- manage users and roles inside an organization
- manage contacts
- manage companies
- associate contacts to companies
- manage deals/opportunities with stages and values
- attach notes and tasks to contacts, companies, and deals
- maintain a visible activity timeline for writes
- support search, filtering, and simple dashboard counts

Do not overbuild v1.

Explicit non-goals for MVP:
- marketing automation
- email sync
- calendar sync
- custom workflow engine
- public API versioning guarantees
- mobile app
- event bus / Kafka / service mesh nonsense
- multi-region anything

---

## 2. Core engineering stance

### Build a modular monolith, not microservices

Microservices here would be cargo cult bullshit. The right move is:
- one API app
- one web app
- one Postgres database
- clear module boundaries inside the API
- clear shared contracts package between frontend and backend

That gets us:
- fast local dev
- simple deploys
- fewer moving parts
- easier tracing and debugging
- clean future extraction points if scale ever justifies it

### Use Postgres as the source of truth

Use Postgres for:
- auth/session data
- CRM entities
- audit/activity records
- dashboard aggregates

Do not add Redis on day one. Earn it later.

### Optimize for maintainability

Rules:
- each domain module owns its schema, service logic, and HTTP handlers
- route handlers stay thin
- business rules live in services
- SQL access stays in repositories/data access functions
- shared validation lives in `packages/contracts`
- no circular dependencies between modules
- no “utils” dumping ground; make helpers live near the domain they serve

---

## 3. Recommended repository layout

```text
open_crm/
  apps/
    api/
      src/
        app.ts
        server.ts
        config/
          env.ts
        db/
          client.ts
          schema/
            core.ts
            auth.ts
            crm.ts
          migrations/
        lib/
          errors.ts
          logger.ts
          pagination.ts
        modules/
          auth/
            auth.routes.ts
            auth.service.ts
            auth.repository.ts
            auth.types.ts
            auth.test.ts
          orgs/
            orgs.routes.ts
            orgs.service.ts
            orgs.repository.ts
            orgs.test.ts
          users/
          contacts/
          companies/
          deals/
          notes/
          tasks/
          activities/
          dashboard/
      tests/
        helpers/
        integration/
      package.json
      tsconfig.json
      vitest.config.ts
    web/
      src/
        main.tsx
        app/
          router.tsx
          providers.tsx
        components/
        features/
          auth/
          contacts/
          companies/
          deals/
          dashboard/
        hooks/
        lib/
          api-client.ts
          forms.ts
        routes/
          __root.tsx
          login.tsx
          dashboard.tsx
          contacts/
          companies/
          deals/
        styles/
      public/
      package.json
      vite.config.ts
  packages/
    contracts/
      src/
        auth.ts
        common.ts
        contacts.ts
        companies.ts
        deals.ts
        notes.ts
        tasks.ts
        index.ts
      package.json
      tsconfig.json
    config/
      eslint/
      typescript/
  infra/
    docker/
      postgres/
        init.sql
  .github/
    workflows/
      ci.yml
  docker-compose.yml
  .env.example
  .gitignore
  package.json
  pnpm-workspace.yaml
  README.md
  mvp.md
```

Notes:
- do not add Turborepo unless the repo actually starts hurting without it
- do not split `packages/contracts` into five packages before there is real pain
- keep schema files grouped logically, not one 2,000-line monster

---

## 4. Domain model for MVP

### Organizations and membership

Tables:
- `organizations`
- `users`
- `organization_memberships`
- `sessions`

Roles:
- `owner`
- `admin`
- `member`
- `viewer`

Important choice:
- schema should support many-to-many user/org membership
- UI can initially focus on one active organization per user

### CRM entities

Tables:
- `companies`
- `contacts`
- `contact_company_links`
- `deals`
- `deal_stages`
- `notes`
- `tasks`
- `activities`

Recommended minimum fields:

`organizations`
- `id`
- `name`
- `slug`
- `created_at`
- `updated_at`

`users`
- `id`
- `email`
- `password_hash`
- `first_name`
- `last_name`
- `created_at`
- `updated_at`

`organization_memberships`
- `id`
- `organization_id`
- `user_id`
- `role`
- `created_at`

`companies`
- `id`
- `organization_id`
- `name`
- `domain`
- `industry`
- `phone`
- `website`
- `status`
- `owner_user_id`
- `created_at`
- `updated_at`
- `archived_at`

`contacts`
- `id`
- `organization_id`
- `first_name`
- `last_name`
- `email`
- `phone`
- `job_title`
- `status`
- `owner_user_id`
- `created_at`
- `updated_at`
- `archived_at`

`contact_company_links`
- `id`
- `organization_id`
- `contact_id`
- `company_id`
- `relationship_title`
- `is_primary`

`deal_stages`
- `id`
- `organization_id`
- `name`
- `position`
- `is_closed`
- `is_won`

`deals`
- `id`
- `organization_id`
- `company_id` nullable
- `primary_contact_id` nullable
- `stage_id`
- `name`
- `status`
- `value_amount`
- `value_currency`
- `expected_close_date`
- `owner_user_id`
- `created_at`
- `updated_at`
- `archived_at`

`notes`
- `id`
- `organization_id`
- `entity_type`
- `entity_id`
- `body`
- `created_by_user_id`
- `created_at`
- `updated_at`

`tasks`
- `id`
- `organization_id`
- `entity_type`
- `entity_id`
- `title`
- `description`
- `status`
- `due_at`
- `assigned_to_user_id`
- `created_by_user_id`
- `created_at`
- `updated_at`
- `completed_at`

`activities`
- `id`
- `organization_id`
- `entity_type`
- `entity_id`
- `actor_user_id`
- `action`
- `summary`
- `metadata_json`
- `created_at`

### Design rules for the schema

- every tenant-owned table gets `organization_id`
- every primary entity gets `created_at` and `updated_at`
- use `archived_at` instead of hard delete for CRM records
- unique indexes should be scoped by organization when appropriate
- avoid JSON blobs for core business fields
- JSON is fine for flexible activity metadata only

---

## 5. Backend module design

Each module should follow the same shape:
- `*.routes.ts`: Fastify route registration and request/response wiring
- `*.service.ts`: business rules, orchestration, permissions
- `*.repository.ts`: Drizzle queries and persistence operations
- `*.types.ts`: local domain types if contracts package is not enough
- `*.test.ts`: focused tests for business rules

Example boundary:
- `contacts.service.ts` can call `contacts.repository.ts` and `activities.service.ts`
- it should not know raw SQL details
- route handlers should not compose SQL directly

Shared cross-cutting pieces:
- auth/session middleware
- org membership guard
- pagination helper
- typed error classes
- audit/activity writer

---

## 6. Frontend design

The frontend should feel boring in a good way.

Use:
- route-based feature organization
- TanStack Query for server state
- local component state for UI-only concerns
- React Hook Form + Zod for forms
- feature-local components before global shared components

Suggested page set for MVP:
- `/login`
- `/dashboard`
- `/contacts`
- `/contacts/:contactId`
- `/companies`
- `/companies/:companyId`
- `/deals`
- `/deals/:dealId`
- `/tasks`
- `/settings/users`

UI rules:
- list/detail pattern everywhere
- same filter/search bar pattern for contacts, companies, deals
- drawer or modal for quick create where it helps
- no giant global state store for CRUD forms
- no generated client SDK until the API actually stabilizes

---

## 7. Auth and authorization

MVP auth should be simple and robust:
- email + password login
- password hashes with Argon2id
- opaque server-side sessions in Postgres
- session cookie is HttpOnly, Secure in production, SameSite=Lax
- org membership checked on every tenant-scoped request

Authorization rules:
- `owner/admin`: full org management
- `member`: create/update CRM records
- `viewer`: read-only

Do not do OAuth first. It looks fancy and solves the wrong problem for an MVP.

---

## 8. API shape

Keep the API explicit and unsurprising.

Suggested route groups:
- `POST /auth/login`
- `POST /auth/logout`
- `GET /auth/me`
- `GET /organizations/current`
- `GET /users`
- `POST /users`
- `GET /contacts`
- `POST /contacts`
- `GET /contacts/:id`
- `PATCH /contacts/:id`
- `GET /companies`
- `POST /companies`
- `GET /companies/:id`
- `PATCH /companies/:id`
- `GET /deals`
- `POST /deals`
- `GET /deals/:id`
- `PATCH /deals/:id`
- `POST /notes`
- `GET /tasks`
- `POST /tasks`
- `PATCH /tasks/:id`
- `GET /dashboard/summary`

Request and response validation:
- define Zod contracts in `packages/contracts`
- reuse those contracts in API and UI
- return structured validation errors

Pagination:
- cursor pagination for lists if easy
- offset pagination is acceptable for MVP if kept consistent

Search:
- simple `ILIKE` search first
- add trigram or full-text indexes only when real data says it matters

---

## 9. Activity and audit trail

Every write to a core entity should create an activity row.

Minimum events:
- contact created
- contact updated
- company created
- company updated
- deal created
- deal stage changed
- note added
- task created
- task completed

Keep it simple:
- write activity in the same transaction where possible
- store a short human-readable summary plus structured metadata
- render timeline items from activity rows instead of trying to reconstruct history later

This buys a lot of product value cheaply.

---

## 10. Local development and containerization

Use Docker Compose for Postgres.

Initial `docker-compose.yml` should include:
- `postgres` service on port `5432`
- named volume for persistence
- healthcheck with `pg_isready`

Environment variables:
- `DATABASE_URL`
- `SESSION_COOKIE_SECRET`
- `APP_BASE_URL`
- `VITE_API_BASE_URL`
- `NODE_ENV`

Recommended local dev commands:
- `pnpm install`
- `docker compose up -d postgres`
- `pnpm db:migrate`
- `pnpm dev`

Do not containerize the app immediately unless deployment requires it. Containerize Postgres first. Keep app dev fast.

---

## 11. Testing strategy

Be selective and serious.

### Backend
- unit tests for business rules in services
- integration tests for route handlers + DB behavior
- test auth/session edge cases
- test tenant isolation hard

### Frontend
- component tests for forms, lists, and validation behavior
- a few route-level tests
- Playwright smoke tests for:
  - login
  - create contact
  - create company
  - create deal
  - add note
  - complete task

### High-value invariants
- users cannot read another org’s records
- archived records do not appear in active lists
- activity rows are created on writes
- deal stage changes persist and render correctly
- company/contact linking stays consistent

---

## 12. CI and quality gates

Initial CI should stay lean:
- install dependencies
- typecheck
- lint
- run backend tests
- run frontend tests
- optionally run Playwright later once the app exists

Do not build a Rube Goldberg CI pipeline. Fast and trustworthy beats elaborate.

---

## 13. MVP implementation slices

These are the right slices to ship in order.

### Slice 0: Repo bootstrap

Objective: create the workspace, local dev setup, shared config, and Postgres wiring.

Files to create:
- `package.json`
- `pnpm-workspace.yaml`
- `.gitignore`
- `.env.example`
- `docker-compose.yml`
- `README.md`
- `apps/api/package.json`
- `apps/web/package.json`
- `packages/contracts/package.json`
- base TypeScript and ESLint config files

Acceptance criteria:
- `pnpm install` works
- `docker compose up -d postgres` works
- both app packages boot with placeholder pages
- CI can run basic typecheck/lint

### Slice 1: Database and schema foundation

Objective: set up Drizzle, migrations, core tables, and seed data.

Files to create:
- `apps/api/src/db/client.ts`
- `apps/api/src/db/schema/core.ts`
- `apps/api/src/db/schema/auth.ts`
- `apps/api/src/db/schema/crm.ts`
- `apps/api/src/db/migrations/*`
- `apps/api/src/db/seed.ts`

Acceptance criteria:
- migrations create all foundation tables
- a seed command creates a default org, owner user, and default deal stages
- schema includes tenant boundaries and indexes

### Slice 2: Auth and current-session flow

Objective: make login/logout/me fully work.

Files to create:
- `apps/api/src/modules/auth/*`
- `apps/web/src/routes/login.tsx`
- `apps/web/src/features/auth/*`
- `packages/contracts/src/auth.ts`

Acceptance criteria:
- user can log in with seeded account
- session cookie persists across page refresh
- `/auth/me` returns current user + org membership
- unauthorized routes redirect to login

### Slice 3: Organizations and user management

Objective: basic org context and user listing/invite creation.

Files to create:
- `apps/api/src/modules/orgs/*`
- `apps/api/src/modules/users/*`
- `apps/web/src/routes/settings/users.tsx`
- `packages/contracts/src/common.ts`

Acceptance criteria:
- current org resolves on each request
- admin can list users in org
- admin can create another user account for the org
- role-based access is enforced

### Slice 4: Contacts vertical slice

Objective: contacts list, detail, create, update, archive, and activity entries.

Files to create:
- `apps/api/src/modules/contacts/*`
- `apps/web/src/routes/contacts/index.tsx`
- `apps/web/src/routes/contacts/$contactId.tsx`
- `apps/web/src/features/contacts/*`
- `packages/contracts/src/contacts.ts`

Acceptance criteria:
- searchable paginated contact list
- create/edit form with validation
- detail page shows notes/tasks/activity sections
- archive hides contact from default list

### Slice 5: Companies vertical slice

Objective: same quality bar as contacts, plus contact/company linking.

Files to create:
- `apps/api/src/modules/companies/*`
- `apps/web/src/routes/companies/index.tsx`
- `apps/web/src/routes/companies/$companyId.tsx`
- `apps/web/src/features/companies/*`
- `packages/contracts/src/companies.ts`

Acceptance criteria:
- company CRUD works
- contact can be linked to one or more companies
- company detail shows linked contacts and recent activity

### Slice 6: Deals and stages vertical slice

Objective: deal pipeline, stage changes, and summary metrics.

Files to create:
- `apps/api/src/modules/deals/*`
- `apps/web/src/routes/deals/index.tsx`
- `apps/web/src/routes/deals/$dealId.tsx`
- `apps/web/src/features/deals/*`
- `packages/contracts/src/deals.ts`

Acceptance criteria:
- deal CRUD works
- stage transitions work
- list view supports filtering by stage and owner
- dashboard can show open deals, won deals, and total pipeline value

### Slice 7: Notes, tasks, and activity timeline

Objective: shared engagement layer across CRM entities.

Files to create:
- `apps/api/src/modules/notes/*`
- `apps/api/src/modules/tasks/*`
- `apps/api/src/modules/activities/*`
- `apps/web/src/features/notes/*`
- `apps/web/src/features/tasks/*`
- `packages/contracts/src/notes.ts`
- `packages/contracts/src/tasks.ts`

Acceptance criteria:
- note creation works for contacts, companies, deals
- tasks can be assigned and completed
- entity detail pages show unified activity timeline

### Slice 8: Dashboard and operational polish

Objective: make the app feel coherent and usable.

Files to create:
- `apps/api/src/modules/dashboard/*`
- `apps/web/src/routes/dashboard.tsx`
- `apps/web/src/features/dashboard/*`

Acceptance criteria:
- dashboard shows counts and recent activity
- empty states are handled cleanly
- form UX is consistent
- app is usable without dev-only knowledge

### Slice 9: CI hardening and release readiness

Objective: make the repo safe to keep shipping from `main`.

Files to create:
- `.github/workflows/ci.yml`
- `apps/api/tests/integration/*`
- `apps/web/e2e/*`

Acceptance criteria:
- CI runs on push to main
- critical auth and CRUD flows are covered
- readme documents setup, dev flow, and env vars

---

## 14. Coding standards for implementation

### Backend
- keep route files thin
- return DTOs, not raw DB rows if shape drift is likely
- use transactions for multi-write operations
- centralize permission checks in services or dedicated guards
- prefer small pure functions for business rules

### Frontend
- colocate feature code near the route it serves
- keep API hooks near features, not in a giant global folder
- isolate reusable table/form components only after the second real use
- do not over-abstract CRUD screens before patterns settle

### Database
- add indexes intentionally
- document every non-obvious constraint
- never let org scoping be optional in repositories

---

## 15. What “good” looks like for this repo

A good v1 of `open_crm` should feel like this:
- you can clone it and be productive in minutes
- there is one obvious way to add a new domain module
- entity flows are consistent across contacts, companies, and deals
- tests catch tenant isolation and core CRUD regressions
- there is no mystery architecture
- adding a second engineer does not create chaos

That means choosing boring, explicit structure over flashy patterns.

---

## 16. API conventions and response envelope

Keep the API boring and consistent.

Success response shape:
```json
{
  "data": {},
  "meta": {
    "requestId": "req_123",
    "pagination": null
  }
}
```

List response shape:
```json
{
  "data": [],
  "meta": {
    "requestId": "req_123",
    "pagination": {
      "page": 1,
      "pageSize": 25,
      "total": 200,
      "hasNextPage": true
    }
  }
}
```

Error response shape:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": {
      "fieldErrors": {
        "email": ["Invalid email"]
      }
    }
  },
  "meta": {
    "requestId": "req_123"
  }
}
```

Rules:
- all handlers return one of these shapes
- every request gets a `requestId`
- avoid leaking raw database errors to clients
- use stable error codes even if messages change
- use `PATCH` for partial updates and `POST` for create actions
- normalize empty-string form input to `null` where that is the domain meaning

Recommended initial error codes:
- `VALIDATION_ERROR`
- `UNAUTHORIZED`
- `FORBIDDEN`
- `NOT_FOUND`
- `CONFLICT`
- `INVARIANT_VIOLATION`
- `INTERNAL_ERROR`

---

## 17. Permissions matrix

Do not hand-wave authorization. Write it down and code to it.

### Organization and users

| Capability | owner | admin | member | viewer |
|---|---|---|---|---|
| View org data | yes | yes | yes | yes |
| Manage org settings | yes | yes | no | no |
| List users | yes | yes | no | no |
| Create users | yes | yes | no | no |
| Change roles | yes | yes, except owner transfer | no | no |
| Delete/deactivate users | yes | yes | no | no |

### CRM entities

| Capability | owner | admin | member | viewer |
|---|---|---|---|---|
| View contacts/companies/deals | yes | yes | yes | yes |
| Create contacts/companies/deals | yes | yes | yes | no |
| Edit contacts/companies/deals | yes | yes | yes | no |
| Archive contacts/companies/deals | yes | yes | yes | no |
| Create notes/tasks | yes | yes | yes | no |
| Complete assigned tasks | yes | yes | yes | no |
| View dashboard | yes | yes | yes | yes |

Rules worth enforcing early:
- only org members can access org data
- cross-org IDs must resolve as `404`, not `403`, to avoid leaking record existence
- owner transfer is not an MVP feature unless it is deliberately implemented
- viewer cannot mutate anything, period

---

## 18. Data lifecycle and record rules

This is where a lot of CRUD apps become a swamp. Don’t let that happen.

### Create rules
- all writes must stamp actor user and organization context
- create endpoints must validate referenced foreign keys inside the same org
- defaults should be set server-side, not trusted from clients

### Update rules
- `updated_at` changes on every meaningful mutation
- partial updates should only touch supplied fields
- changing a deal stage must create an activity entry with old and new stage metadata

### Archive rules
- archive instead of delete for companies, contacts, and deals
- archived records stay queryable by explicit filter or direct detail route
- archived records do not appear in default active lists
- linking new notes/tasks to archived entities should be blocked unless there is a deliberate business case

### Hard delete policy
- hard delete only for sessions and maybe failed/incomplete seed data in development
- do not expose hard delete routes for core CRM entities in MVP

### Referential integrity rules
- do not allow cross-org references, ever
- use DB foreign keys where possible
- polymorphic note/task targets need explicit service-level validation because the DB cannot save you there

---

## 19. Query, indexing, and performance posture

Don’t optimize for fantasy scale, but don’t be sloppy either.

Initial indexes worth having:
- `organization_memberships (organization_id, user_id)` unique
- `users (email)` unique
- `companies (organization_id, name)`
- `companies (organization_id, domain)`
- `contacts (organization_id, email)`
- `contacts (organization_id, last_name, first_name)`
- `deals (organization_id, stage_id)`
- `deals (organization_id, owner_user_id)`
- `tasks (organization_id, status, due_at)`
- `activities (organization_id, entity_type, entity_id, created_at desc)`

Search posture:
- start with `ILIKE` on bounded fields
- search should always include `organization_id` and active/not-archived filters
- cap page size aggressively; 25 default, 100 max
- if search becomes hot, add trigram indexes on `companies.name`, `contacts.first_name`, `contacts.last_name`, and maybe `contacts.email`

Dashboard posture:
- compute simple aggregates live first
- if dashboard queries become visibly slow, add small read-model queries or materialized views later
- do not add background jobs just to feel sophisticated

---

## 20. Seed data, fixtures, and developer ergonomics

A repo like this lives or dies on setup friction.

Seed command should create:
- one organization: `Acme, Inc.`
- one owner user: `owner@acme.test`
- one admin user: `admin@acme.test`
- one member user: `member@acme.test`
- one viewer user: `viewer@acme.test`
- default password for local dev from env var or a clearly documented dev default
- default deal stages:
  - Lead
  - Qualified
  - Proposal
  - Negotiation
  - Closed Won
  - Closed Lost
- a small demo set of companies, contacts, deals, notes, and tasks

Seed rules:
- make it idempotent enough for local re-runs
- clearly separate dev seed from future production bootstrap
- never hide credentials; print the seeded login accounts in the seed script output for local dev

Developer experience baseline:
- one command to start Postgres
- one command to run migrations
- one command to seed
- one command to start both apps
- if a command needs five env vars and three manual steps, the plan is bad

---

## 21. Observability, logging, and operational sanity

Build just enough to debug real failures.

Backend logging:
- structured JSON logs in production
- pretty logs in local dev
- every request log includes `requestId`, `userId` if authenticated, `organizationId` if resolved, route, status code, and duration
- log auth failures, permission denials, and unexpected exceptions
- do not log passwords, session tokens, or raw cookies

Health endpoints:
- `GET /healthz` for simple process health
- `GET /readyz` for dependency readiness including DB connectivity

Operational checks for MVP:
- startup should fail loudly if required env vars are missing
- DB connection errors should be explicit
- migrations should run as an intentional command, not magically on every boot

---

## 22. Delivery plan by milestone

This repo should be built to a few concrete milestones, not as an endless blob of “in progress.”

### Milestone A: Foundation
Includes:
- Slice 0
- Slice 1
- minimal README
- CI for typecheck/lint/tests

Done means:
- a new engineer can clone, run Postgres, migrate, seed, and boot the repo
- the database is shaped correctly enough to support the first real features

### Milestone B: Usable internal alpha
Includes:
- Slice 2
- Slice 3
- Slice 4

Done means:
- org admin can log in
- manage users
- create and update contacts
- trust the app not to leak tenant data

### Milestone C: Core CRM loop
Includes:
- Slice 5
- Slice 6
- Slice 7

Done means:
- contacts, companies, and deals are connected
- notes/tasks/activity timeline make the app feel like a CRM instead of a spreadsheet with opinions

### Milestone D: MVP ready for broader use
Includes:
- Slice 8
- Slice 9
- docs cleanup
- basic operational runbook

Done means:
- the app is coherent
- the happy path is tested
- main is shippable without crossed fingers

---

## 23. Immediate next move

Start with Slice 0 and Slice 1 only.

Do not try to scaffold the whole product in one blast. That is how repos turn into dead weight.

The first real build sequence should be:
1. workspace bootstrap
2. Postgres + Drizzle + migrations
3. seeded owner login
4. contacts slice
5. companies slice
6. deals slice

Before writing the first line of feature code, lock these repo-level decisions:
- package manager is pnpm
- backend is Fastify + Drizzle + Postgres
- frontend is React + Vite + TanStack Router + TanStack Query
- auth is server-side sessions, not JWT cargo culting
- deploy shape is modular monolith + web app + Postgres

If those are clean, the rest gets much easier.

---

## 24. Detailed execution plan for Slice 0: repo bootstrap

This is the first slice because it makes every later slice cheaper.

### Outcome
By the end of Slice 0:
- the monorepo installs cleanly
- Postgres runs locally via Docker Compose
- API and web apps both boot
- contracts package builds and is consumed by both apps
- CI can lint and typecheck without heroics

### Task 0.1: Create the root workspace files

Files:
- create `package.json`
- create `pnpm-workspace.yaml`
- create `.gitignore`
- create `.npmrc`
- create `.env.example`
- create `README.md`

Rules:
- root `package.json` should expose top-level scripts only for orchestration
- do not dump app-specific scripts into the root unless they are cross-workspace wrappers
- pin Node and pnpm versions in `package.json` engines or `.nvmrc` later if needed

Recommended root scripts:
- `dev`
- `build`
- `typecheck`
- `lint`
- `test`
- `db:migrate`
- `db:generate`
- `db:seed`
- `format`

### Task 0.2: Create shared config packages

Files:
- create `packages/config/typescript/base.json`
- create `packages/config/typescript/node.json`
- create `packages/config/typescript/react.json`
- create `packages/config/eslint/base.cjs`
- create `packages/config/eslint/react.cjs`
- create `packages/config/package.json`

Rules:
- one place for tsconfig defaults
- one place for eslint defaults
- app packages extend shared config rather than copy-pasting local config sludge

### Task 0.3: Create the contracts package

Files:
- create `packages/contracts/package.json`
- create `packages/contracts/tsconfig.json`
- create `packages/contracts/src/index.ts`
- create `packages/contracts/src/common.ts`

Initial contract scope:
- shared pagination schema
- shared API envelope types
- shared ID/date/status schema helpers
- auth/session DTOs can land in Slice 2

Do not stuff domain logic in here. This package is for contracts, not “shared everything.”

### Task 0.4: Bootstrap the API app

Files:
- create `apps/api/package.json`
- create `apps/api/tsconfig.json`
- create `apps/api/src/server.ts`
- create `apps/api/src/app.ts`
- create `apps/api/src/config/env.ts`
- create `apps/api/src/lib/logger.ts`
- create `apps/api/src/routes/health.routes.ts`

Initial behavior:
- Fastify server starts on configurable port
- `/healthz` returns process health
- `/readyz` checks DB reachability once DB is wired in Slice 1
- env parsing is strict and fails startup loudly when broken

### Task 0.5: Bootstrap the web app

Files:
- create `apps/web/package.json`
- create `apps/web/tsconfig.json`
- create `apps/web/vite.config.ts`
- create `apps/web/index.html`
- create `apps/web/src/main.tsx`
- create `apps/web/src/app/router.tsx`
- create `apps/web/src/app/providers.tsx`
- create `apps/web/src/routes/__root.tsx`
- create `apps/web/src/routes/index.tsx`
- create `apps/web/src/styles/index.css`

Initial behavior:
- Vite dev server boots
- root route renders a placeholder shell
- TanStack Query provider is wired once
- routing is in place without pretending the whole app exists yet

### Task 0.6: Add Docker Compose for Postgres

Files:
- create `docker-compose.yml`
- create `infra/docker/postgres/init.sql` only if absolutely needed

Rules:
- Postgres is the only required container in local dev for now
- use a named volume
- expose `5432`
- add healthcheck
- do not build a local app container yet unless deployment forces it

### Task 0.7: Add baseline test and quality tooling

Files:
- create root and app-level config for Vitest and ESLint as needed
- create `apps/api/src/app.test.ts`
- create `apps/web/src/routes/index.test.tsx`
- create `.github/workflows/ci.yml`

Initial quality bar:
- API test proves server can boot and answer `/healthz`
- web test proves placeholder route renders
- CI runs install, lint, typecheck, and tests

### Task 0.8: Verify Slice 0

Commands:
```bash
pnpm install
docker compose up -d postgres
pnpm dev
pnpm lint
pnpm typecheck
pnpm test
```

Expected outcome:
- install succeeds
- Postgres container is healthy
- API and web app boot without manual patching
- lint/typecheck/tests pass

### Slice 0 exit criteria
- a fresh clone can boot in under 10 minutes
- there is one obvious place for backend code, one for frontend code, and one for contracts
- no fake abstractions were introduced “for later”

---

## 25. Detailed execution plan for Slice 1: database and schema foundation

This slice is where the repo stops being scaffolding and starts being a product.

### Outcome
By the end of Slice 1:
- Drizzle is wired to Postgres
- migrations create the core auth and CRM tables
- the repo can seed a usable demo org
- DB readiness works
- tests cover tenant scoping and schema assumptions where practical

### Task 1.1: Add DB dependencies and config

Files:
- modify `apps/api/package.json`
- create `apps/api/drizzle.config.ts`
- create `apps/api/src/db/client.ts`
- create `apps/api/src/db/migrate.ts`

Rules:
- DB client creation lives in one place
- use connection pooling appropriate for Node server usage
- env-driven connection string only; do not scatter DB config across files

### Task 1.2: Define shared table primitives

Files:
- create `apps/api/src/db/schema/_shared.ts`

Put the boring repeated columns here:
- `id`
- `organizationId`
- `createdAt`
- `updatedAt`
- `archivedAt`

But do not get too clever. Shared helpers should remove repetition, not obscure schema meaning.

### Task 1.3: Create auth/core schema

Files:
- create `apps/api/src/db/schema/core.ts`
- create `apps/api/src/db/schema/auth.ts`

Tables:
- `organizations`
- `users`
- `organization_memberships`
- `sessions`

Important constraints:
- unique user email
- unique org slug
- unique membership on `(organization_id, user_id)`
- session token hash or opaque session ID stored safely

### Task 1.4: Create CRM schema

Files:
- create `apps/api/src/db/schema/crm.ts`

Tables:
- `companies`
- `contacts`
- `contact_company_links`
- `deal_stages`
- `deals`
- `notes`
- `tasks`
- `activities`

Rules:
- every tenant-owned table carries `organization_id`
- foreign keys should be explicit where structurally possible
- indexes should reflect the query plan in this document, not guesswork from a blog post

### Task 1.5: Generate and review the initial migration

Files:
- create `apps/api/src/db/migrations/*`

Rules:
- generate migration from schema definitions
- inspect the SQL instead of blindly trusting the generator
- make sure index names and constraint names are readable
- if the generated SQL is ugly or wrong, fix it before shipping

### Task 1.6: Add DB readiness and startup integration

Files:
- modify `apps/api/src/app.ts`
- modify `apps/api/src/routes/health.routes.ts`

Behavior:
- `/healthz` should return process-up regardless of DB state
- `/readyz` should fail if DB cannot be reached
- startup logs should print enough to diagnose DB config problems fast

### Task 1.7: Create seed script

Files:
- create `apps/api/src/db/seed.ts`

Seed content:
- dev org and users from Section 20
- default deal stages
- small sample CRM dataset

Rules:
- seed should be re-runnable without exploding
- passwords must be hashed the same way production auth will hash them
- print seeded credentials and org info clearly at the end

### Task 1.8: Add repository smoke tests around DB assumptions

Files:
- create `apps/api/tests/integration/db-schema.test.ts`
- create `apps/api/tests/integration/tenant-isolation.test.ts`

Minimum tests:
- migration-applied schema can insert a user/org/membership tuple
- duplicate membership is rejected
- cross-org linked data is rejected by service validation or constraints where relevant
- archived rows stay out of default active queries once repositories exist

### Task 1.9: Verify Slice 1

Commands:
```bash
docker compose up -d postgres
pnpm db:generate
pnpm db:migrate
pnpm db:seed
pnpm test
```

Expected outcome:
- migration runs cleanly on empty DB
- seed completes cleanly
- seeded credentials are printed
- readiness endpoint passes with DB up
- tests pass

### Slice 1 exit criteria
- schema matches the domain model in this document
- local DB setup is boring and repeatable
- the repo is ready for auth implementation without backtracking on core table design

---

## 26. Non-negotiable implementation rules for the first build phase

These rules prevent stupid drift early.

- no branchy speculative architecture work before Slice 0 and 1 are real
- no generic repository base class unless it materially simplifies real code already written
- no codegen-heavy API client workflow for MVP
- no Redis, queues, cron workers, or websocket layer unless a real feature forces them
- no soft-deleting users or org memberships without a clearly defined behavior model
- no mixing tenant resolution logic into random route handlers; centralize it
- no frontend global store for CRUD data that TanStack Query already handles well
- no unbounded list endpoints
- no nullable chaos; if a field is optional, define why

---

## 27. What to document as the repo evolves

`mvp.md` is the product/engineering plan. Keep it honest.

As implementation begins, also add:
- `README.md` for setup and daily dev workflow
- `docs/adr/` for a few short architecture decisions if a decision becomes sticky
- `docs/runbooks/local-development.md` if setup gets non-trivial
- `docs/runbooks/release.md` once deployment exists

Good ADR candidates:
- why server-side sessions beat JWT for this repo
- why modular monolith beats service sprawl here
- why Postgres is the single source of truth for MVP

If a decision stops being true, update the docs. Dead docs are worse than no docs.
