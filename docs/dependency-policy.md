# Dependency And Runtime Policy

Last reviewed: 2026-07-20

Open CRM keeps a small dependency surface, but every dependency and runtime in
that surface must remain supported and auditable. The dependency budget in
[`mvp.md`](../mvp.md#4-dependency-budget) remains the architectural rule; this
document defines the release gate.

## Supported baseline

| Surface | Supported baseline | Repository enforcement |
| --- | --- | --- |
| Go | Go 1.26.5 (latest reviewed 1.26 patch) | `go 1.26.5`, exact CI version and builder image/digest |
| Node.js | Node 24 LTS | `.nvmrc`, `.node-version`, CI Node 24 |
| API runtime | Alpine 3.24 patch release | Exact tag and digest in `apps/api/Dockerfile` |
| Database | PostgreSQL 16.14 | Exact tag and digest in Compose, CI, and restore drills |
| Go modules | Minimal direct set: `pgx/v5` and `x/crypto` | `go.mod`, `go mod tidy`, Dependabot |
| Frontend packages | React/Vite application dependencies in the lockfile | `npm ci`, lockfile, Dependabot |

The Docker builder/runtime and PostgreSQL tags are patch-pinned and
digest-pinned so a deployment cannot pull a different base under the same
release identity. Dependabot proposes digest and module/package updates;
operators do not update a production database image independently of the same
CI and deploy-recovery path used for application changes.

## Required audit gates

- `go mod tidy` must leave `go.mod` and `go.sum` unchanged.
- `go vet ./...` and `go test -p 1 ./...` must pass on the exact reviewed Go
  1.26 patch against PostgreSQL. Package serialization avoids known lock-table
  pressure from concurrent fresh-schema integration suites.
- Pinned `gosec` must report no unsuppressed finding across all backend
  packages. Suppressions must name one rule inline and explain the already
  enforced invariant; bare or unexplained `#nosec` comments are prohibited.
- Pinned `govulncheck` must report no reachable symbol vulnerability. Package
  vulnerabilities imported by executable code are also release blockers.
- `npm audit --audit-level=high` scans the complete frontend lockfile, including
  development tooling. Any high or critical finding blocks release.
- `scripts/check-third-party-notices.mjs --check` must reproduce the committed
  notice from the exact shipped Go command graph and clean Linux/Node 24 npm
  install. Missing declarations, unreviewed SPDX identifiers, graph drift, or
  stale output block release.
- Base images must be on a supported upstream branch and use a reviewed exact
  digest. Reaching upstream end of support is a release blocker even when a
  scanner has no finding.

Moderate/low npm findings and Go module-only advisories are reviewed in the
same slice but do not automatically block when the affected package is neither
imported nor compiled into Open CRM. An exception must record the advisory,
exposure analysis, available fix, and removal/review condition here or in the
capability matrix.

Current reviewed exception: `GO-2026-5932` marks the unmaintained
`golang.org/x/crypto/openpgp` package. Open CRM requires `x/crypto` for Argon2id
but does not import or compile `openpgp`; there is no fixed module version and
`govulncheck -show verbose` reports it only at module level. Adding an OpenPGP
import is prohibited unless this exception is replaced by a maintained
implementation and its own security review.

## License and notice gate

[`scripts/license-policy.json`](../scripts/license-policy.json) is the explicit
reviewed SPDX allowlist and Go-module mapping. The generator inventories the
dependency graph reachable from `open_crm_api`, `migrate`, and `seed`, including
the Go standard library, and every package installed by the supported npm
lockfile. Optional packages unavailable on the supported Linux runner are not
part of that artifact.

[`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md) lists every distributed Go
and browser-runtime component and embeds the license and notice files shipped
by those packages. Vite and Rollup are conservatively treated as output
contributors because helpers or polyfills may enter the generated bundle.
Development-only npm packages are listed and validated, but their full texts
are not copied unless they are output contributors. The API image contains the
project license and generated notice; every supported frontend build emits the
generated notice beside the SPA.

Adding a dependency with an already allowed license still requires the change
review in this document. A new license identifier must be reviewed for its
obligations before the policy is changed; the allowlist is an engineering gate,
not a substitute for legal review.

## Change rules

1. Prefer the standard library and existing module seams. New production
   dependencies require a concrete capability, license review, maintenance
   signal, supported-runtime check, and tests at the failure boundary.
2. Patch/minor upgrades travel with tidy/lockfile output and all relevant
   tests. Major upgrades also require behavior and migration review.
3. Runtime/base upgrades rerun real-PostgreSQL backend tests, browser acceptance,
   backup/restore acceptance, immutable deployment recovery, and smoke checks.
4. Review supported branches and audit output at least monthly and before every
   pilot release. Security updates do not wait for a feature milestone.

Local evidence commands mirror CI:

```sh
cd apps/api
go mod tidy
go vet ./...
GOTOOLCHAIN=go1.26.5 go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 -quiet ./...
OPEN_CRM_TEST_DATABASE_URL='postgres://open_crm:open_crm@127.0.0.1:5432/open_crm_test?sslmode=disable' go test -p 1 ./...
GOTOOLCHAIN=go1.26.5 go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

cd ../web
npm ci
npm audit --audit-level=high
npm test
npm run lint
npm run check:source
npm run build:checked

cd ../..
node scripts/check-third-party-notices.mjs --check
```

After an intentional dependency or Go-toolchain change, regenerate on
Linux/Node 24 with `node scripts/check-third-party-notices.mjs --write`, review
the component and legal-text diff, and commit it with the dependency change.
