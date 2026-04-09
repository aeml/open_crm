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

## 16. Immediate next move

Start with Slice 0 and Slice 1 only.

Do not try to scaffold the whole product in one blast. That is how repos turn into dead weight.

The first real build sequence should be:
1. workspace bootstrap
2. Postgres + Drizzle + migrations
3. seeded owner login
4. contacts slice
5. companies slice
6. deals slice

If those are clean, the rest gets much easier.
