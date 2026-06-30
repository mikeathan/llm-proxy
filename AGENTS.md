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
go run ./tools/check-complexity/   # cyclomatic complexity check (threshold 12)
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
2. Read the relevant SPEC file(s) for the subsystem you are modifying. See [`docs/INDEX.md`](docs/INDEX.md) for the mapping of subsystems to SPEC IDs (SPEC-001 through SPEC-009).
3. `.agents/rules/` has deeper Go and Vue guidance — check there if a task needs architecture-level patterns.
4. See [`docs/INDEX.md`](docs/INDEX.md) for the full documentation catalog.
5. Run `go build ./... && go test ./...` to establish a clean baseline.

## Checklist: Adding a Frontend Settings Tab

When adding a new settings tab (e.g. "Communication", "Notifications"):

1. Add tab name to `SettingsTab` type in `frontend/src/types/admin.ts`
2. Add icon + label in `frontend/src/constants/providers.ts`
3. If the tab is NOT a cloud provider, add exclusion to `isProviderTab()` in `frontend/src/domain/settings.ts`
4. Register in the appropriate settings group in `getSettingsGroups()` in `frontend/src/domain/settings.ts`
5. Create the settings component in `frontend/src/components/settings/`
6. Import the component in `frontend/src/components/settings/Settings.vue`
7. Add `v-show="activeTab === 'your-tab'"` div in the Settings.vue template
8. Run `npm run build` — TS errors will catch any missing icon/label entries

## Documentation stewardship: After any change, load `docs/skills/documentation-stewardship.md` and follow its post-completion checklist.

## Cyclomatic Complexity (Go — Backend)

Every Go function must have McCabe cyclomatic complexity ≤ 12. Enforced by `backend/tools/check-complexity/` (stdlib only — no external deps). Run `go run ./tools/check-complexity/` before committing. If it fails, extract helpers or restructure to reduce branches.

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
- **Service response types**: every API method that calls `fetch()` must define its response type in `types/` and explicitly deserialise via `const data: T = await res.json(); return data`. Never return bare `res.json()` — the explicit variable type catches field mismatches at compile time if the backend shape changes.
