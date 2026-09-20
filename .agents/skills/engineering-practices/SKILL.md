---
name: engineering-practices
description: "Repo engineering mechanics: Go patterns, error handling, code style, frontend icon conventions, and file-change checklists. Use when writing Go or Vue in this repo."
when_to_use: "Editing Go or Vue in this repo; adding a tool, model field, or prompt; style/complexity questions."
status: reference
last_reviewed: 2026-07-11
---

# Engineering Practices — Go Patterns, Code Style & Architecture

**Source docs:** AGENTS.md (Coding Rules, Engineering Patterns, File Change Checklist)

> Language-agnostic principles (naming, functions, comments, dependency boundaries,
> the smells checklist) live in [`clean-code`](../clean-code/SKILL.md); this file covers
> the repo/Go/Vue mechanics that build on them.

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

### Constants over Magic Values
Every hardcoded string, int, or float in logic files must be a named `const`. Group related constants at the top of the file. Exceptions: `0`, `1`, `""`, `nil` in zero-value initialisation or loop counters.

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

### Strategy Pattern for Branching
When a `switch` or `if-else` chain grows with new cases over time, replace with a strategy map. New cases become registrations, not new branches.

```go
// Before (open for modification):
switch key {
case "a": ... break;
case "b": ... break;
}

// After (closed for modification, open for extension):
var strategies = map[string]Strategy{"a": ..., "b": ...}
strategies[key].execute()
```

### Value Objects for Domain Primitives
Use typed constants with `Validate()` instead of raw strings:

```go
type Scope string
const ( ScopeUser Scope = "user"; ScopeWorkspace Scope = "workspace" )
func (s Scope) Validate() error {
    if s != ScopeUser && s != ScopeWorkspace { return fmt.Errorf("invalid scope: %s", s) }
    return nil
}
```

Only for values with bounded valid states — not for freeform strings like names or messages.

### Null Object over nil Checks
Return a no-op object instead of nil when a function has a valid "do nothing" path. The caller shouldn't check for nil before every call.

### Command Query Separation
A function either returns data OR mutates state, never both. A save returns `error` or `ok`. A query returns data. If a function currently does both, split it.

### No Silent Failures
Every error must be handled — returned to the caller, logged, or explicitly ignored with a comment explaining why. Never use `_ = doSomething()` without a comment.

### Immutability for Function Parameters
Don't modify input slices or maps. Make a copy first (`append([]T{}, input...)` for slices, a fresh `map` for maps).

### Function Composition for Complex Conditions
Extract multi-condition checks into a named helper. `if isRetryable(err)` is better than `if errors.Is(err, io.EOF) || ...`.

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

**When adding a new model-level field:**
1. `models/config.go` or `models/infrastructure.go` — type definition
2. `internal/transport/http/registry_handlers.go` — request struct
3. `internal/transport/http/admin_handlers.go` — view struct
4. `internal/transport/http/admin_view.go` — view mapping
5. `internal/core/llm/manager.go` — if field affects runtime behaviour
6. `internal/testing/mocks/manager.go` — if interface changed
7. Frontend component (if UI field)

**When adding a tool:**
1. `models/tools.go` — constant
2. `internal/core/tools/manifests/{tool}.json` — manifest
3. `internal/core/tools/{tool_category}.go` — implementation
4. `internal/core/assistant/registry.go` — registration

**When adding a search provider:**
1. `models/config.go` — add the `SearchProvider*` const and its entry in `SearchProviderIDs()` (the frontend fallback `constants/search.ts` mirrors the set; the dropdown itself is backend-driven via `search_providers`).
2. `internal/core/tools/searchproviders/search_<name>.go` — implement `tools.SearchProvider`; constructor `new<Name>Provider(cfg tools.SearchProviderConfig) (tools.SearchProvider, error)` rejecting a nil client and a missing key, bound responses with `io.LimitReader`, skip malformed result URLs (`validResultURL`), wrap non-2xx with `%w` via `providerStatusError` (which maps **401/403** to `models.ErrToolUnavailable` so the loop classifies a rejected key as terminal; other statuses stay plain), never log the API key or query. Add the mirroring `search_<name>_test.go` (httptest + the shared `newTestClient`).
3. `internal/core/tools/searchproviders/search_registry.go` — add one table row `{RequiresKey: true, New: new<Name>Provider}`; the drift test `search_registry_test.go` fails until the row matches `models.SearchProviderIDs()`.
4. No other plumbing: the operator key lives at `search:<provider>` (`SecretStore.SetSecret("search", "<provider>", key)`); availability, schema-hide, and live resolution are generic.

**When adding a prompt:**
1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file — uses the template, never inlines strings

## Architecture Rules

- Dependencies point inward: `internal/core/` → `internal/platform/` → Models. Never the reverse.
- SOLID: Single Responsibility, Open/Closed, Liskov Substitution, Interface Segregation, Dependency Inversion.
- Clean Architecture boundaries: transport handlers never contain business logic.

## Frontend Conventions

### Icon/Emoji Centralization

All emoji and icon constants live in `frontend/src/constants/icons.ts`. Never hardcode
`"⚠️"`, `"✅"`, or any Unicode symbol directly in a `.vue` template or `.ts` composable.
Import the named constant instead. This centralises visual identity, prevents `⚠` vs `⚠️`
variation-selector inconsistency, and makes global updates possible in one place.

The frontend has **three competing icon systems** — know which to use:

| System | Location | Icons | When to use |
|--------|----------|-------|-------------|
| `UIIcon.vue` | `components/common/UIIcon.vue` | 12 UI icons (close, plus, check, chevron-*, trash, settings, play, stop, spinner, document, search) | **Default for new UI work** |
| `Icon.vue` | `components/icons/Icon.vue` | 3 SVG icons from `assets/svg/` (arrow-down, arrow-up, trash) | When SVG asset is needed from `assets/svg/index.ts` |
| Raw inline `<svg>` | Scattered across ~16 files | ~40+ unique inline SVGs | Only for single-use decorative icons that don't fit existing systems |

**Adding a new SVG icon used in 2+ components:**
1. Add SVG file to `assets/svg/`
2. Register in `assets/svg/index.ts` with named export
3. Add case to `UIIcon.vue` if it's a standard UI icon
4. Add named constant to `constants/icons.ts` if it's an emoji/Unicode symbol

See the header comment in `constants/icons.ts` for the full rule.
