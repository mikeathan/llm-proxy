# AGENTS.md — Instructions for Coding Agents
Written for AI coding assistants (Claude Code, Cursor, Copilot, etc.). Read before making changes.

## Git Policy
Never run any git operation (stash, commit, add, unstage, reset, push, pull, branch, etc.) without asking first. The user has full control over git. If you need git for any reason (e.g., checking baseline test behavior), ask before proceeding.

## Quick Start

Go module root is `backend/`. All `go` commands run from that directory.
Go 1.26.2. No golangci-lint, no Makefile, no pre-commit hooks — verification is manual.

```bash
# Backend — build, test, run
cd backend
go build ./...          # build all packages
go test ./...           # run all tests
go test ./internal/core/assistant/... -v   # agent-loop tests
go test ./internal/core/proxy/... -v       # parser + history tests
go run main.go          # start the server (default :4001)

# Frontend — dev server, production build
cd frontend
npm install             # install dependencies
npm run dev             # dev server (proxies API to :4001)
npm run build           # production build → ../backend/internal/transport/http/frontend_dist/

# Production server
cd backend
go run main.go --data ./data   # http://0.0.0.0:4001
```

For deep directory mappings, file change checklists, and architectural invariants, read `docs/architecture.md`.

## Before You Write Any Code

1. Read `CONSTITUTION.md` — it defines 6 architectural sections covering validation boundaries, system prompt format, model architecture, terminal/network safety, no telemetry, and abstraction/invariants. Your change must comply with all of them.
2. Read the relevant SPEC file(s) for the subsystem you are modifying. See [`docs/INDEX.md`](docs/INDEX.md) for the mapping of subsystems to SPEC IDs (SPEC-001 through SPEC-008).
3. `.agents/rules/` has deeper Go and Vue guidance — check there if a task needs architecture-level patterns.
4. See [`docs/INDEX.md`](docs/INDEX.md) for the full documentation catalog.
5. Run `go build ./... && go test ./...` to establish a clean baseline.

**Documentation stewardship**: After any change (new feature, refactor, behavior fix, or revert), update all affected docs:
- **SPEC files** (`docs/SPECS/`) — update behavioral contracts if the change alters system behaviour
- **Plan files** (`docs/PLANS/`) — add a new entry documenting what changed and why, organized by subsystem
- **Skill files** (`docs/skills/`) — add new gotchas, patterns, or architecture decisions discovered during the work
- **INDEX** (`docs/INDEX.md`) — add entries for any new files created; update statuses for changed plans
- **Audits** (`docs/audits/`) — create a new audit for any regression or post-mortem analysis
- **`docs/architecture.md`** — update Common Pitfalls if a new pattern emerges that future agents should know

## Coding Rules (Go)

### Comments

- **No comments unless the WHY is non-obvious.** Well-named identifiers document the WHAT.
- **Single-line only.** No multi-line docstrings or comment blocks.
- **Never remove existing comments unless they are stale** (referencing removed code, outdated behavior, or incorrect logic). If a comment is still accurate, keep it.
- If removing the comment wouldn't confuse a reader, remove it.

### Error Handling

- Validate at system boundaries (user input, external APIs) — trust internal code.
- Use `fmt.Errorf` with `%w` to wrap errors and maintain the chain.
- Use sentinel errors from `models/llm.go` for known conditions (`ErrUnknownModel`, `ErrModelExists`, `ErrModelStarting`).

### Abstraction

- Don't DRY until the pattern repeats 3+ times. Three similar lines > premature abstraction.
- Don't add features, refactor, or introduce abstractions beyond what the task requires.
- No feature flags, backward-compat shims, or `// TODO` stubs.

### Prompts

- ALL prompt strings go in `internal/core/assistant/prompts/templates.go`. Nowhere else.
- This includes system messages, nag prompts, parse-error feedback, JSON translations.

### Network & Terminal

- All network I/O via `NetworkTools` (never raw `http.Client` or `net.Dial`).
- All terminal execution via `ShellProvider` (never raw `os/exec`).
- All file paths validated with `IsSecurePath` for workspace jailing.

### Cyclomatic Complexity & Readability

- Keep functions short and focused: limit any function to a maximum of 80 lines. If a function grows larger, extract sub-logic into small, well-named helper functions.
- Keep cyclomatic complexity under 10 per function. Avoid nested conditionals deeper than 3 levels; instead, structure flow with early returns and guard clauses ("happy path to the left").
- Encapsulate transient loop or session state in temporary structs (e.g. `agentRunner`) instead of passing multiple pointers to simple type counters (like `*int`, `*bool`) between functions.
- Decouple orchestration logic from data parsing/formatting. Put parsing and validation logic in dedicated helper types/functions.

## Coding Rules (TypeScript/Vue — Frontend)

- `frontend/` is a Vue 3 + Vite + TypeScript SPA
- Composables are singletons — state is module-level, shared across components
- Use `ref()` for reactive state, not `reactive()`
- Type imports from `types/` directory (barrel exports)
- Services are stateless — API calls only, no local state caching
- Polling uses `mountCount` pattern: ref counts subscribers, stops when zero
- Dev server at `localhost:5173` proxies `/admin/api` to `:4001` — start backend first
- Build output goes to `../backend/internal/transport/http/frontend_dist/` (Go embed)
- `npm run build` runs `vue-tsc -b` (type-check) then `vite build` — TS errors fail the build
- Model form defaults and derived names live in `src/utils/modelUtils.ts` — reusable across components
