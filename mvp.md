# Open CRM MVP Plan

> For Hermes: build this as small vertical slices on `main`. Use a Go modular monolith for the backend, a JavaScript React app for the frontend, Postgres in Docker Compose, plain SQL where it stays readable, and a UI that feels clean without dragging in a circus of dependencies.

Goal: ship a production-capable CRM MVP for managing organizations, users, contacts, companies, deals, notes, tasks, and activity history without creating a repo that needs a support group.

Architecture: one Go API service, one JavaScript web app, one Postgres database. Keep the backend modular inside one process. Keep the frontend simple, fast, and visually polished with hand-rolled design primitives instead of a massive component stack.

Tech stack:
- Backend language: Go 1.23+
- Backend HTTP: Go stdlib `net/http` with `ServeMux`
- Backend DB access: `pgx/v5` with explicit SQL and small repositories
- Auth: server-side sessions stored in Postgres, signed opaque session cookie, Argon2id password hashing
- Database: PostgreSQL 16 via Docker Compose
- Frontend language: JavaScript
- Frontend UI: React + Vite + React Router + plain CSS
- Frontend data access: `fetch` with a thin API client, no heavy state library
- Testing: Go `testing` + `httptest`, Vitest + Testing Library, Playwright only for a few critical flows later
- Tooling: `go`, `npm`, Docker Compose, GitHub Actions, `make` for top-level orchestration

Guiding principle: minimal dependencies does not mean ugly. It means every dependency has to earn its existence.

---

## 1. Product scope

This MVP should do a few things well:
- manage organizations and memberships
- manage users and roles inside an organization
- manage contacts
- manage companies
- associate contacts to companies
- manage deals/opportunities with stages and values
- attach notes and tasks to contacts, companies, and deals
- maintain a visible activity timeline for writes
- support search, filtering, and basic dashboard counts

Do not overbuild v1.

Explicit non-goals for MVP:
- marketing automation
- email sync
- calendar sync
- custom workflow engine
- mobile app
- event bus / queue sprawl
- microservices
- public API versioning guarantees
- multi-region anything
- real-time collaboration features

> **Note (post-MVP strategic change):** These remain non-goals for the MVP, but several were deliberately reclassified as competitive requirements in Part II of the roadmap. Open CRM's direction has expanded toward a full-featured, multi-tenant SaaS product. See `docs/professionalization-roadmap.md` → "Strategic Direction Change" and "Part II — Competitive SaaS Platform" for how email sync, calendar sync, marketing automation, the workflow engine, mobile, and real-time collaboration are now planned. This document remains the record of the original MVP scope.

---

## 2. Core engineering stance

### Build a modular monolith, not fake distributed systems

Microservices here would be cargo cult bullshit. The right move is:
- one Go API app
- one JavaScript web app
- one Postgres database
- clear module boundaries inside the backend
- explicit APIs between frontend and backend

That gets us:
- fast local dev
- simple deploys
- fewer moving parts
- easier debugging
- cleaner ownership boundaries
- realistic future extraction points if they are ever actually needed

### Use boring, explicit primitives

Backend:
- stdlib HTTP server
- explicit routes
- explicit services
- explicit repositories
- explicit SQL

Frontend:
- React for composable UI
- React Router for navigation
- plain CSS with tokens and reusable UI primitives
- local component state plus a tiny amount of React context where it really helps
- `fetch` wrappers, not a whole data access religion

### Use Postgres as the source of truth

Use Postgres for:
- auth/session data
- CRM entities
- audit/activity records
- dashboard aggregates

Do not add Redis on day one. Earn it later.

### Optimize for maintainability

Rules:
- each domain module owns its routes, service logic, repository logic, and tests
- HTTP handlers stay thin
- business rules live in services
- SQL access lives in repositories
- no circular dependencies between modules
- no generic base repository nonsense unless it materially simplifies real code already written
- no `utils` junk drawer; helpers live near the domain they help
- no ORM if plain SQL is still clean and obvious

---

## 3. Recommended repository layout

```text
open_crm/
  apps/
    api/
      cmd/
        open_crm_api/
          main.go
        migrate/
          main.go
        seed/
          main.go
      internal/
        config/
          env.go
        db/
          postgres.go
          migrations/
            001_initial_schema.sql
            002_seed_helpers.sql
        platform/
          logger/
            logger.go
          auth/
            passwords.go
            sessions.go
          web/
            json.go
            errors.go
            middleware.go
        modules/
          auth/
            handler.go
            service.go
            repository.go
            types.go
            auth_test.go
          orgs/
            handler.go
            service.go
            repository.go
            orgs_test.go
          users/
          contacts/
          companies/
          deals/
          notes/
          tasks/
          activities/
          dashboard/
      test/
        integration/
        fixtures/
      go.mod
      go.sum
      Makefile
    web/
      src/
        main.jsx
        app/
          router.jsx
          providers.jsx
          shell.jsx
        components/
          ui/
            button.jsx
            card.jsx
            field.jsx
            modal.jsx
            table.jsx
          layout/
            app_header.jsx
            side_nav.jsx
            page_header.jsx
        features/
          auth/
          contacts/
          companies/
          deals/
          tasks/
          dashboard/
        lib/
          api.js
          date.js
          forms.js
          validation.js
        routes/
          root.jsx
          login.jsx
          dashboard.jsx
          contacts/
            index.jsx
            detail.jsx
          companies/
            index.jsx
            detail.jsx
          deals/
            index.jsx
            detail.jsx
          tasks/
            index.jsx
          settings/
            users.jsx
        styles/
          tokens.css
          base.css
          layout.css
          components.css
          utilities.css
      public/
      package.json
      vite.config.js
      vitest.config.js
  docs/
    adr/
    runbooks/
  .github/
    workflows/
      ci.yml
  docker-compose.yml
  .env.example
  .gitignore
  Makefile
  README.md
  mvp.md
```

Notes:
- no npm workspace is required at the root unless the frontend eventually grows more packages
- no shared contracts package until there is real pain that justifies it
- keep the backend module layout consistent so adding a new domain module is obvious
- keep CSS organized by concern, not as a 7,000-line blob

---

## 4. Dependency budget

Minimal dependencies is a hard requirement here, not a suggestion.

### Backend dependency budget

Use only what earns its keep:
- `github.com/jackc/pgx/v5` for Postgres access and pooling
- `golang.org/x/crypto` for Argon2id
- maybe one tiny assertion helper for tests if it truly improves readability, but default to stdlib testing first

Do not add for MVP:
- Gin
- Echo
- Fiber
- GORM
- sqlx unless raw `pgx` becomes painful
- dependency injection frameworks
- validation frameworks unless hand-written validation actually becomes unmanageable
- background job libraries

The stdlib is good now. Use it.

### Frontend dependency budget

Use only what materially improves UX or developer speed:
- `react`
- `react-dom`
- `react-router-dom`
- `vite`
- `vitest`
- `@testing-library/react`
- `@testing-library/jest-dom`
- maybe `@testing-library/user-event`

Do not add for MVP:
- Tailwind
- component libraries
- state libraries like Redux, Zustand, MobX
- TanStack Query
- React Hook Form
- schema validation libraries on the client
- date libraries unless native date handling proves unbearable
- icon packs with 2,000 icons when 12 inline SVGs will do

Nice UI does not require dependency obesity.

---

## 5. Domain model for MVP

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
- `deal_stages`
- `deals`
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

`sessions`
- `id`
- `user_id`
- `organization_id`
- `token_hash`
- `expires_at`
- `created_at`
- `last_seen_at`

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
- unique indexes should be scoped by organization where appropriate
- avoid JSON blobs for core business fields
- JSON is fine for flexible activity metadata only
- use explicit SQL migrations checked into git

---

## 6. Backend module design

Each backend module should follow the same shape:
- `handler.go`: route registration and HTTP request/response wiring
- `service.go`: business rules, orchestration, permissions
- `repository.go`: SQL queries and persistence operations
- `types.go`: request/response and internal domain structs
- `*_test.go`: focused tests for business rules and handler behavior

Example boundary:
- `contacts/service.go` can call `contacts/repository.go` and `activities/service.go`
- it should not know raw SQL details
- handlers should not compose SQL directly

Shared cross-cutting pieces:
- auth/session middleware
- org membership resolution middleware
- request ID middleware
- pagination helpers
- typed application errors
- JSON response helpers
- audit/activity writer

Backend rules:
- keep handlers thin
- keep services explicit
- write SQL that a human can read six months later
- prefer small functions over generic magic
- use transactions for multi-write flows
- always scope tenant-owned queries by `organization_id`

---

## 7. Frontend design

The frontend should feel boring in a good way: obvious, fast, clean, and low-friction.

Use:
- React with JavaScript, not TypeScript for this repo
- route-based feature organization
- local component state for UI concerns
- `fetch` plus small feature-local hooks for server interaction
- plain CSS with design tokens, layout primitives, and reusable form/table/card styles
- small reusable UI primitives after patterns appear twice, not before

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
- same filter/search bar pattern for contacts, companies, and deals
- create and edit flows should feel consistent across entities
- keyboard-friendly forms
- clear empty states
- fast perceived performance through good loading states and optimistic-feeling UI, not fake spinners everywhere
- no giant global state store for CRUD screens
- no component framework that dictates the product’s personality

### UI/UX design stance

A nice UI here means:
- readable hierarchy
- low visual noise
- sharp spacing discipline
- predictable actions
- obvious next steps
- no cramped forms
- table/list views that are actually usable
- detail pages that group related information well

Recommended visual approach:
- neutral base palette with one strong accent color
- generous whitespace
- 8px spacing scale
- strong typography contrast
- cards and panels used sparingly, not everywhere
- subtle borders and shadows, not glassmorphism clownery
- inline feedback close to the field or action that caused it

### Frontend dependency stance

Do this with code, not libraries:
- validation helpers in `src/lib/validation.js`
- reusable `Field`, `Button`, `Card`, `Table`, and `Modal` components
- CSS tokens in `tokens.css`
- shared page layout primitives in `components/layout`

That is enough to make it look polished if the taste is good.

---

## 8. Auth and authorization

MVP auth should be simple and robust:
- email + password login
- password hashes with Argon2id
- opaque server-side sessions stored in Postgres
- session cookie is HttpOnly, Secure in production, SameSite=Lax
- org membership checked on every tenant-scoped request

Authorization rules:
- `owner/admin`: full org management
- `member`: create/update CRM records
- `viewer`: read-only

Do not do OAuth first. It looks fancy and solves the wrong problem for an MVP.

---

## 9. API shape

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

Validation rules:
- validate on the backend with explicit request structs and validation functions
- client-side validation should improve UX, not replace server validation
- use stable error codes even if human messages change
- normalize empty strings to `null` where that is the actual domain meaning

Pagination:
- offset pagination is fine for MVP if kept consistent
- default page size 25, cap at 100

Search:
- simple `ILIKE` search first
- add trigram or full-text indexes only when real data proves it matters

---

## 10. Activity and audit trail

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

## 11. Local development and containerization

Use Docker Compose for Postgres.

Initial `docker-compose.yml` should include:
- `postgres` service on port `5432`
- named volume for persistence
- healthcheck with `pg_isready`

Environment variables:
- `DATABASE_URL`
- `SESSION_COOKIE_SECRET`
- `APP_BASE_URL`
- `WEB_BASE_URL`
- `API_BASE_URL`
- `NODE_ENV`
- `GO_ENV`

Recommended local dev commands:
- `make db-up`
- `make db-migrate`
- `make db-seed`
- `make api-dev`
- `make web-dev`

Do not containerize the app immediately unless deployment requires it. Containerize Postgres first. Keep app dev fast.

---

## 12. Testing strategy

Be selective and serious.

### Backend
- unit tests for business rules in services
- handler tests with `httptest`
- integration tests for DB behavior and tenant isolation
- auth/session edge case tests

### Frontend
- component tests for forms, lists, and validation behavior
- route-level tests for main screens
- Playwright smoke tests later for:
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

## 13. CI and quality gates

Initial CI should stay lean:
- setup Go and run backend tests
- setup Node and run frontend install/tests/build
- run lint if linting is actually configured and useful
- keep CI fast enough that it does not become background wallpaper

Do not build a Rube Goldberg CI pipeline. Fast and trustworthy beats elaborate.

---

## 14. Permissions matrix

Do not hand-wave authorization. Write it down and code to it.

### Organization and users

| Capability | owner | admin | member | viewer |
|---|---|---|---|---|
| View org data | yes | yes | yes | yes |
| Manage org settings | yes | yes | no | no |
| List users | yes | yes | no | no |
| Create users | yes | yes | no | no |
| Change roles | yes | yes, except owner transfer | no | no |
| Deactivate users | yes | yes | no | no |

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
- owner transfer is not an MVP feature unless deliberately implemented later
- viewer cannot mutate anything, period

---

## 15. Data lifecycle and record rules

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

## 16. Query, indexing, and performance posture

Do not optimize for fantasy scale, but do not be sloppy either.

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
- cap page size aggressively
- if search becomes hot, add trigram indexes on the few fields that matter

Dashboard posture:
- compute simple aggregates live first
- if dashboard queries become visibly slow, add small read-model queries or materialized views later
- do not add workers or caches just to feel sophisticated

---

## 17. Seed data, fixtures, and developer ergonomics

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
- one command to run the API
- one command to run the web app
- if a command needs five env vars and three manual steps, the plan is bad

---

## 18. Observability, logging, and operational sanity

Build just enough to debug real failures.

Backend logging:
- structured logs in production
- readable logs in local dev
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

## 19. Delivery plan by milestone

This repo should be built to a few concrete milestones, not as an endless blob of “in progress.”

### Milestone A: Foundation
Includes:
- Slice 0
- Slice 1
- minimal README
- CI for backend and frontend tests

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

## 20. MVP implementation slices

These are the right slices to ship in order.

### Slice 0: Repo bootstrap

Objective: create the repo structure, local dev setup, baseline Go API, baseline JS web app, and Postgres wiring.

Files to create:
- `Makefile`
- `.gitignore`
- `.env.example`
- `docker-compose.yml`
- `README.md`
- `apps/api/go.mod`
- `apps/api/cmd/open_crm_api/main.go`
- `apps/api/internal/config/env.go`
- `apps/api/internal/platform/logger/logger.go`
- `apps/api/internal/platform/web/json.go`
- `apps/api/internal/platform/web/errors.go`
- `apps/api/internal/modules/health/handler.go` or equivalent health route wiring
- `apps/web/package.json`
- `apps/web/vite.config.js`
- `apps/web/src/main.jsx`
- `apps/web/src/app/router.jsx`
- `apps/web/src/routes/root.jsx`
- `apps/web/src/routes/dashboard.jsx`
- `apps/web/src/styles/*`

Acceptance criteria:
- Go API boots and serves `/healthz`
- web app boots and renders a clean placeholder shell
- Postgres starts with Docker Compose
- `make` targets exist for the common dev flows
- CI can run basic backend/frontend checks

### Slice 1: Database and schema foundation

Objective: set up Postgres access, SQL migrations, core tables, and seed data.

Files to create:
- `apps/api/internal/db/postgres.go`
- `apps/api/internal/db/migrations/*`
- `apps/api/cmd/migrate/main.go`
- `apps/api/cmd/seed/main.go`
- `apps/api/internal/modules/*/repository.go` foundation files as needed

Acceptance criteria:
- migrations create the core auth and CRM tables
- a seed command creates a default org, owner user, and default deal stages
- schema includes tenant boundaries and indexes
- `/readyz` reflects DB readiness

### Slice 2: Auth and current-session flow

Objective: make login/logout/me fully work.

Files to create:
- `apps/api/internal/modules/auth/*`
- `apps/web/src/routes/login.jsx`
- `apps/web/src/features/auth/*`

Acceptance criteria:
- user can log in with seeded account
- session cookie persists across page refresh
- `/auth/me` returns current user + org membership
- unauthorized routes redirect to login

### Slice 3: Organizations and user management

Objective: basic org context and user listing/create flow.

Files to create:
- `apps/api/internal/modules/orgs/*`
- `apps/api/internal/modules/users/*`
- `apps/web/src/routes/settings/users.jsx`

Acceptance criteria:
- current org resolves on each request
- admin can list users in org
- admin can create another user account for the org
- role-based access is enforced

### Slice 4: Contacts vertical slice

Objective: contacts list, detail, create, update, archive, and activity entries.

Files to create:
- `apps/api/internal/modules/contacts/*`
- `apps/web/src/routes/contacts/index.jsx`
- `apps/web/src/routes/contacts/detail.jsx`
- `apps/web/src/features/contacts/*`

Acceptance criteria:
- searchable paginated contact list
- create/edit form with validation
- detail page shows notes/tasks/activity sections
- archive hides contact from default list

### Slice 5: Companies vertical slice

Objective: same quality bar as contacts, plus contact/company linking.

Files to create:
- `apps/api/internal/modules/companies/*`
- `apps/web/src/routes/companies/index.jsx`
- `apps/web/src/routes/companies/detail.jsx`
- `apps/web/src/features/companies/*`

Acceptance criteria:
- company CRUD works
- contact can be linked to one or more companies
- company detail shows linked contacts and recent activity

### Slice 6: Deals and stages vertical slice

Objective: deal pipeline, stage changes, and summary metrics.

Files to create:
- `apps/api/internal/modules/deals/*`
- `apps/web/src/routes/deals/index.jsx`
- `apps/web/src/routes/deals/detail.jsx`
- `apps/web/src/features/deals/*`

Acceptance criteria:
- deal CRUD works
- stage transitions work
- list view supports filtering by stage and owner
- dashboard can show open deals, won deals, and total pipeline value

### Slice 7: Notes, tasks, and activity timeline

Objective: shared engagement layer across CRM entities.

Files to create:
- `apps/api/internal/modules/notes/*`
- `apps/api/internal/modules/tasks/*`
- `apps/api/internal/modules/activities/*`
- `apps/web/src/features/notes/*`
- `apps/web/src/features/tasks/*`

Acceptance criteria:
- note creation works for contacts, companies, deals
- tasks can be assigned and completed
- entity detail pages show unified activity timeline

### Slice 8: Dashboard and UX polish

Objective: make the app feel coherent, pleasant, and fast.

Files to create:
- `apps/api/internal/modules/dashboard/*`
- `apps/web/src/routes/dashboard.jsx`
- `apps/web/src/features/dashboard/*`
- `apps/web/src/styles/components.css`
- `apps/web/src/styles/layout.css`

Acceptance criteria:
- dashboard shows counts and recent activity
- empty states are handled cleanly
- spacing, typography, and feedback patterns are consistent
- app is usable without dev-only knowledge

### Slice 9: CI hardening and release readiness

Objective: make the repo safe to keep shipping from `main`.

Files to create:
- `.github/workflows/ci.yml`
- `apps/api/test/integration/*`
- `apps/web/src/**/*.test.jsx`
- `apps/web/e2e/*` later if needed

Acceptance criteria:
- CI runs on push to main
- critical auth and CRUD flows are covered
- readme documents setup, dev flow, and env vars

---

## 21. Detailed execution plan for Slice 0: repo bootstrap

This is the first slice because it makes every later slice cheaper.

### Outcome
By the end of Slice 0:
- the repo boots cleanly
- Postgres runs locally via Docker Compose
- API and web apps both boot
- the frontend has a good-looking shell, not raw scaffolding garbage
- CI can run without heroics

### Task 0.1: Create root workflow files

Files:
- create `Makefile`
- create `.gitignore`
- create `.env.example`
- create `README.md`
- create `docker-compose.yml`

Recommended root targets:
- `db-up`
- `db-down`
- `db-migrate`
- `db-seed`
- `api-dev`
- `web-dev`
- `test-api`
- `test-web`
- `test`

### Task 0.2: Bootstrap the Go API module

Files:
- create `apps/api/go.mod`
- create `apps/api/cmd/open_crm_api/main.go`
- create `apps/api/internal/config/env.go`
- create `apps/api/internal/platform/logger/logger.go`
- create `apps/api/internal/platform/web/json.go`
- create `apps/api/internal/platform/web/errors.go`
- create `apps/api/internal/platform/web/middleware.go`

Initial behavior:
- API starts on configurable port
- `GET /healthz` returns process health
- request IDs are assigned early
- env parsing is strict and fails startup loudly when broken

### Task 0.3: Bootstrap the JS web app

Files:
- create `apps/web/package.json`
- create `apps/web/vite.config.js`
- create `apps/web/index.html`
- create `apps/web/src/main.jsx`
- create `apps/web/src/app/router.jsx`
- create `apps/web/src/app/providers.jsx`
- create `apps/web/src/app/shell.jsx`
- create `apps/web/src/routes/root.jsx`
- create `apps/web/src/routes/dashboard.jsx`
- create `apps/web/src/styles/tokens.css`
- create `apps/web/src/styles/base.css`
- create `apps/web/src/styles/layout.css`
- create `apps/web/src/styles/components.css`

Initial behavior:
- Vite dev server boots
- root route renders a clean application shell
- navigation and page layout feel intentional, not default-template ugly
- CSS tokens define color, spacing, border radius, shadow, and typography scales

### Task 0.4: Add minimal reusable UI primitives

Files:
- create `apps/web/src/components/ui/button.jsx`
- create `apps/web/src/components/ui/card.jsx`
- create `apps/web/src/components/ui/field.jsx`
- create `apps/web/src/components/layout/app_header.jsx`
- create `apps/web/src/components/layout/side_nav.jsx`
- create `apps/web/src/components/layout/page_header.jsx`

Rules:
- keep components small and obvious
- style them well once instead of importing a UI framework
- prioritize readability, spacing, and hover/focus states

### Task 0.5: Add Docker Compose for Postgres

Files:
- create `docker-compose.yml`

Rules:
- Postgres is the only required container in local dev for now
- use a named volume
- expose `5432`
- add healthcheck
- do not build app containers yet unless deployment forces it

### Task 0.6: Add baseline test and CI wiring

Files:
- create `apps/api/internal/platform/web/health_test.go` or equivalent
- create `apps/web/src/routes/dashboard.test.jsx`
- create `.github/workflows/ci.yml`

Initial quality bar:
- backend test proves API can answer `/healthz`
- frontend test proves the placeholder shell renders
- CI runs backend tests and frontend tests/build

### Task 0.7: Verify Slice 0

Commands:
```bash
make db-up
make api-dev
make web-dev
make test
```

Expected outcome:
- Postgres container is healthy
- API boots cleanly
- web app boots cleanly
- initial UI shell already looks deliberate
- tests pass

### Slice 0 exit criteria
- a fresh clone can boot in under 10 minutes
- the web app does not look like bare scaffolding
- there is one obvious place for backend code and one for frontend code
- no fake abstractions were introduced for imaginary future scale

---

## 22. Detailed execution plan for Slice 1: database and schema foundation

This slice is where the repo stops being scaffolding and starts being a product.

### Outcome
By the end of Slice 1:
- `pgx` is wired to Postgres
- SQL migrations create the core auth and CRM tables
- the repo can seed a usable demo org
- DB readiness works
- tests cover schema assumptions and tenant scoping where practical

### Task 1.1: Add DB dependencies and connection layer

Files:
- modify `apps/api/go.mod`
- create `apps/api/internal/db/postgres.go`
- create `apps/api/cmd/migrate/main.go`

Rules:
- DB connection creation lives in one place
- use `pgxpool`
- use env-driven connection strings only
- do not scatter DB config across files

### Task 1.2: Create explicit SQL migrations

Files:
- create `apps/api/internal/db/migrations/001_initial_schema.sql`
- create `apps/api/internal/db/migrations/002_seed_helpers.sql` only if it truly earns its existence

Tables:
- `organizations`
- `users`
- `organization_memberships`
- `sessions`
- `companies`
- `contacts`
- `contact_company_links`
- `deal_stages`
- `deals`
- `notes`
- `tasks`
- `activities`

Rules:
- migrations are reviewed SQL, not generated sludge trusted blindly
- index and constraint names should be readable
- every tenant-owned table carries `organization_id`

### Task 1.3: Add DB readiness and startup integration

Files:
- modify API startup wiring
- add DB checks to `GET /readyz`

Behavior:
- `/healthz` returns process-up regardless of DB state
- `/readyz` fails if DB cannot be reached
- startup logs print enough to diagnose DB config failures fast

### Task 1.4: Create seed command

Files:
- create `apps/api/cmd/seed/main.go`

Seed content:
- dev org and users from Section 17
- default deal stages
- small sample CRM dataset

Rules:
- seed should be re-runnable without exploding
- passwords must be hashed the same way production auth will hash them
- print seeded credentials clearly at the end

### Task 1.5: Add DB integration tests

Files:
- create `apps/api/test/integration/db_schema_test.go`
- create `apps/api/test/integration/tenant_isolation_test.go`

Minimum tests:
- migration-applied schema can insert a user/org/membership tuple
- duplicate membership is rejected
- cross-org linked data is rejected by service validation or constraints where relevant
- readiness endpoint passes with DB up

### Task 1.6: Verify Slice 1

Commands:
```bash
make db-up
make db-migrate
make db-seed
make test-api
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

## 23. Non-negotiable implementation rules for the first build phase

These rules prevent stupid drift early.

- no branchy speculative architecture work before Slice 0 and 1 are real
- no Gin/Echo/Fiber because “everyone uses it”
- no ORM if plain SQL is still readable
- no generic repository base class unless it materially simplifies real code already written
- no Redis, queues, cron workers, or websocket layer unless a real feature forces them
- no frontend state library for CRUD data that plain React and a small API client already handle fine
- no Tailwind or component framework as a crutch for visual design
- no unbounded list endpoints
- no nullable chaos; if a field is optional, define why
- no auth token gymnastics when a good session cookie solves the problem cleanly

---

## 24. What to document as the repo evolves

`mvp.md` is the product and engineering plan. Keep it honest.

As implementation begins, also add:
- `README.md` for setup and daily dev workflow
- `docs/adr/` for short architecture decisions when a decision becomes sticky
- `docs/runbooks/local-development.md` if setup gets non-trivial
- `docs/runbooks/release.md` once deployment exists
- `docs/ui-guidelines.md` once the first polished UI patterns exist

Good ADR candidates:
- why Go stdlib HTTP beat framework sprawl here
- why server-side sessions beat JWT for this repo
- why plain SQL beat ORM abstraction for MVP
- why the frontend stayed JavaScript with minimal dependencies

If a decision stops being true, update the docs. Dead docs are worse than no docs.

---

## 25. Immediate next move

Start with Slice 0 and Slice 1 only.

Do not try to scaffold the whole product in one blast. That is how repos turn into dead weight.

The first real build sequence should be:
1. repo bootstrap
2. Go API skeleton
3. JS web shell with polished base styles
4. Postgres + SQL migrations
5. seeded owner login
6. contacts slice
7. companies slice
8. deals slice

Before writing the first line of feature code, lock these repo-level decisions:
- backend is Go + stdlib HTTP + pgx + Postgres
- frontend is JavaScript + React + Vite + React Router + plain CSS
- auth is server-side sessions, not JWT cargo culting
- visual quality comes from deliberate design primitives, not dumping a UI framework into the repo
- dependency count stays low unless a dependency clearly buys speed, quality, or reliability

If those are clean, the rest gets much easier.
