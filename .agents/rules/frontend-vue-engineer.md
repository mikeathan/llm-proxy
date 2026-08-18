---
description: Staff Frontend Engineering constitution for Vue 3, TypeScript, UX and AI coding assistants.
---

# Staff Frontend Engineering Constitution

**Target:** `**/*.vue`, `**/*.ts`, `**/*.js`

## Core Principles

-   Correctness, maintainability, accessibility, security, performance,
    consistency.
-   Follow existing repository conventions before introducing new
    patterns.
-   Extend existing abstractions; avoid unnecessary dependencies.
-   Produce complete, production-ready code.

## Architecture

-   Layering: **Views → Feature Components → Composables → Services →
    HTTP Client**.
-   Business logic belongs in composables/services, never presentation
    components.
-   Domain models remain separate from API DTOs.
-   Prefer composition over inheritance.
-   **Centralization:** app-level init (startup data refresh, global watchers,
    socket/SSE subscriptions, bootstrap) must live in **one centralized
    composable/bootstrap** (e.g. a module-level singleton that registers its own
    watchers), not scattered across components — so frontend lifecycle wiring is
    not forgotten or duplicated.

## Vue 3

-   Use `<script setup>` and Composition API.
-   Prefer `ref()`; use `reactive()` only for cohesive object state.
-   Use typed `defineProps`, `defineEmits`, `defineSlots` where
    applicable.
-   Never mutate props.
-   Use `computed()` for derived state; `watch()` only for side effects.
-   Avoid deep watchers unless unavoidable.
-   Shared composable state must be intentional.

## TypeScript

-   No `any`; prefer `unknown`.
-   Use discriminated unions and exhaustive switches.
-   Prefer inference over redundant annotations.
-   Keep types in `/types`. **Export types ONLY from `src/types/`** — never declare or
    export an interface/type (`export type` / `export interface`) from `utils/`,
    `composables/`, or components; import it from `src/types/` with `import type` instead.
    **Enforced:** ESLint `no-restricted-syntax` in `frontend/eslint.config.js`
    (`types-must-live-in-src-types`) fails `npm run lint` on any exported type/interface
    outside `src/types/**`. Pure logic bound to a type (e.g. `isStreamingPhase`) may live in
    `utils/`/`constants/`, but MUST `import type` the type from `src/types/`.
-   Annotate API response types; never consume untyped responses.
-   No hardcoded values: strings, numbers in logic → named `const` at file top. Errors, labels, keys included.
-   **Fixed-value option sets → a typed union + `as const`/`Record` map, never a bare
    `string` in domain types** (precedent: `LoopStrategy`, `WorkloadClass`). Backend-driven
    open sets (e.g. `loop_strategy_options`) stay `string[]`; the typed union covers the
    known values and helpers degrade gracefully for unknown runtime values.
-   **No inline `import("...")` type references.** Always import a type at the
    top of the file (`import type { T } from "..."`) and reference `T` by name.
    Inline dynamic-type imports are banned — they are unreadable and defeat the
    single import block.

## Components

-   Single responsibility.
-   Presentation components receive props and emit events.
-   No API calls inside presentation components.
-   Promote reusable UI into base components.
-   Prefer slots over duplication.
-   **Extract shared/reusable UI into base components under `src/components/common/<domain>/`**
    (e.g. `display/`, `buttons/`, `layout/`, `chat/`). The second use of the same markup
    is the trigger to extract; a single-use block stays inline where it belongs. Never
    copy-paste a field/control across features.

## State

-   Prefer local state.
-   Use `provide/inject` for feature scope.
-   Use Pinia only for true application state.
-   Avoid global state by default.

## Services

-   Components never call `fetch()`.
-   Centralize authentication, retries and error handling.
-   Validate responses before use.
-   Use strategy/registry instead of switch chains.

## Performance

-   Lazy-load routes.
-   Virtualize large lists.
-   Debounce search; throttle resize/scroll.
-   Use `v-show` for frequent toggles, `v-if` for infrequent rendering.
-   Measure before optimizing; use `computed()`, virtualization, or memoization only when justified by measured cost.

## Accessibility & UX

-   Semantic HTML first.
-   Full keyboard support.
-   Visible focus states.
-   Label all controls.
-   Use ARIA only when native semantics are insufficient.
-   Every feature supports: Loading, Empty, Success, Error, Permission
    and Offline states.

## Content Design

-   Plain English.
-   Active voice.
-   Action-oriented labels.
-   Helpful errors explaining cause and next action.
-   Consistent terminology.
-   Avoid vague messages like "Something went wrong."

## Security

-   Never trust client input.
-   Sanitize untrusted HTML; avoid `v-html`.
-   Never expose secrets or tokens.
-   Prefer HttpOnly cookies when possible.
-   Validate before sending to APIs.
-   Never log credentials, JWTs or PII.

## Observability

-   Structured telemetry.
-   Correlate client/server requests.
-   Capture rendering and API failures.
-   Console logging only during development.

## Testing

- Unit: Vitest. Tests live in `src/__TESTS__/` **mirroring the source tree** — a test for
  `src/utils/message/textAppend.ts` goes in `src/__TESTS__/utils/message/textAppend.test.ts`.
  Run with `npm test` (`vitest run`).
- Component: Vue Test Utils.
- E2E: Playwright.
- Test behavior, not implementation.
- Cover accessibility and error paths.

## Operational Rules

1.  Read existing patterns first.
2.  Match project conventions.
3.  Refactor duplication.
4.  Prefer native browser APIs.
5.  Avoid unrelated code changes.
6.  Ship complete implementations.

## Before Coding

-   Understand the feature.
-   Identify reusable components.
-   Reuse existing composables/services.
-   Consider accessibility, security and performance.
-   Add tests where behavior changes.
