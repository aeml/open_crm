# ADR-004: JavaScript over TypeScript on the frontend

Status: accepted
Date: 2024-01-01

## Context

Open CRM's frontend is a React app built with Vite. JavaScript and TypeScript are both valid choices. TypeScript adds static type checking and IDE completion at the cost of a compilation step, type annotation overhead, and configuration surface.

## Options considered

**TypeScript** — catches type errors at build time. Enables richer IDE tooling (go-to-definition, refactor-safe renames, autocomplete on API shapes). Requires `.tsconfig.json`, type declarations for third-party packages, and explicit annotation of component props, API responses, and utility functions. Adds friction for quick iteration early in a project's life.

**JavaScript with JSDoc** — JavaScript with optional inline type hints via JSDoc. Some editor tooling benefits without compilation. Type discipline is voluntary.

**Plain JavaScript** — no type annotations, no compilation beyond Vite's transpile step. Components, hooks, and utilities are short enough that type errors are caught quickly via tests and review. The API surface between frontend and backend is small, explicit, and stable.

## Decision

Use plain JavaScript (no TypeScript).

Reasons:
- The frontend has a small, well-understood API surface with stable contracts documented in `mvp.md` and the backend route handlers.
- Component files are short; prop shapes are obvious from usage.
- Test coverage (Vitest + Testing Library) catches the class of errors TypeScript would also catch, without build-time type plumbing.
- Adding TypeScript mid-project requires annotating all existing files to avoid `any` sprawl, creating a migration cliff.
- Vite and React are fast without a TypeScript compilation step.
- The dependency philosophy (minimal, deliberate) applies to build tooling too.

## Revisit conditions

Revisit this decision if:
- The API response shapes become complex enough that manual inspection is error-prone.
- Team size grows to a point where shared type contracts across many contributors meaningfully reduce integration bugs.
- A custom fields or integrations layer introduces a large number of dynamic shapes that TypeScript generics would constrain usefully.

A future ADR should document the migration if this changes. Do not introduce TypeScript incrementally via `// @ts-check` in select files; the noise exceeds the benefit.

## Consequences

- No `tsconfig.json`, no type declaration files, no `@types/*` packages.
- IDE completion relies on JSDoc annotations in library helpers where they are added voluntarily.
- New API fields must be traced through usage manually; tests and review serve this function.
- A future migration to TypeScript is possible but requires annotating the full existing surface.
