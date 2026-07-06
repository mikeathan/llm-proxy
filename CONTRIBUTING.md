# Contributing

## Quick Start

```bash
# Backend
cd backend
go build ./...
go test ./...
go run ./tools/check-complexity/   # cyclomatic complexity ≤ 12
go run main.go                     # start server on :4001

# Frontend
cd frontend
npm install
npm run dev       # dev server (proxies API to :4001)
npm run build     # → ../backend/internal/transport/http/frontend_dist/
```

## Before You Write Code

1. Read `AGENTS.md` (AI agents) or this file (human contributors).
2. Read `CONSTITUTION.md` — architectural invariants covering validation boundaries, system prompt format, model architecture, terminal/network safety, no telemetry, and abstraction boundaries.
3. Read the relevant SPEC file for the subsystem. See [`docs/INDEX.md`](docs/INDEX.md) for the mapping (SPEC-001 through SPEC-009).
4. Run `go build ./... && go test ./...` to establish a clean baseline.

## Code Standards

### Go (Backend)

- Every function must have McCabe cyclomatic complexity ≤ 12. Enforced by `backend/tools/check-complexity/`.
- Run `go build ./...` before committing — no linters, no pre-commit hooks.
- Keep imports grouped: stdlib → internal → external.
- Handler types go in `internal/transport/http/handlers/` (package `handlers`).
- Service interfaces go in `internal/core/` or `internal/platform/`.
- Defensive: validate workspace IDs via `validateID()`, gate filenames for path traversal.

### TypeScript / Vue (Frontend)

- Composables are singletons — module-level state shared across components.
- `ref()` over `reactive()`. Type imports from `types/`. Services are stateless.
- Behavior belongs on the type — no switch/if-else chains in consumers; each variant is its own module.
- Service response types: every `fetch()` method must define its response type in `types/` and explicitly deserialize via `const data: T = await res.json(); return data`.

### Git

- No direct pushes to main. PRs only.
- Conventional Commits format.
- The user manages all git operations — AI agents must ask before any git command.

## Documentation Stewardship

After any change, load `docs/skills/documentation-stewardship.md` and follow its post-completion checklist.

## Directory Map

```
backend/                          # Go module root
  internal/
    app/                          # Composition root (bootstrap.go)
    core/                         # Domain logic
      assistant/                  # Agent loop, guardrails, tools, sessions
      automation/                 # Dispatcher, automation executor, event bus
      llm/                        # LLM client, model registry, runtimes
      mcp/                        # MCP orchestrator, resource mirror
      proxy/                      # Message protocol, history, normalisation
      tools/                      # Tool implementations (terminal, filesystem, network)
    platform/                     # Infrastructure
      persistence/                # Workspace config, file I/O
      logging/                    # Structured logging
      storage/                    # File resolver, data manager
    transport/
      http/                       # Router, middleware, frontend embed
        handlers/                 # HTTP handler types (package handlers)
    testing/                      # Test mocks and helpers
  models/                         # Shared domain types
  tools/                          # Build tools (complexity check)

frontend/                         # Vue 3 + Vite
  src/
    components/                   # Vue SFC components
    composables/                  # Shared state + logic
    services/                     # Typed HTTP clients
      admin/                      # Admin API
      monitoring/                 # Metrics, logs
      mcp/                        # MCP CRUD
      automation/                 # Dispatcher, workspaces
      assistant/                  # Conversation API
      memory/                     # Memory store
      template/                   # Templates
    types/                        # TypeScript type definitions
    constants/                    # API endpoints, provider definitions
```
