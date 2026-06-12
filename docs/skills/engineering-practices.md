# Engineering Practices — Go Patterns, Code Style & Architecture

**Source docs:** AGENTS.md (Coding Rules, Engineering Patterns, File Change Checklist)

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

**When adding a prompt:**
1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file — uses the template, never inlines strings

## Architecture Rules

- Dependencies point inward: `internal/core/` → `internal/platform/` → Models. Never the reverse.
- SOLID: Single Responsibility, Open/Closed, Liskov Substitution, Interface Segregation, Dependency Inversion.
- Clean Architecture boundaries: transport handlers never contain business logic.
