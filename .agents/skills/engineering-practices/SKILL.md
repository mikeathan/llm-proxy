---
name: engineering-practices
description: "Repo engineering mechanics: Go patterns, error handling, code style, frontend icon conventions, and the ordered file-change checklists for a model field / tool / prompt / connector. Use when writing Go or Vue in this repo or adding one of those."
last_reviewed: 2026-10-04
---

# Engineering Practices — Go Patterns, Code Style & Architecture

**Source docs:** `docs/architecture.md` (package layout, contracts, file checklists), `.agents/rules/go-staff-engineer.md`, `.agents/rules/frontend-vue-engineer.md`

> Language-agnostic principles (naming, functions, comments, dependency boundaries,
> the smells checklist) live in [`clean-code`](../clean-code/SKILL.md); this file covers
> only the repo/Go/Vue mechanics that build on them. For multi-file changes, start with
> [`task-planning`](../task-planning/SKILL.md) to trace the real code path first.

---

## Go Coding Rules

### Comments
- No comments unless the WHY is non-obvious. Well-named identifiers document the WHAT.
- Never remove existing comments unless they are stale (referencing removed code, outdated behavior, or incorrect logic).
- Single-line only. No multi-line docstrings or comment blocks.

### Error Handling
- Validate at system boundaries (user input, external APIs) — trust internal code.
- Use `fmt.Errorf` with `%w` to wrap errors and maintain the chain.
- Use sentinel errors from `models/llm.go` for known LLM conditions.
- Return early — happy path to the left. No deep nesting of `if err == nil`.
- Don't log AND return — one or the other. Return for the caller to handle.

### Abstraction
- Don't DRY until the pattern repeats 3+ times.
- Don't add features, refactor, or introduce abstractions beyond what the task requires.
- No feature flags, backward-compat shims, or `// TODO` stubs.

### Cyclomatic Complexity & Readability
- Max 80 lines per function. If it grows larger, extract helpers.
- Cyclomatic complexity under 10 per function.
- Max 3 levels of nesting. Use early returns and guard clauses.
- Encapsulate transient loop/session state in temporary structs instead of passing multiple `*int`, `*bool` pointers.

## Engineering Patterns

Repo-specific patterns only. General patterns (constants over magic values, strategy maps over
growing switches, value objects, null object, command–query separation, no silent failures,
parameter immutability, named predicates) live in [`clean-code`](../clean-code/SKILL.md) §2/§5/§6 and
`.agents/rules/go-staff-engineer.md`.

### Constants over Magic Values
Every hardcoded string, int, or float in logic files must be a named `const`, grouped at the top of
the file. Exceptions: `0`, `1`, `""`, `nil` in zero-value initialisation or loop counters.

### Centralized UI Status Copy
Backend user-facing assistant status copy — including any emoji — lives in the `Msg*`
const block in `internal/core/assistant/agent_events.go` (the single documented home).
Never inline a status string or emoji in a notify method or handler: add a named const
and reference it from the producer and any matcher (e.g. `assistant.MsgExecutionComplete`
is published by the automation executor and matched by the webhook handler).

Emit through the two shared helpers — `a.notifySystem(MsgFoo)` for a verbatim const and
`a.notifySystemf(MsgFoo, args...)` when the const carries `%s` placeholders. Do not add a
per-message `notify<Thing>` wrapper that only renders one const: call the helper at the
site instead, and keep a wrapper only when it derives arguments (e.g. choosing which
`tool_call_format` to suggest) or emits a non-message event type.

## Idiomatic Go

- Zero-value initialization over constructors for simple types.
- Accept interfaces, return structs.
- **Name dependency interfaces; never inline `interface { ... }` in a parameter list.**
  Declare a named interface in the *consuming* package (interfaces belong to the consumer),
  list only the methods actually called (interface segregation), and let the concrete provider
  satisfy it implicitly. Precedents: `assistant.AgentStackContext` /
  `assistant.configSecretsReader`, `handlers.AdminService`. An inline method-set is undocumented,
  unreusable, drifts silently, and tends to carry methods no call site uses — drop those too.
- No getters/setters — export struct fields directly.
- Table-driven tests with `t.Run`.
- Prefer `range` over index-based loops.
- `var` zero-init for package-level, `:=` for local.

## File Organization

- One primary type per file, named after the type.
- Handlers in `internal/transport/http/` — thin, parse → call → write. No business logic.
- Services in `internal/platform/` — reusable infrastructure.
- Implementation in `internal/core/` — business logic with minimal imports from `internal/platform/`.
- No `init()` functions — explicit construction via `New*` or `Initialize*`.
- Split concerns across files within a package (e.g., `types.go`, `store.go`, `search.go`, `tags.go`, `fts.go` for a memory package).
- **Package size cap (soft, ~15 `.go` files incl. tests).** When a cohesive tool/feature family would push a package past it, extract that family into a subpackage instead of growing one flat package.
- **Multi-implementation families split by precedent.** The parent package owns the contract + facade + factory seam; the subpackage owns the concrete implementations and the explicit registration table. Existing pairs: `tools/communication.go` + `tools/notifiers/`, `tools/search.go` + `tools/searchproviders/` (search deliberately has no `init()` — the table is explicit). The subpackage may import the parent for the contract; the parent must never import the subpackage (import cycle).
- **Test files mirror their source 1:1** (`x.go` → `x_test.go`). Never add a `<feature>_test.go` with no matching source file — put shared test-only helpers in the test file that mirrors the source they support.

## File Change Checklist

Canonical path-level checklists live in `docs/architecture.md` → **File Change Checklist** (new model
field, tool category, single tool, connector, prompt, frontend settings tab, backend endpoint). Read
the one matching your change type there before editing.

**When adding a search provider** — the one recipe kept here (architecture.md links to it):
1. `models/config.go` — add the `SearchProvider*` const and its entry in `SearchProviderIDs()` (the frontend fallback `constants/search.ts` mirrors the set; the dropdown itself is backend-driven via `search_providers`).
2. `internal/core/tools/searchproviders/search_<name>.go` — implement `tools.SearchProvider` (`Search(ctx, query, timeRange tools.SearchTimeRange)`: map the window to the provider's recency filter and send no filter for `SearchRangeAny`; add the provider to `TestProviders_MapTimeRange` in `search_registry_test.go`); constructor `new<Name>Provider(cfg tools.SearchProviderConfig) (tools.SearchProvider, error)` rejecting a nil client and a missing key, bound responses with `io.LimitReader`, skip malformed result URLs (`validResultURL`), wrap non-2xx with `%w` via `providerStatusError` (which maps **401/403** to `models.ErrToolUnavailable` so the loop classifies a rejected key as terminal; other statuses stay plain), never log the API key or query. Add the mirroring `search_<name>_test.go` (httptest + the shared `newTestClient`).
3. `internal/core/tools/searchproviders/search_registry.go` — add one table row `{RequiresKey: true, New: new<Name>Provider}`; the drift test `search_registry_test.go` fails until the row matches `models.SearchProviderIDs()`.
4. No other plumbing: the operator key lives at `search:<provider>` (`SecretStore.SetSecret("search", "<provider>", key)`); availability, schema-hide, and live resolution are generic.

## Architecture Rules

- Dependencies point inward: `internal/core/` → `internal/platform/` → Models. Never the reverse.
- SOLID: Single Responsibility, Open/Closed, Liskov Substitution, Interface Segregation, Dependency Inversion.
- Clean Architecture boundaries: transport handlers never contain business logic.

## Frontend Conventions

### Icon/Emoji Centralization

All emoji and Unicode symbol constants live in `frontend/src/constants/icons.ts`. Never hardcode
`"⚠️"`, `"✅"`, or any Unicode symbol directly in a `.vue` template or `.ts` composable —
import the named constant instead (this prevents `⚠` vs `⚠️` variation-selector drift and keeps
one source of truth).

SVG icons are rendered by `Icon.vue` (`frontend/src/components/icons/Icon.vue`), which
dynamically imports `frontend/src/assets/svg/<name>.svg?component` by name. There is no barrel
file and no registration step — the filename *is* the API.

| Need | Use |
|------|-----|
| An SVG icon | Drop `<name>.svg` into `frontend/src/assets/svg/`, then `<Icon name="<name>" size="sm" />` (`spinner` is special-cased; sizes `xs/sm/md/lg`). |
| A text symbol / emoji | Add a named constant to `frontend/src/constants/icons.ts` and import it. |
| A single-use decorative SVG | Inline `<svg>` is acceptable; promote to `assets/svg/` on the **second** use. |

See the header comment in `constants/icons.ts` for the full rule.
