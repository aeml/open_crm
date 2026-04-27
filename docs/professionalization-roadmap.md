# Open CRM Professionalization Roadmap

This roadmap starts at version `0.1.1` and moves the project from MVP-complete to professional-grade without changing the core product direction: a small, explicit CRM built from a Go API, React web app, and Postgres.

Each version is a shippable slice. The goal is to improve safety, reliability, maintainability, and operator trust without introducing unnecessary platform complexity.

## Progress

- `0.1.1` Migration Safety: complete.
- `0.1.2` HTTP Runtime Hardening: complete.
- `0.1.3` CI Quality Gates: complete.
- `0.1.4` Tooling Reproducibility: complete.
- `0.1.5` Frontend API Client Consolidation: complete.
- `0.1.6` Request Validation And Body Limits: complete.

## Version 0.1.1 - Migration Safety

Status: complete.

Goal: make database deploys safer and repeatable.

- Add a `schema_migrations` table.
- Record applied migration names and timestamps.
- Skip already-applied migrations on future deploys.
- Keep migration execution explicit and easy to debug.
- Update migration tests to cover tracked execution.

Exit criteria:

- Running migrations twice does not reapply completed migrations.
- Backend tests pass with `go test ./...`.

## Version 0.1.2 - HTTP Runtime Hardening

Status: complete.

Goal: make the API process safer under production traffic and deploy restarts.

- Add `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, and `IdleTimeout` to the API server.
- Add graceful shutdown on `SIGINT` and `SIGTERM`.
- Add tests or small seams around server configuration where practical.
- Document production runtime behavior.

Exit criteria:

- API shuts down cleanly during deploys.
- Server has bounded request lifecycle defaults.

## Version 0.1.3 - CI Quality Gates

Status: complete.

Goal: separate quality validation from deployment.

- Add a general CI workflow for pushes and pull requests.
- Run `go test ./...` and `go vet ./...`.
- Run `npm ci`, `npm test`, and `npm run build` for the frontend.
- Add a formatting check for Go files.
- Keep deploy workflows focused on deployment.

Exit criteria:

- Pull requests fail before deploy if backend or frontend quality gates fail.
- CI uses the same Node version documented for local development.

## Version 0.1.4 - Tooling Reproducibility

Status: complete.

Goal: make local setup predictable for future contributors and deployment debugging.

- Add `.nvmrc` and optional `.node-version` for Node 18.
- Add package manager metadata where useful.
- Update README local development instructions.
- Confirm frontend tests and builds run from a clean checkout.

Exit criteria:

- A new contributor can install and test with the documented runtime versions.
- Local frontend verification no longer depends on an accidental global Node/npm version.

## Version 0.1.5 - Frontend API Client Consolidation

Status: complete.

Goal: reduce duplicated fetch behavior and make errors consistent.

- Create one shared frontend API request helper.
- Centralize `credentials: 'include'`, JSON parsing, error message extraction, and 204 handling.
- Preserve feature-specific helpers in `src/lib/*`, but make them call the shared client.
- Add tests for error parsing and empty responses.

Exit criteria:

- Feature API modules no longer duplicate low-level fetch boilerplate.
- Auth/session errors can be handled consistently in one place later.

## Version 0.1.6 - Request Validation And Body Limits

Status: complete.

Goal: make backend request handling more robust.

- Add bounded JSON body reads with `http.MaxBytesReader`.
- Add a shared JSON decode helper.
- Consider `DisallowUnknownFields` for API write payloads after checking current frontend behavior.
- Standardize bad-request responses.

Exit criteria:

- Oversized request bodies are rejected predictably.
- JSON decode behavior is consistent across handlers.

## Version 0.1.7 - Error Semantics

Goal: make API behavior more professional and predictable.

- Introduce module-level `ErrNotFound` values where needed.
- Map not-found errors to `404` instead of generic `500` responses.
- Standardize validation and conflict errors across modules.
- Add handler tests for not-found paths.

Exit criteria:

- Missing resources return consistent `404` responses.
- Client behavior can rely on stable error codes.

## Version 0.1.8 - Security Baseline

Goal: address the highest-value security gaps for cookie-auth CRM usage.

- Add CSRF protection for state-changing requests or document a deliberate alternative.
- Add login/bootstrap rate limiting.
- Remove unused `SESSION_COOKIE_SECRET` or wire it to a real purpose.
- Add basic security headers at the API or documented edge layer.
- Review session token storage and session cleanup.

Exit criteria:

- Cookie-auth write requests have CSRF mitigation.
- Auth endpoints have abuse protection.
- Configuration no longer contains unused security settings.

## Version 0.1.9 - User Lifecycle

Goal: replace temporary-password behavior with a safer onboarding workflow.

- Replace hardcoded temporary passwords with invite or password setup tokens.
- Add token expiry and one-time-use semantics.
- Add user-facing setup/reset flow in the frontend.
- Add tests for invite creation and consumption.

Exit criteria:

- New users are not created with a shared known password.
- Admin-created users can securely activate accounts.

## Version 0.2.0 - Observability And Operations

Goal: make production behavior easier to understand and support.

- Add structured request logging with method, path, status, duration, and request ID.
- Add API container health checks in deploy compose.
- Document backup and restore procedures for Postgres.
- Add a deploy/runbook document for common operational tasks.

Exit criteria:

- Production issues can be traced by request ID.
- Operators have documented backup, restore, and deploy recovery steps.

## Version 0.2.1 - Backend Maintainability

Goal: keep the modular monolith explicit while reducing oversized files.

- Split `internal/app/app.go` by handler area.
- Keep route registration centralized and easy to scan.
- Move repeated response and decode helpers into small platform/web utilities.
- Avoid new framework dependencies unless they clearly remove more complexity than they add.

Exit criteria:

- Handler files are easier to review in isolation.
- No behavior changes beyond tested refactors.

## Version 0.2.2 - Frontend Maintainability

Goal: make route components easier to evolve without changing the visual language.

- Extract large route sections into feature-local components where it reduces complexity.
- Add consistent loading, empty, error, and retry states.
- Add request cancellation with `AbortController` through the shared API client.
- Add minimal ESLint configuration.

Exit criteria:

- Large routes are easier to modify safely.
- Search and detail loading no longer leave unnecessary in-flight requests.

## Version 0.2.3 - Database Integrity

Goal: move important invariants closer to the data.

- Add constraints for roles, statuses, entity types, and positive monetary values.
- Add unique indexes where duplicates are not valid.
- Add integration-style migration verification against Postgres.
- Review indexes for common list/search patterns.

Exit criteria:

- Invalid core states are rejected by the database, not only by application code.
- Migration tests verify real schema outcomes.

## Version 0.3.0 - Professional Release Candidate

Goal: complete the professional-grade baseline and prepare for real usage feedback.

- Re-run security and reliability review.
- Verify full local bootstrap from clean checkout.
- Verify CI, deploy, migration, backup, and restore workflows.
- Update README to reflect current production posture and roadmap status.

Exit criteria:

- The project is ready to present as a professional, production-conscious CRM foundation.
- Remaining work is product-driven rather than infrastructure cleanup.
